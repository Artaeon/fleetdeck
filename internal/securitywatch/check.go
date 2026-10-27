// Package securitywatch reports update and vulnerability evidence. It never
// upgrades software or restarts workloads, and incomplete checks stay unknown.
package securitywatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Check struct {
	Status    string `json:"status"`
	Code      string `json:"code,omitempty"`
	Installed string `json:"installed,omitempty"`
	Latest    string `json:"latest,omitempty"`
	Count     int    `json:"count,omitempty"`
}
type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
}
type Image struct {
	ID       string    `json:"id"`
	Status   string    `json:"status"`
	Code     string    `json:"code,omitempty"`
	Findings []Finding `json:"findings"`
}
type Container struct {
	ID       string    `json:"id"`
	ImageID  string    `json:"image_id"`
	Findings []Finding `json:"findings"`
}
type Report struct {
	CheckedAt  time.Time   `json:"checked_at"`
	Release    Check       `json:"release"`
	Host       Check       `json:"host"`
	Inventory  Check       `json:"inventory"`
	Images     []Image     `json:"images"`
	Containers []Container `json:"containers"`
}
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type ExecRunner struct{}
type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 32<<20 {
		return 0, fmt.Errorf("scanner output exceeds 32 MiB")
	}
	return b.Buffer.Write(p)
}
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out limitedOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("security tool failed: %w", err)
	}
	return out.Bytes(), nil
}

type Checker struct {
	Runner      Runner
	HTTP        *http.Client
	API         string
	Now         func() time.Time
	ScanTimeout time.Duration
	HostOS      string
	IndexStamp  string
}

