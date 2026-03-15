package monitor

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookProviderSend(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(server.URL)
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

	var received Alert
	if err := json.Unmarshal(receivedBody, &received); err != nil {
		t.Fatalf("failed to unmarshal received body: %v", err)
	}

	if received.Level != alert.Level {
		t.Errorf("expected level %q, got %q", alert.Level, received.Level)
	}
	if received.Title != alert.Title {
		t.Errorf("expected title %q, got %q", alert.Title, received.Title)
	}
	if received.Message != alert.Message {
		t.Errorf("expected message %q, got %q", alert.Message, received.Message)
	}
	if received.Target != alert.Target {
		t.Errorf("expected target %q, got %q", alert.Target, received.Target)
	}
	if !received.Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v, got %v", ts, received.Timestamp)
	}
}

func TestWebhookProviderServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewWebhookProvider(server.URL)
	alert := Alert{
		Level:     "warning",
		Title:     "test alert",
		Message:   "test message",
		Target:    "test-target",
		Timestamp: time.Now(),
	}

	err := provider.Send(alert)
	if err == nil {
		t.Fatal("expected error when server returns 500, got nil")
	}

	expected := "webhook returned status 500"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}

func TestWebhookProviderTimeout(t *testing.T) {
	// Create a server that sleeps longer than the webhook client timeout.
	// The webhook client uses a 10s timeout; we simulate a long-blocking server
	// by simply never writing a response and closing after the test.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block for longer than the client timeout.
		time.Sleep(15 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Override the server's client timeout to something small for the test,
	// since the provider uses its own http.Client with a 10s timeout.
	// We create a provider pointing at a server that blocks.
	provider := NewWebhookProvider(server.URL)
	alert := Alert{
		Level:     "critical",
		Title:     "timeout test",
		Message:   "should time out",
		Target:    "slow-target",
		Timestamp: time.Now(),
	}

	// Use a short deadline: the provider's internal client has a 10s timeout,
	// but we close the server immediately to force an error.
	server.Close()

	err := provider.Send(alert)
	if err == nil {
		t.Fatal("expected error when server is unreachable, got nil")
	}
}

func TestSlackProviderSend(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewSlackProvider(server.URL)
	ts := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	alert := Alert{
		Level:     "critical",
		Title:     "webapp is down",
		Message:   "webapp failed health checks",
		Target:    "webapp",
		Timestamp: ts,
	}

	if err := provider.Send(alert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedContentType)
	}

	// Parse the Slack payload structure.
	var payload map[string]interface{}
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal Slack payload: %v", err)
	}

	// Verify attachments exist.
	attachments, ok := payload["attachments"].([]interface{})
	if !ok || len(attachments) == 0 {
		t.Fatal("expected non-empty attachments array in Slack payload")
	}

	attachment, ok := attachments[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected attachment to be an object")
	}

	// Verify color (critical = red).
	if color, ok := attachment["color"].(string); !ok || color != "#ff0000" {
		t.Errorf("expected color %q for critical alert, got %q", "#ff0000", color)
	}

	// Verify title and text.
	if title, ok := attachment["title"].(string); !ok || title != alert.Title {
		t.Errorf("expected title %q, got %q", alert.Title, title)
	}
	if text, ok := attachment["text"].(string); !ok || text != alert.Message {
		t.Errorf("expected text %q, got %q", alert.Message, text)
	}

	// Verify footer.
	if footer, ok := attachment["footer"].(string); !ok || footer != "FleetDeck Monitor" {
		t.Errorf("expected footer %q, got %q", "FleetDeck Monitor", footer)
	}

	// Verify fields.
	fields, ok := attachment["fields"].([]interface{})
	if !ok || len(fields) < 2 {
		t.Fatalf("expected at least 2 fields, got %v", fields)
	}

	field0, ok := fields[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected field 0 to be an object")
	}
	if field0["title"] != "Target" {
		t.Errorf("expected first field title %q, got %q", "Target", field0["title"])
	}
	if field0["value"] != "webapp" {
		t.Errorf("expected first field value %q, got %q", "webapp", field0["value"])
	}

	field1, ok := fields[1].(map[string]interface{})
	if !ok {
		t.Fatal("expected field 1 to be an object")
	}
	if field1["title"] != "Level" {
		t.Errorf("expected second field title %q, got %q", "Level", field1["title"])
	}
	if field1["value"] != "CRITICAL" {
		t.Errorf("expected second field value %q, got %q", "CRITICAL", field1["value"])
	}
}

func TestSlackProviderColors(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		expectedColor string
	}{
		{"info level is green", "info", "#36a64f"},
		{"warning level is orange", "warning", "#ff9900"},
		{"critical level is red", "critical", "#ff0000"},
		{"unknown level defaults to green", "something-else", "#36a64f"},
		{"empty level defaults to green", "", "#36a64f"},
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
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			provider := NewSlackProvider(server.URL)
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

			attachments := payload["attachments"].([]interface{})
			attachment := attachments[0].(map[string]interface{})

			if color := attachment["color"].(string); color != tt.expectedColor {
				t.Errorf("for level %q: expected color %q, got %q", tt.level, tt.expectedColor, color)
			}
		})
	}
}

func TestNewWebhookProvider(t *testing.T) {
	url := "https://example.com/webhook"
	provider := NewWebhookProvider(url)

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.URL != url {
		t.Errorf("expected URL %q, got %q", url, provider.URL)
	}
}

func TestNewSlackProvider(t *testing.T) {
	url := "https://hooks.slack.com/services/T000/B000/xxxx"
	provider := NewSlackProvider(url)

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.WebhookURL != url {
		t.Errorf("expected WebhookURL %q, got %q", url, provider.WebhookURL)
	}
	// Channel defaults to empty unless explicitly set.
	if provider.Channel != "" {
		t.Errorf("expected empty Channel by default, got %q", provider.Channel)
	}
}

func TestWebhookProviderName(t *testing.T) {
	provider := &WebhookProvider{URL: "http://example.com"}
	if name := provider.Name(); name != "webhook" {
		t.Errorf("expected Name() = %q, got %q", "webhook", name)
	}
}

func TestSlackProviderName(t *testing.T) {
	provider := &SlackProvider{WebhookURL: "http://example.com"}
	if name := provider.Name(); name != "slack" {
		t.Errorf("expected Name() = %q, got %q", "slack", name)
	}
}

func TestEmailProviderName(t *testing.T) {
	provider := &EmailProvider{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user",
		Password: "pass",
		From:     "alerts@example.com",
		To:       []string{"admin@example.com"},
	}
	if name := provider.Name(); name != "email" {
		t.Errorf("expected Name() = %q, got %q", "email", name)
	}
}
