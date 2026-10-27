package monitor

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestChallengeNeverProvesApplicationHealth(t *testing.T) {
	for _, tc := range []struct {
		name             string
		status, expected int
		marker           string
		want             string
	}{
		{"challenge", 403, 200, "challenge", "blocked"},
		{"expected forbidden", 403, 403, "challenge", "blocked"},
		{"challenge with success code", 200, 200, "challenge", "blocked"},
		{"plain forbidden", 403, 200, "", "unhealthy"},
		{"unknown marker", 403, 200, "other", "unhealthy"},
		{"normal success", 200, 200, "", "healthy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cf-Mitigated", tc.marker)
				w.Header().Set("Set-Cookie", "private-cookie")
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			target := Target{Name: "web", URL: srv.URL, ExpectedStatus: tc.expected}
			result := New(nil, nil, 1).CheckOnce(target)
			if result.HealthStatus() != tc.want || result.Healthy != (tc.want == "healthy") {
				t.Fatalf("unexpected result: %+v", result)
			}
			path := filepath.Join(t.TempDir(), "state.json")
			if err := SaveState(path, []Target{target}, []CheckResult{result}); err != nil {
				t.Fatal(err)
			}
			restored, err := LoadState(path)
			if err != nil || restored.Results["web"].HealthStatus() != tc.want {
				t.Fatalf("state lost diagnosis: %v", err)
			}
		})
	}
}

func TestChallengeRedirectIsNotFollowed(t *testing.T) {
	reached := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer destination.Close()
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer edge.Close()
	result := New(nil, nil, 1).CheckOnce(Target{Name: "web", URL: edge.URL})
	if reached || result.HealthStatus() != "blocked" || result.StatusCode != http.StatusFound {
		t.Fatalf("challenge hidden by redirect: %+v", result)
	}
}
