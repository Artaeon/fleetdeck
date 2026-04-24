package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks request and deployment counters for the Prometheus endpoint.
type Metrics struct {
	httpRequestsTotal   atomic.Int64
	httpRequestErrors   atomic.Int64
	deploymentsTotal    atomic.Int64
	deploymentsFailures atomic.Int64
	backupsTotal        atomic.Int64
	startedAt           time.Time

	// stopCh signals the cache refresh goroutine to exit. Closed by
	// Stop(), which is guarded by stopOnce so double-close does not
	// panic — matters because Shutdown can be called from both a
	// signal handler and a test cleanup in the same process.
	stopCh   chan struct{}
	stopOnce sync.Once

	// Cached system metrics (refreshed periodically)
	cacheMu          sync.RWMutex
	cachedProjects   int
	cachedRunning    int
	cachedStopped    int
	cachedContainers int
	cachedMemTotal   int64
	cachedMemAvail   int64
	cachedDiskTotal  int64
	cachedDiskUsed   int64
	cachedTraefikUp  int
	cacheUpdatedAt   time.Time
}

func newMetrics() *Metrics {
	return &Metrics{
		startedAt: time.Now(),
		stopCh:    make(chan struct{}),
	}
}

func (m *Metrics) incRequests()           { m.httpRequestsTotal.Add(1) }
func (m *Metrics) incErrors()             { m.httpRequestErrors.Add(1) }
func (m *Metrics) incDeployments()        { m.deploymentsTotal.Add(1) }
func (m *Metrics) incDeploymentFailures() { m.deploymentsFailures.Add(1) }
func (m *Metrics) incBackups()            { m.backupsTotal.Add(1) }

// Stop signals the cache-refresh goroutine (if any) to exit. Safe to
// call multiple times and safe to call when startCacheRefresh was
// never invoked — the sync.Once guards against both.
func (m *Metrics) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

// startCacheRefresh launches a background goroutine that refreshes cached
// system metrics every 30 seconds, so handleMetrics never runs expensive
// subprocess calls inline.
func (m *Metrics) startCacheRefresh(s *Server) {
	m.refreshCache(s)

	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.refreshCache(s)
			case <-m.stopCh:
				return
			}
		}
	}()
}

// refreshCache collects all expensive system metrics and stores them under
// cacheMu so that handleMetrics can read them cheaply.
//
// The whole refresh is bounded by a 10 s deadline — long enough for a
// healthy box, short enough that a slow DB query or a hung docker
// daemon doesn't stall the 30 s ticker and starve /metrics scrapes.
// Partial results are stored: if the DB query works but the docker
// `ps` call hangs, Prometheus still sees fresh project counts, just
// with traefikUp stuck at its last value.
func (m *Metrics) refreshCache(s *Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	projects, _ := s.db.ListProjects()
	var running, stopped, totalContainers int
	for _, p := range projects {
		switch p.Status {
		case "running":
			running++
		case "stopped":
			stopped++
		}
		_, cnt := countContainers(p.ProjectPath)
		totalContainers += cnt
		// Bail early if the outer budget is blown — prevents us from
		// walking 100 projects while the ticker fires again.
		if ctx.Err() != nil {
			log.Printf("metrics: timed out during project scan after %d projects", running+stopped)
			break
		}
	}

	memTotal, memAvail := parseMemInfo()
	diskTotal, diskUsed := parseDiskUsage(s.cfg.Server.BasePath)

	traefikUp := 0
	if ctx.Err() == nil {
		// exec.CommandContext so a hung docker daemon propagates the
		// timeout instead of pinning the refresh goroutine.
		if out, err := exec.CommandContext(ctx, "docker", "ps", "--filter", "name=traefik", "--format", "{{.Status}}").Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
			traefikUp = 1
		}
	}

	m.cacheMu.Lock()
	m.cachedProjects = len(projects)
	m.cachedRunning = running
	m.cachedStopped = stopped
	m.cachedContainers = totalContainers
	m.cachedMemTotal = memTotal
	m.cachedMemAvail = memAvail
	m.cachedDiskTotal = diskTotal
	m.cachedDiskUsed = diskUsed
	m.cachedTraefikUp = traefikUp
	m.cacheUpdatedAt = time.Now()
	m.cacheMu.Unlock()

	log.Printf("metrics cache refreshed: %d projects, %d containers", len(projects), totalContainers)
}

