package monitor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

type boundProvider struct {
	retryProvider
	destination string
}

func (p *boundProvider) DestinationIdentity() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(p.destination)))
}

func TestPendingAlertsCannotMoveToANewDestination(t *testing.T) {
	path := queuePath(t)
	p := &boundProvider{retryProvider: retryProvider{name: "mail"}, destination: "original"}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := am.Process(makeResult("web", false, 503)); err != nil {
		t.Fatal(err)
	}
	if err := am.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p.destination = "replacement"
	if other, err := NewDurableAlertManager([]AlertProvider{p}, 1, path); err == nil {
		other.Close()
		t.Fatal("outstanding private alerts were rebound to a different recipient")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("refused destination change altered the queue")
	}
	p.destination = "original"
	am, err = NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := am.RetryPending(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := am.Close(); err != nil {
		t.Fatal(err)
	}
	p.destination = "replacement"
	am, err = NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal("destination change after delivery should be allowed:", err)
	}
	defer am.Close()
}

func TestLegacyUnboundBacklogStopsBeforeDelivery(t *testing.T) {
	path := queuePath(t)
	p := &boundProvider{retryProvider: retryProvider{name: "mail"}, destination: "original"}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := am.Process(makeResult("web", false, 503)); err != nil {
		t.Fatal(err)
	}
	if err := am.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	delete(disk, "destinations")
	data, err = json.Marshal(disk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := NewDurableAlertManager([]AlertProvider{p}, 1, path); err == nil {
		other.Close()
		t.Fatal("legacy pending messages were adopted without an established destination")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) || len(p.alerts) != 0 {
		t.Fatal("legacy queue was changed or sent")
	}
}

func TestDestinationMutationDoesNotRedirectRetry(t *testing.T) {
	p := &boundProvider{retryProvider: retryProvider{name: "mail"}, destination: "original"}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, queuePath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	if err := am.Process(makeResult("web", false, 503)); err != nil {
		t.Fatal(err)
	}
	p.destination = "replacement"
	if err := am.RetryPending(time.Now()); err == nil || len(p.alerts) != 0 {
		t.Fatal("mutated provider redirected pending notification")
	}
}

func TestBuiltInDestinationBindings(t *testing.T) {
	for _, tc := range []struct {
		name               string
		first, same, other AlertProvider
	}{
		{"telegram", &TelegramProvider{BotToken: "12345:fixture-old", ChatID: 10}, &TelegramProvider{BotToken: "12345:fixture-new", ChatID: 10}, &TelegramProvider{BotToken: "12345:fixture-new", ChatID: 11}},
		{"telegram-topic", &TelegramProvider{BotToken: "12345:fixture", ChatID: 10, ThreadID: 1}, &TelegramProvider{BotToken: "12345:fixture", ChatID: 10, ThreadID: 1}, &TelegramProvider{BotToken: "12345:fixture", ChatID: 10, ThreadID: 2}},
		{"telegram-bot", &TelegramProvider{BotToken: "12345:fixture", ChatID: 10}, &TelegramProvider{BotToken: "12345:fixture", ChatID: 10}, &TelegramProvider{BotToken: "23456:fixture", ChatID: 10}},
		{"email", &EmailProvider{Host: "smtp.example.com", Username: "fixture", From: "sender@example.com", To: []string{"a@example.com", "b@example.com"}, Password: "old-fixture"}, &EmailProvider{Host: "smtp.example.com", Username: "fixture", From: "Sender <sender@example.com>", To: []string{"b@example.com", "a@example.com"}, Password: "new-fixture"}, &EmailProvider{Host: "smtp.example.com", Username: "fixture", From: "sender@example.com", To: []string{"replacement@example.com"}}},
		{"webhook", &WebhookProvider{URL: "https://one.example.com"}, &WebhookProvider{URL: "https://one.example.com"}, &WebhookProvider{URL: "https://two.example.com"}},
		{"slack", &SlackProvider{WebhookURL: "https://one.example.com", Channel: "first"}, &SlackProvider{WebhookURL: "https://one.example.com", Channel: "first"}, &SlackProvider{WebhookURL: "https://one.example.com", Channel: "second"}},
		{"discord", &DiscordProvider{WebhookURL: "https://one.example.com"}, &DiscordProvider{WebhookURL: "https://one.example.com"}, &DiscordProvider{WebhookURL: "https://two.example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, err := providerDestination(tc.first)
			if err != nil {
				t.Fatal(err)
			}
			same, err := providerDestination(tc.same)
			if err != nil {
				t.Fatal(err)
			}
			other, err := providerDestination(tc.other)
			if err != nil {
				t.Fatal(err)
			}
			if first != same || first == other || !validDestination(first) {
				t.Fatal("binding ignored recipient change or included rotated authentication material")
			}
		})
	}
}
