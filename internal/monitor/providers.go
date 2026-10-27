package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WebhookProvider sends alerts as JSON POST requests to a URL.
type WebhookProvider struct {
	URL string
}

func NewWebhookProvider(url string) *WebhookProvider {
	return &WebhookProvider{URL: url}
}

func (p *WebhookProvider) Name() string { return "webhook" }

func (p *WebhookProvider) Send(alert Alert) error {
	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshaling alert: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(p.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// SlackProvider sends formatted messages to a Slack incoming webhook.
type SlackProvider struct {
	WebhookURL string
	Channel    string
}

func NewSlackProvider(webhookURL string) *SlackProvider {
	return &SlackProvider{WebhookURL: webhookURL}
}

func (p *SlackProvider) Name() string { return "slack" }

func (p *SlackProvider) Send(alert Alert) error {
	color := "#36a64f" // green
	switch alert.Level {
	case "warning":
		color = "#ff9900"
	case "critical":
		color = "#ff0000"
	}

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  alert.Title,
				"text":   alert.Message,
				"footer": "FleetDeck Monitor",
				"ts":     alert.Timestamp.Unix(),
				"fields": []map[string]string{
					{"title": "Target", "value": alert.Target, "short": "true"},
					{"title": "Level", "value": strings.ToUpper(alert.Level), "short": "true"},
				},
			},
		},
	}

	if p.Channel != "" {
		payload["channel"] = p.Channel
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling slack payload: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(p.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending slack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}
	return nil
}
