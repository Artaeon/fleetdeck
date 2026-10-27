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

func TestChallengeAlertTransitions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		states []string
		levels []string
	}{
		{"below threshold", []string{"blocked"}, nil},
		{"blocked repeated", []string{"blocked", "blocked", "blocked"}, []string{"warning"}},
		{"blocked then failed", []string{"blocked", "blocked", "unhealthy", "unhealthy"}, []string{"warning", "critical"}},
		{"failure then blocked is not recovery", []string{"unhealthy", "unhealthy", "blocked", "blocked"}, []string{"critical", "warning"}},
		{"real recovery", []string{"blocked", "blocked", "healthy"}, []string{"warning", "info"}},
		{"mixed failures still alert", []string{"blocked", "unhealthy", "blocked", "unhealthy"}, []string{"critical"}},
		{"temporary classification change", []string{"blocked", "blocked", "unhealthy", "blocked"}, []string{"warning"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &mockAlertProvider{}
			manager := NewAlertManager([]AlertProvider{provider}, 2)
			for _, status := range tc.states {
				r := makeResult("web", status == "healthy", 200)
				if status == "blocked" {
					r.FailureKind = FailureCloudflareChallenge
					r.StatusCode = 403
				}
				if status == "unhealthy" {
					r.StatusCode = 503
				}
				manager.Process(r)
			}
			alerts := provider.getAlerts()
			if len(alerts) != len(tc.levels) {
				t.Fatalf("got alerts %+v; expected levels %v", alerts, tc.levels)
			}
			for i, want := range tc.levels {
				if alerts[i].Level != want {
					t.Fatalf("alert %d: %+v, want %s", i, alerts[i], want)
				}
			}
		})
	}
}
