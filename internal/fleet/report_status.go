package fleet

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// ReportRun is the entire information disclosed to an independent delivery monitor.
type ReportRun struct {
	LastEvaluation time.Time `json:"last_evaluation"`
	Pending        int       `json:"pending"`
}

type ReportRuns struct {
	Daily  ReportRun `json:"daily"`
	Weekly ReportRun `json:"weekly"`
}

func readReportRuns(path string, now time.Time) (ReportRuns, error) {
	const limit = 4096
	var runs ReportRuns
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0027 != 0 || info.Size() > limit {
		return runs, fmt.Errorf("report status unavailable")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return runs, err
	}
	defer f.Close()
	current, err := f.Stat()
	if err != nil || !os.SameFile(info, current) || current.Size() > limit || current.Mode().Perm()&0027 != 0 {
		return runs, fmt.Errorf("report status changed")
	}
	return DecodeReportRuns(f, now)
}

// DecodeReportRuns accepts only the small reporting projection. Both ends
// evaluate the timestamps against their own clock.
func DecodeReportRuns(reader io.Reader, now time.Time) (ReportRuns, error) {
	var runs ReportRuns
	const limit = 4096
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || len(data) > limit {
		return runs, fmt.Errorf("invalid report status size")
	}
	// Pointers distinguish a missing count from an explicitly reported zero.
	type runInput struct {
		LastEvaluation time.Time `json:"last_evaluation"`
		Pending        *int      `json:"pending"`
	}
	var input struct{ Daily, Weekly runInput }
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF {
		return runs, fmt.Errorf("invalid report status")
	}
	for _, value := range []runInput{input.Daily, input.Weekly} {
		if value.Pending == nil || *value.Pending < 0 || *value.Pending > 2 || value.LastEvaluation.IsZero() || value.LastEvaluation.After(now.Add(30*time.Second)) {
			return runs, fmt.Errorf("invalid reporting run")
		}
	}
	runs.Daily = ReportRun{input.Daily.LastEvaluation.UTC(), *input.Daily.Pending}
	runs.Weekly = ReportRun{input.Weekly.LastEvaluation.UTC(), *input.Weekly.Pending}
	return runs, nil
}

// WithReportStatus adds a distinct read-only route. Snapshot clients and
// report-status clients are authorized independently, after mutual TLS.
func WithReportStatus(snapshot http.Handler, path string, clients map[[32]byte]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/report-status" {
			snapshot.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || !clients[sha256.Sum256(r.TLS.PeerCertificates[0].Raw)] {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		runs, err := readReportRuns(path, time.Now())
		if err != nil {
			http.Error(w, "report status unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	})
}
