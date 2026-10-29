// Package secretscan checks source content locally using pinned Gitleaks rules.
// It never verifies credentials online or returns matched values/source lines.
package secretscan

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/logging"
)

const RulesVersion = "gitleaks/v8.30.1"
const MaxFileBytes int64 = 16 << 20
const MaxBytes int64 = 1 << 30
const MaxFiles = 100000
const MaxFindings = 1000

type Finding struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Rule string `json:"rule"`
}

type Report struct {
	Scanner  string    `json:"scanner"`
	Scope    string    `json:"scope"`
	Status   string    `json:"status"`
	Code     string    `json:"code,omitempty"`
	Files    int       `json:"files"`
	Bytes    int64     `json:"bytes"`
	Findings []Finding `json:"findings"`
}

// RequireClean deliberately treats an incomplete scan differently from a pass.
func (r Report) RequireClean() error {
	if r.Status == "unknown" {
		return fmt.Errorf("secret scan incomplete (%s); deployment blocked", r.Code)
	}
	if r.Status != "checked" || len(r.Findings) > 0 {
		return fmt.Errorf("potential secrets detected; deployment blocked; run 'fleetdeck security secrets' for file, line and rule metadata")
	}
	return nil
}

var rulesOnce sync.Once
var rules config.Config
var rulesErr error

func init() {
	// Gitleaks trace logs can contain unredacted matches. Disable its dedicated
	// logger once, before any scanners exist; never mutate global logging per scan.
	logging.Logger = zerolog.Nop()
}

func newScan(ctx context.Context) *scan {
	rulesOnce.Do(func() {
		// Never read repository config, user config, environment or allowlists.
		v := viper.New()
		v.SetConfigType("toml")
		if rulesErr = v.ReadConfig(strings.NewReader(config.DefaultConfig)); rulesErr != nil {
			return
		}
		var source config.ViperConfig
		if rulesErr = v.Unmarshal(&source); rulesErr == nil {
			rules, rulesErr = source.Translate()
		}
	})
	s := &scan{ctx: ctx, report: Report{Scanner: RulesVersion, Scope: "source-content", Status: "checked", Findings: []Finding{}}, seen: map[string]bool{}}
	if rulesErr != nil {
		s.fail("rules_unavailable")
		return s
	}
	s.detector = detect.NewDetectorContext(ctx, rules)
	s.detector.IgnoreGitleaksAllow = true
	s.detector.Redact = 100
	s.detector.MaxDecodeDepth = 3
	return s
}

type scan struct {
	ctx      context.Context
	detector *detect.Detector
	report   Report
	seen     map[string]bool
	total    int64
}

func (s *scan) fail(code string) { s.report.Status, s.report.Code = "unknown", code }

func validName(name string) bool {
	return filepath.IsLocal(name) && name != "." && filepath.ToSlash(filepath.Clean(name)) == name && !strings.ContainsAny(name, "\\:\x00\r\n")
}

func archiveContent(name string, b []byte) bool {
	for _, ext := range []string{".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".zst", ".7z", ".rar"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			return true
		}
	}
	for _, magic := range [][]byte{{'P', 'K', 3, 4}, {0x1f, 0x8b}, {'B', 'Z', 'h'}, {0xfd, '7', 'z', 'X', 'Z', 0}, {'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}, {'R', 'a', 'r', '!'}, {0x28, 0xb5, 0x2f, 0xfd}} {
		if bytes.HasPrefix(b, magic) {
			return true
		}
	}
	return len(b) >= 262 && string(b[257:262]) == "ustar"
}

func (s *scan) add(name string, size int64, input io.Reader) bool {
	if s.ctx.Err() != nil {
		s.fail("cancelled")
		return false
	}
	if !validName(name) || s.seen[name] {
		s.fail("invalid_source_entry")
		return false
	}
	s.seen[name] = true
	s.total += size
	if size < 0 || size > MaxFileBytes || s.total > MaxBytes || len(s.seen) > MaxFiles {
		s.fail("source_limit_exceeded")
		return false
	}
	// The only credential payload explicitly permitted by the source protocol.
	// This is excluded from builds and preserves an existing server .env.
	if name == ".env" {
		return true
	}
	b, err := io.ReadAll(io.LimitReader(input, MaxFileBytes+1))
	if err != nil || int64(len(b)) != size {
		s.fail("source_read_failed")
		return false
	}
	if archiveContent(name, b) {
		s.fail("archive_content_unsupported")
		return false
	}
	s.report.Files++
	s.report.Bytes += size
	// Content-only fragments avoid filename allowlists and filesystem traversal
	// that can silently skip files. Scan raw bytes, including binary data. The
	// real relative path is added only to our value-free result below.
	findings := s.detector.Detect(detect.Fragment{Raw: string(b), StartLine: 1})
	for _, f := range findings {
		if len(s.report.Findings) >= MaxFindings {
			s.fail("finding_limit_exceeded")
			return false
		}
		s.report.Findings = append(s.report.Findings, Finding{Path: name, Line: f.StartLine, Rule: f.RuleID})
	}
	if s.ctx.Err() != nil {
		s.fail("cancelled")
		return false
	}
	return true
}

func (s *scan) finish() Report {
	if s.ctx.Err() != nil {
		s.fail("cancelled")
	}
	if s.report.Status != "unknown" && len(s.report.Findings) > 0 {
		s.report.Status = "blocked"
	}
	sort.Slice(s.report.Findings, func(i, j int) bool {
		a, b := s.report.Findings[i], s.report.Findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Rule < b.Rule
	})
	return s.report
}

// Archive checks the exact materialized source bundle, before network transfer.
func Archive(ctx context.Context, input io.Reader) Report {
	s := newScan(ctx)
	if s.detector == nil {
		return s.finish()
	}
	tr := tar.NewReader(&io.LimitedReader{R: input, N: MaxBytes + MaxFiles*1024 + 1})
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.fail("invalid_source_archive")
			break
		}
		if h.Typeflag != tar.TypeReg {
			s.fail("invalid_source_entry")
			break
		}
		if !s.add(h.Name, h.Size, tr) {
			break
		}
	}
	if len(s.seen) == 0 {
		s.fail("empty_source_archive")
	}
	return s.finish()
}

// Files checks only extracted incoming files, before server secrets are added.
// Staging must remain private to the receiver until checking and applying finish.
func Files(ctx context.Context, dir string, names []string) Report {
	s := newScan(ctx)
	if s.detector == nil {
		return s.finish()
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		s.fail("source_read_failed")
		return s.finish()
	}
	defer root.Close()
	for _, name := range names {
		if !validName(name) {
			s.fail("invalid_source_entry")
			break
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			s.fail("source_read_failed")
			break
		}
		f, err := root.Open(name)
		if err != nil {
			s.fail("source_read_failed")
			break
		}
		current, err := f.Stat()
		if err != nil || !os.SameFile(info, current) {
			f.Close()
			s.fail("source_read_failed")
			break
		}
		ok := s.add(name, info.Size(), f)
		f.Close()
		if !ok {
			break
		}
	}
	if len(names) == 0 {
		s.fail("empty_source_archive")
	}
	return s.finish()
}
