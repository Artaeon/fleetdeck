package monitor

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustDiscordProvider(t *testing.T, url string) *DiscordProvider {
	t.Helper()
	p, err := NewDiscordProvider(url)
	if err != nil {
		t.Fatalf("NewDiscordProvider(%q): %v", url, err)
	}
	return p
}

func TestDiscordProviderSend(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider := mustDiscordProvider(t, server.URL)
	ts := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	alert := Alert{
		Level:     "critical",
		Title:     "myapp is down",
		Message:   "myapp failed 3 consecutive checks: connection refused",
		Target:    "myapp",
		Timestamp: ts,
	}

	if err := provider.Send(alert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedContentType)
	}

	// Parse the Discord payload structure.
	var payload map[string]interface{}
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal Discord payload: %v", err)
	}

	// Content should be null.
	if payload["content"] != nil {
		t.Errorf("expected content to be null, got %v", payload["content"])
	}

	// Verify embeds exist.
	embeds, ok := payload["embeds"].([]interface{})
	if !ok || len(embeds) == 0 {
		t.Fatal("expected non-empty embeds array in Discord payload")
	}

	embed, ok := embeds[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected embed to be an object")
	}

	// Verify color (critical = red = 16711680).
	if color, ok := embed["color"].(float64); !ok || int(color) != 16711680 {
		t.Errorf("expected color %d for critical alert, got %v", 16711680, embed["color"])
	}

	// Verify title and description.
	if title, ok := embed["title"].(string); !ok || title != alert.Title {
		t.Errorf("expected title %q, got %q", alert.Title, title)
	}
	if desc, ok := embed["description"].(string); !ok || desc != alert.Message {
		t.Errorf("expected description %q, got %q", alert.Message, desc)
	}

	// Verify timestamp.
	if tsStr, ok := embed["timestamp"].(string); !ok || tsStr != "2026-03-15T10:30:00Z" {
		t.Errorf("expected timestamp %q, got %q", "2026-03-15T10:30:00Z", tsStr)
	}

	// Verify footer.
	footer, ok := embed["footer"].(map[string]interface{})
	if !ok {
		t.Fatal("expected footer to be an object")
	}
	if footerText, ok := footer["text"].(string); !ok || footerText != "FleetDeck Monitor" {
		t.Errorf("expected footer text %q, got %q", "FleetDeck Monitor", footerText)
	}

	// Verify fields.
	fields, ok := embed["fields"].([]interface{})
	if !ok || len(fields) < 2 {
		t.Fatalf("expected at least 2 fields, got %v", fields)
	}

	field0, ok := fields[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected field 0 to be an object")
	}
	if field0["name"] != "Target" {
		t.Errorf("expected first field name %q, got %q", "Target", field0["name"])
	}
	if field0["value"] != "myapp" {
		t.Errorf("expected first field value %q, got %q", "myapp", field0["value"])
	}
	if field0["inline"] != true {
		t.Errorf("expected first field inline to be true")
	}

	field1, ok := fields[1].(map[string]interface{})
	if !ok {
		t.Fatal("expected field 1 to be an object")
	}
	if field1["name"] != "Level" {
		t.Errorf("expected second field name %q, got %q", "Level", field1["name"])
	}
	if field1["value"] != "CRITICAL" {
		t.Errorf("expected second field value %q, got %q", "CRITICAL", field1["value"])
	}
}

func TestDiscordProviderColors(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		expectedColor int
	}{
		{"info level is green", "info", 65280},
		{"warning level is orange", "warning", 16744192},
		{"critical level is red", "critical", 16711680},
		{"unknown level defaults to green", "something-else", 65280},
		{"empty level defaults to green", "", 65280},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody []byte

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				receivedBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("failed to read body: %v", err)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			provider := mustDiscordProvider(t, server.URL)
			alert := Alert{
				Level:     tt.level,
				Title:     "color test",
				Message:   "testing color",
				Target:    "test",
				Timestamp: time.Now(),
			}

			if err := provider.Send(alert); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(receivedBody, &payload); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			embeds := payload["embeds"].([]interface{})
			embed := embeds[0].(map[string]interface{})

			if color := int(embed["color"].(float64)); color != tt.expectedColor {
				t.Errorf("for level %q: expected color %d, got %d", tt.level, tt.expectedColor, color)
			}
		})
	}
}

func TestDiscordProviderServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	provider := mustDiscordProvider(t, server.URL)
	alert := Alert{
		Level:     "critical",
		Title:     "test alert",
		Message:   "test message",
		Target:    "test-target",
		Timestamp: time.Now(),
	}

	err := provider.Send(alert)
	if err == nil {
		t.Fatal("expected error when server returns 400, got nil")
	}

	expected := "discord returned status 400"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}

func TestDiscordProviderUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	serverURL := server.URL
	server.Close() // Close immediately to make it unreachable.

	provider := mustDiscordProvider(t, serverURL)
	alert := Alert{
		Level:     "critical",
		Title:     "test alert",
		Message:   "test message",
		Target:    "test-target",
		Timestamp: time.Now(),
	}

	err := provider.Send(alert)
	if err == nil {
		t.Fatal("expected error when server is unreachable, got nil")
	}
}

func TestNewDiscordProvider(t *testing.T) {
	url := "https://discord.com/api/webhooks/123/abc"
	provider, err := NewDiscordProvider(url)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.WebhookURL != url {
		t.Errorf("expected WebhookURL %q, got %q", url, provider.WebhookURL)
	}
}

func TestNewDiscordProviderInvalidURL(t *testing.T) {
	tests := []string{
		"",
		"not-a-url",
		"ftp://example.com",
	}
	for _, url := range tests {
		_, err := NewDiscordProvider(url)
		if err == nil {
			t.Errorf("expected error for URL %q, got nil", url)
		}
	}
}

func TestDiscordProviderName(t *testing.T) {
	provider := &DiscordProvider{WebhookURL: "http://example.com"}
	if name := provider.Name(); name != "discord" {
		t.Errorf("expected Name() = %q, got %q", "discord", name)
	}
}
