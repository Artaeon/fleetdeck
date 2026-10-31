package securitywatch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSourceTrackingRequiresExactRefAndFastForwardHistory(t *testing.T) {
	old, next := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, tc := range []struct {
		name, ref, kind, sha, comparison, status, code string
	}{
		{"same", "refs/heads/main", "commit", old, "", "up_to_date", "source_commit_current"},
		{"newer", "refs/heads/main", "commit", next, fmt.Sprintf(`{"status":"ahead","ahead_by":3,"behind_by":0,"base_commit":{"sha":%q},"merge_base_commit":{"sha":%q}}`, old, old), "update_available", "source_commit_update_available"},
		{"different ref", "refs/heads/other", "commit", old, "", "unknown", "invalid_source_branch_response"},
		{"tag object", "refs/heads/main", "tag", old, "", "unknown", "invalid_source_branch_response"},
		{"short sha", "refs/heads/main", "commit", "aaaaaaa", "", "unknown", "invalid_source_branch_response"},
		{"diverged", "refs/heads/main", "commit", next, `{"status":"diverged","ahead_by":2,"behind_by":1}`, "unknown", "source_history_not_fast_forward"},
		{"ahead local", "refs/heads/main", "commit", next, `{"status":"behind","ahead_by":0,"behind_by":1}`, "unknown", "source_history_not_fast_forward"},
		{"wrong base", "refs/heads/main", "commit", next, fmt.Sprintf(`{"status":"ahead","ahead_by":3,"behind_by":0,"base_commit":{"sha":%q},"merge_base_commit":{"sha":%q}}`, next, old), "unknown", "source_history_not_fast_forward"},
		{"wrong ancestor", "refs/heads/main", "commit", next, fmt.Sprintf(`{"status":"ahead","ahead_by":3,"behind_by":0,"base_commit":{"sha":%q},"merge_base_commit":{"sha":%q}}`, old, next), "unknown", "source_history_not_fast_forward"},
		{"empty comparison", "refs/heads/main", "commit", next, `{}`, "unknown", "source_history_not_fast_forward"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch r.URL.Path {
				case "/repos/example/fleetdeck/git/ref/heads/main":
					fmt.Fprintf(w, `{"ref":%q,"object":{"type":%q,"sha":%q}}`, tc.ref, tc.kind, tc.sha)
				case "/repos/example/fleetdeck/compare/" + old + "..." + next:
					fmt.Fprint(w, tc.comparison)
				default:
					t.Error("unexpected mutable or release lookup", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer api.Close()
			c := New()
			c.API, c.SourceCommit, c.SourceBranch = api.URL, old, "main"
			r := c.release(context.Background(), "example/fleetdeck", "dev")
			if r.Status != tc.status || r.Code != tc.code || r.Installed != old || r.SourceBranch != "main" {
				t.Fatalf("unexpected source report: %+v", r)
			}
			if (tc.comparison == "" && calls != 1) || (tc.comparison != "" && calls != 2) {
				t.Fatal("unexpected request count", calls)
			}
		})
	}
}

func TestSourceTrackingRejectsMissingMetadataAndUnsafeTargetsWithoutNetwork(t *testing.T) {
	for _, tc := range []struct{ version, commit, repo, branch string }{
		{"v1.2.3", strings.Repeat("a", 40), "example/fleetdeck", "main"},
		{"dev", "", "example/fleetdeck", "main"},
		{"dev", "aaaaaaa", "example/fleetdeck", "main"},
		{"dev", strings.Repeat("a", 40) + "-dirty", "example/fleetdeck", "main"},
		{"dev", strings.Repeat("a", 40), "../fleetdeck", "main"},
		{"dev", strings.Repeat("a", 40), "example/fleetdeck", "../main"},
		{"dev", strings.Repeat("a", 40), "example/fleetdeck", "main?token=x"},
		{"dev", strings.Repeat("a", 40), "example/fleetdeck", "main#fragment"},
		{"dev", strings.Repeat("a", 40), "example/fleetdeck", "main.lock"},
	} {
		c := New()
		c.API = ":invalid"
		c.SourceCommit, c.SourceBranch = tc.commit, tc.branch
		r := c.release(context.Background(), tc.repo, tc.version)
		if r.Status != "unknown" || !strings.HasPrefix(r.Code, "invalid_source_") {
			t.Fatal(tc, r)
		}
	}
}

func TestSourceTrackingFailuresStayUnknown(t *testing.T) {
	for _, mode := range []string{"missing", "rate limited", "redirect", "invalid json", "oversized", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "missing":
					w.WriteHeader(404)
				case "rate limited":
					w.WriteHeader(429)
				case "redirect":
					w.Header().Set("Location", "/redirected")
					w.WriteHeader(302)
					if r.URL.Path == "/redirected" {
						t.Error("followed redirect")
					}
				case "oversized":
					fmt.Fprint(w, strings.Repeat(" ", (1<<20)+1))
				default:
					fmt.Fprint(w, "invalid")
				}
			}))
			defer api.Close()
			c := New()
			c.API, c.SourceCommit, c.SourceBranch = api.URL, strings.Repeat("a", 40), "main"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			if r := c.release(ctx, "example/fleetdeck", "dev"); r.Status != "unknown" || r.Code != "source_branch_lookup_unavailable" {
				t.Fatal(r)
			}
		})
	}
}

func TestSourceBranchMayContainSlashes(t *testing.T) {
	sha := strings.Repeat("a", 40)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/example/fleetdeck/git/ref/heads/release/next" {
			t.Fatal(r.URL.Path)
		}
		fmt.Fprintf(w, `{"ref":"refs/heads/release/next","object":{"type":"commit","sha":%q}}`, sha)
	}))
	defer api.Close()
	c := New()
	c.API, c.SourceCommit, c.SourceBranch = api.URL, sha, "release/next"
	if r := c.release(context.Background(), "example/fleetdeck", "dev"); r.Status != "up_to_date" {
		t.Fatal(r)
	}
}
