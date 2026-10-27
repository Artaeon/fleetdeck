package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type telegramTransport func(*http.Request) (*http.Response, error)

func (f telegramTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const testTelegramToken = "123456789:abcdefghijklmnopqrstuvwxyz_12345"

func TestTelegramDeliveryAndPrivacy(t *testing.T) {
	p := &TelegramProvider{BotToken: testTelegramToken, ChatID: -123456, ThreadID: 42}
	p.client = &http.Client{Transport: telegramTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.telegram.org" || r.Method != "POST" {
			t.Fatal("unexpected destination")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["chat_id"] != float64(-123456) || payload["message_thread_id"] != float64(42) || payload["protect_content"] != true || payload["parse_mode"] != nil {
			t.Fatal("unsafe message policy")
		}
		if payload["link_preview_options"].(map[string]any)["is_disabled"] != true {
			t.Fatal("link previews enabled")
		}
		message := payload["text"].(string)
		if strings.Contains(message, testTelegramToken) || len([]rune(message)) > 4096 || !strings.Contains(message, "[truncated]") {
			t.Fatal("unsafe message content/size")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})}
	if err := p.Send(Alert{Level: "critical", Title: "<plain>", Message: strings.Repeat("🔒", 5000), Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	p.client.Transport = telegramTransport(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("transport %s", r.URL) })
	if err := p.Send(Alert{}); err == nil || strings.Contains(err.Error(), testTelegramToken) {
		t.Fatal("transport error leaks credentials")
	}
	for _, test := range []struct {
		status int
		body   string
	}{{429, testTelegramToken}, {200, `{"ok":false,"description":"` + testTelegramToken + `"}`}, {200, `invalid`}} {
		p.client.Transport = telegramTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header)}, nil
		})
		if err := p.Send(Alert{}); err == nil || strings.Contains(err.Error(), testTelegramToken) {
			t.Fatal("response error missing or leaks credentials")
		}
	}
}

func TestTelegramPrivateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telegram.json")
	valid := `{"bot_token":"` + testTelegramToken + `","chat_id":12345}`
	for _, test := range []struct {
		body string
		mode os.FileMode
		ok   bool
	}{{valid, 0600, true}, {valid, 0644, false}, {valid + valid, 0600, false}, {`{"bot_token":"bad","chat_id":1}`, 0600, false}, {`{"bot_token":"` + testTelegramToken + `","chat_id":0}`, 0600, false}, {`{"bot_token":"` + testTelegramToken + `","chat_id":1,"api_url":"https://example.com"}`, 0600, false}} {
		if err := os.WriteFile(path, []byte(test.body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, test.mode); err != nil {
			t.Fatal(err)
		}
		_, err := LoadTelegramProvider(path)
		if (err == nil) != test.ok {
			t.Fatalf("config validation: %v", err)
		}
	}
}