func New() *Checker {
	return &Checker{Runner: ExecRunner{}, HTTP: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, API: "https://api.github.com", Now: time.Now, ScanTimeout: 5 * time.Minute, HostOS: runtime.GOOS, IndexStamp: "/var/lib/apt/periodic/update-success-stamp"}
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var imageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var containerIDPattern = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
var releasePattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

func (c *Checker) Run(ctx context.Context, repository, installed string) Report {
	report := Report{CheckedAt: c.Now().UTC(), Release: c.release(ctx, repository, installed), Host: c.host(ctx), Images: []Image{}, Containers: []Container{}}
	ids, err := c.Runner.Run(ctx, "docker", "ps", "--quiet")
	if err != nil {
		report.Inventory = Check{Status: "unknown", Code: "docker_inventory_unavailable"}
		return report
	}
	values := strings.Fields(string(ids))
	if len(values) > 200 {
		report.Inventory = Check{Status: "unknown", Code: "container_limit_exceeded"}
		return report
	}
	for _, id := range values {
		if !containerIDPattern.MatchString(id) {
			report.Inventory = Check{Status: "unknown", Code: "invalid_container_identity"}
			return report
		}
	}
	report.Inventory = Check{Status: "checked", Count: len(values)}
	if len(values) == 0 {
		return report
	}
	// Request only fields required for the audit. Never retrieve container Env.
	format := `{"id":{{json .Id}},"image":{{json .Image}},"user":{{json .Config.User}},"privileged":{{json .HostConfig.Privileged}},"network":{{json .HostConfig.NetworkMode}},"pid":{{json .HostConfig.PidMode}},"security":{{json .HostConfig.SecurityOpt}},"caps":{{json .HostConfig.CapAdd}},"mounts":{{json .Mounts}}}`
	args := append([]string{"inspect", "--format", format}, values...)
	data, err := c.Runner.Run(ctx, "docker", args...)
	if err != nil {
		report.Inventory = Check{Status: "unknown", Code: "container_inspection_failed"}
		return report
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	images := map[string]bool{}
	for {
		var item struct {
			ID         string   `json:"id"`
			Image      string   `json:"image"`
			User       string   `json:"user"`
			Privileged bool     `json:"privileged"`
			Network    string   `json:"network"`
			PID        string   `json:"pid"`
			Security   []string `json:"security"`
			Caps       []string `json:"caps"`
			Mounts     []struct {
				Source      string
				Destination string
			} `json:"mounts"`
		}
		err = decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil || !containerIDPattern.MatchString(item.ID) || !imageIDPattern.MatchString(item.Image) {
			report.Inventory = Check{Status: "unknown", Code: "invalid_container_inspection"}
			return report
		}
		container := Container{ID: item.ID, ImageID: item.Image, Findings: []Finding{}}
		add := func(id, severity string) {
			container.Findings = append(container.Findings, Finding{ID: id, Severity: severity})
		}
		user := strings.Split(item.User, ":")[0]
		if user == "" || user == "0" || user == "root" {
			add("root_or_image_default_startup_user", "warning")
		}
		nnp := false
		for _, opt := range item.Security {
			if opt == "no-new-privileges" || opt == "no-new-privileges:true" || opt == "no-new-privileges=true" {
				nnp = true
			}
			if strings.Contains(opt, "unconfined") {
				add("runtime_confinement_disabled", "critical")
			}
		}
		if !nnp {
			add("no_new_privileges_missing", "warning")
		}
		if item.Privileged {
			add("privileged_container", "critical")
		}
		if item.Network == "host" || item.PID == "host" {
			add("shared_host_namespace", "warning")
		}
		if len(item.Caps) > 0 {
			add("additional_linux_capabilities", "warning")
		}
		for _, m := range item.Mounts {
			if strings.HasSuffix(m.Source, "/docker.sock") || strings.HasSuffix(m.Destination, "/docker.sock") {
				add("docker_socket_mounted", "critical")
			}
		}
		sort.Slice(container.Findings, func(i, j int) bool { return container.Findings[i].ID < container.Findings[j].ID })
		report.Containers = append(report.Containers, container)
		images[item.Image] = true
	}
	if len(report.Containers) != len(values) {
		report.Inventory = Check{Status: "unknown", Code: "incomplete_container_inventory"}
	}
	list := make([]string, 0, len(images))
	for id := range images {
		list = append(list, id)
	}
	sort.Strings(list)
	if len(list) > 100 {
		report.Inventory = Check{Status: "unknown", Code: "image_limit_exceeded"}
		return report
	}
	for _, id := range list {
		report.Images = append(report.Images, c.scan(ctx, id))
	}
	sort.Slice(report.Containers, func(i, j int) bool { return report.Containers[i].ID < report.Containers[j].ID })
	return report
}
func (c *Checker) release(ctx context.Context, repo, installed string) Check {
	result := Check{Status: "unknown", Installed: installed}
	if !repositoryPattern.MatchString(repo) {
		result.Code = "invalid_release_repository"
		return result
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.API+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		result.Code = "release_lookup_failed"
		return result
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "FleetDeck-security-watch")
	response, err := c.HTTP.Do(req)
	if err != nil {
		result.Code = "release_lookup_failed"
		return result
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		result.Code = "release_lookup_unavailable"
		return result
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		result.Code = "invalid_release_response"
		return result
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.Unmarshal(data, &release) != nil || !releasePattern.MatchString(release.Tag) || release.Draft || release.Prerelease {
		result.Code = "invalid_release_response"
		return result
	}
	result.Latest = release.Tag
	current := releasePattern.FindStringSubmatch(installed)
	latest := releasePattern.FindStringSubmatch(release.Tag)
	if current == nil {
		result.Code = "unversioned_local_build"
		return result
	}
	result.Status = "up_to_date"
	for i := 1; i <= 3; i++ {
		a, e1 := strconv.ParseUint(current[i], 10, 64)
		b, e2 := strconv.ParseUint(latest[i], 10, 64)
		if e1 != nil || e2 != nil {
			result.Status = "unknown"
			result.Code = "invalid_version"
			return result
		}
		if a < b {
			result.Status = "update_available"
			return result
		}
		if a > b {
			return result
		}
	}
	if current[4] != "" && latest[4] == "" {
		result.Status = "update_available"
	}
	return result
}
func (c *Checker) scan(ctx context.Context, id string) Image {
	result := Image{ID: id, Status: "unknown", Findings: []Finding{}}
	ctx, cancel := context.WithTimeout(ctx, c.ScanTimeout)
	defer cancel()
	data, err := c.Runner.Run(ctx, "trivy", "image", "--quiet", "--scanners", "vuln", "--format", "json", "--severity", "HIGH,CRITICAL", "--exit-code", "0", "--image-src", "docker", "--timeout", c.ScanTimeout.String(), "--", id)
	if err != nil {
		result.Code = "vulnerability_scan_failed_or_unavailable"
		return result
	}
	var scan struct {
		SchemaVersion int
		ArtifactType  string
		Metadata      struct {
			ImageID string
			OS      *struct{ EOSL bool }
		}
		Results []struct {
			Vulnerabilities []struct {
				ID       string `json:"VulnerabilityID"`
				Severity string
			}
		}
	}
	if json.Unmarshal(data, &scan) != nil || scan.SchemaVersion != 2 || scan.ArtifactType != "container_image" || scan.Metadata.ImageID != id {
		result.Code = "invalid_or_mismatched_scan"
		return result
	}
	seen := map[string]bool{}
	for _, r := range scan.Results {
		for _, v := range r.Vulnerabilities {
			if v.Severity != "HIGH" && v.Severity != "CRITICAL" {
				continue
			}
			if len(v.ID) > 160 || !regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`).MatchString(v.ID) {
				result.Code = "invalid_vulnerability_identity"
				return result
			}
			key := v.ID + v.Severity
			if seen[key] {
				continue
			}
			seen[key] = true
			result.Findings = append(result.Findings, Finding{ID: v.ID, Severity: strings.ToLower(v.Severity)})
		}
	}
	result.Status = "checked"
	if scan.Metadata.OS != nil && scan.Metadata.OS.EOSL {
		result.Findings = append(result.Findings, Finding{ID: "unsupported_os_release", Severity: "warning"})
	}
	sort.Slice(result.Findings, func(i, j int) bool { return result.Findings[i].ID < result.Findings[j].ID })
	return result
}
func (c *Checker) host(ctx context.Context) Check {
	if c.HostOS != "linux" {
		return Check{Status: "not_applicable", Code: "linux_host_check_only"}
	}
	data, err := c.Runner.Run(ctx, "apt-get", "--simulate", "-o", "Debug::NoLocking=1", "upgrade")
	if err != nil {
		return Check{Status: "unknown", Code: "apt_unavailable_or_failed"}
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Inst ") {
			count++
		}
	}
	info, err := os.Stat(c.IndexStamp)
	if err != nil || c.Now().Sub(info.ModTime()) > 48*time.Hour || info.ModTime().After(c.Now().Add(time.Minute)) {
		return Check{Status: "unknown", Code: "apt_index_freshness_unverified", Count: count}
	}
	status := "up_to_date"
	if count > 0 {
		status = "update_available"
	}
	return Check{Status: status, Count: count}
}
func (r Report) Counts() (critical, warning, unknown int) {
	for _, c := range []Check{r.Release, r.Host, r.Inventory} {
		if c.Status == "unknown" {
			unknown++
		}
		if c.Status == "update_available" {
			warning++
		}
	}
	for _, image := range r.Images {
		if image.Status == "unknown" {
			unknown++
		}
		for _, f := range image.Findings {
			if f.Severity == "critical" {
				critical++
			} else {
				warning++
			}
		}
	}
	for _, container := range r.Containers {
		for _, f := range container.Findings {
			if f.Severity == "critical" {
				critical++
			} else {
				warning++
			}
		}
	}
	return
}