// handleMetrics serves a Prometheus-compatible /metrics endpoint using the
// text exposition format. No external dependencies required.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var b strings.Builder

	// Process info
	b.WriteString("# HELP fleetdeck_info FleetDeck server information.\n")
	b.WriteString("# TYPE fleetdeck_info gauge\n")
	fmt.Fprintf(&b, "fleetdeck_info{version=\"1.0.0\"} 1\n")

	// Uptime
	b.WriteString("# HELP fleetdeck_uptime_seconds Time since server started.\n")
	b.WriteString("# TYPE fleetdeck_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "fleetdeck_uptime_seconds %.0f\n", time.Since(s.metrics.startedAt).Seconds())

	// HTTP metrics
	b.WriteString("# HELP fleetdeck_http_requests_total Total HTTP requests served.\n")
	b.WriteString("# TYPE fleetdeck_http_requests_total counter\n")
	fmt.Fprintf(&b, "fleetdeck_http_requests_total %d\n", s.metrics.httpRequestsTotal.Load())

	b.WriteString("# HELP fleetdeck_http_request_errors_total Total HTTP requests that returned errors.\n")
	b.WriteString("# TYPE fleetdeck_http_request_errors_total counter\n")
	fmt.Fprintf(&b, "fleetdeck_http_request_errors_total %d\n", s.metrics.httpRequestErrors.Load())

	// Deployment metrics
	b.WriteString("# HELP fleetdeck_deployments_total Total deployments triggered.\n")
	b.WriteString("# TYPE fleetdeck_deployments_total counter\n")
	fmt.Fprintf(&b, "fleetdeck_deployments_total %d\n", s.metrics.deploymentsTotal.Load())

	b.WriteString("# HELP fleetdeck_deployment_failures_total Total failed deployments.\n")
	b.WriteString("# TYPE fleetdeck_deployment_failures_total counter\n")
	fmt.Fprintf(&b, "fleetdeck_deployment_failures_total %d\n", s.metrics.deploymentsFailures.Load())

	// Backup metrics
	b.WriteString("# HELP fleetdeck_backups_total Total backups created.\n")
	b.WriteString("# TYPE fleetdeck_backups_total counter\n")
	fmt.Fprintf(&b, "fleetdeck_backups_total %d\n", s.metrics.backupsTotal.Load())

	// Read cached system metrics
	s.metrics.cacheMu.RLock()
	projects := s.metrics.cachedProjects
	running := s.metrics.cachedRunning
	stopped := s.metrics.cachedStopped
	totalContainers := s.metrics.cachedContainers
	memTotal := s.metrics.cachedMemTotal
	memAvail := s.metrics.cachedMemAvail
	diskTotal := s.metrics.cachedDiskTotal
	diskUsed := s.metrics.cachedDiskUsed
	traefikUp := s.metrics.cachedTraefikUp
	s.metrics.cacheMu.RUnlock()

	// Project metrics
	b.WriteString("# HELP fleetdeck_projects_total Total number of projects.\n")
	b.WriteString("# TYPE fleetdeck_projects_total gauge\n")
	fmt.Fprintf(&b, "fleetdeck_projects_total %d\n", projects)

	b.WriteString("# HELP fleetdeck_projects_running Number of running projects.\n")
	b.WriteString("# TYPE fleetdeck_projects_running gauge\n")
	fmt.Fprintf(&b, "fleetdeck_projects_running %d\n", running)

	b.WriteString("# HELP fleetdeck_projects_stopped Number of stopped projects.\n")
	b.WriteString("# TYPE fleetdeck_projects_stopped gauge\n")
	fmt.Fprintf(&b, "fleetdeck_projects_stopped %d\n", stopped)

	b.WriteString("# HELP fleetdeck_containers_total Total Docker containers across all projects.\n")
	b.WriteString("# TYPE fleetdeck_containers_total gauge\n")
	fmt.Fprintf(&b, "fleetdeck_containers_total %d\n", totalContainers)

	// System metrics
	b.WriteString("# HELP fleetdeck_cpu_count Number of CPUs available.\n")
	b.WriteString("# TYPE fleetdeck_cpu_count gauge\n")
	fmt.Fprintf(&b, "fleetdeck_cpu_count %d\n", runtime.NumCPU())

	b.WriteString("# HELP fleetdeck_goroutines Number of active goroutines.\n")
	b.WriteString("# TYPE fleetdeck_goroutines gauge\n")
	fmt.Fprintf(&b, "fleetdeck_goroutines %d\n", runtime.NumGoroutine())

	// Memory
	if memTotal > 0 {
		b.WriteString("# HELP fleetdeck_memory_total_bytes Total system memory in bytes.\n")
		b.WriteString("# TYPE fleetdeck_memory_total_bytes gauge\n")
		fmt.Fprintf(&b, "fleetdeck_memory_total_bytes %d\n", memTotal)

		b.WriteString("# HELP fleetdeck_memory_available_bytes Available system memory in bytes.\n")
		b.WriteString("# TYPE fleetdeck_memory_available_bytes gauge\n")
		fmt.Fprintf(&b, "fleetdeck_memory_available_bytes %d\n", memAvail)
	}

	// Disk usage
	if diskTotal > 0 {
		b.WriteString("# HELP fleetdeck_disk_total_bytes Total disk space in bytes.\n")
		b.WriteString("# TYPE fleetdeck_disk_total_bytes gauge\n")
		fmt.Fprintf(&b, "fleetdeck_disk_total_bytes %d\n", diskTotal)

		b.WriteString("# HELP fleetdeck_disk_used_bytes Used disk space in bytes.\n")
		b.WriteString("# TYPE fleetdeck_disk_used_bytes gauge\n")
		fmt.Fprintf(&b, "fleetdeck_disk_used_bytes %d\n", diskUsed)
	}

	// Traefik status
	b.WriteString("# HELP fleetdeck_traefik_up Whether Traefik reverse proxy is running.\n")
	b.WriteString("# TYPE fleetdeck_traefik_up gauge\n")
	fmt.Fprintf(&b, "fleetdeck_traefik_up %d\n", traefikUp)

	w.Write([]byte(b.String()))
}

// parseMemInfo reads /proc/meminfo for total and available memory.
func parseMemInfo() (total, available int64) {
	out, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		// Values in /proc/meminfo are in kB
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = val * 1024
		case strings.HasPrefix(line, "MemAvailable:"):
			available = val * 1024
		}
	}
	return total, available
}

// parseDiskUsage runs df to get disk usage for a path.
func parseDiskUsage(path string) (total, used int64) {
	out, err := exec.Command("df", "-B1", path).Output()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, 0
	}
	total, _ = strconv.ParseInt(fields[1], 10, 64)
	used, _ = strconv.ParseInt(fields[2], 10, 64)
	return total, used
}
