package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DiscordProvider sends alerts as rich embeds to a Discord webhook.
type DiscordProvider struct {
	WebhookURL string
}

// NewDiscordProvider creates a DiscordProvider for the given webhook URL.
func NewDiscordProvider(webhookURL string) *DiscordProvider {
	return &DiscordProvider{WebhookURL: webhookURL}
}

func (p *DiscordProvider) Name() string { return "discord" }

func (p *DiscordProvider) Send(alert Alert) error {
	// Red for critical/warning, green for info (recovery).
	color := 65280 // green (0x00FF00)
	switch alert.Level {
	case "critical":
		color = 16711680 // red (0xFF0000)
	case "warning":
		color = 16744192 // orange (0xFF9900)
	}

	type embedField struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Inline bool   `json:"inline"`
	}

	type embed struct {
		Title       string       `json:"title"`
		Description string       `json:"description"`
		Color       int          `json:"color"`
		Fields      []embedField `json:"fields"`
		Timestamp   string       `json:"timestamp"`
		Footer      *struct {
			Text string `json:"text"`
		} `json:"footer,omitempty"`
	}

	type discordPayload struct {
		Content interface{} `json:"content"`
		Embeds  []embed     `json:"embeds"`
	}

	payload := discordPayload{
		Content: nil,
		Embeds: []embed{
			{
				Title:       alert.Title,
				Description: alert.Message,
				Color:       color,
				Fields: []embedField{
					{Name: "Target", Value: alert.Target, Inline: true},
					{Name: "Level", Value: strings.ToUpper(alert.Level), Inline: true},
				},
				Timestamp: alert.Timestamp.Format(time.RFC3339),
				Footer: &struct {
					Text string `json:"text"`
				}{Text: "FleetDeck Monitor"},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling discord payload: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(p.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending discord message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}
	return nil
}
