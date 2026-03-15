package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// WebhookProvider sends alerts as JSON POST requests to a URL.
type WebhookProvider struct {
	URL string
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

// EmailProvider sends alerts via SMTP.
type EmailProvider struct {
	Host     string // SMTP host
	Port     int    // SMTP port
	Username string
	Password string
	From     string
	To       []string
}

func (p *EmailProvider) Name() string { return "email" }

func (p *EmailProvider) Send(alert Alert) error {
	subject := fmt.Sprintf("[FleetDeck/%s] %s", strings.ToUpper(alert.Level), alert.Title)

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", p.From)
	fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(p.To, ", "))
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	fmt.Fprintf(&msg, "Date: %s\r\n", alert.Timestamp.Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "\r\n")
	fmt.Fprintf(&msg, "Target: %s\r\n", alert.Target)
	fmt.Fprintf(&msg, "Level:  %s\r\n", alert.Level)
	fmt.Fprintf(&msg, "Time:   %s\r\n\r\n", alert.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(&msg, "%s\r\n", alert.Message)

	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)

	var auth smtp.Auth
	if p.Username != "" {
		auth = smtp.PlainAuth("", p.Username, p.Password, p.Host)
	}

	if err := smtp.SendMail(addr, auth, p.From, p.To, msg.Bytes()); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}
	return nil
}
