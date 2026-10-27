package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// TelegramProvider sends outbound notifications only. It cannot approve access
// or execute chat commands. Keep this configuration outside project repositories.
type TelegramProvider struct {
	BotToken string `json:"bot_token"`
	ChatID   int64  `json:"chat_id"`
	ThreadID int64  `json:"message_thread_id,omitempty"`
	client   *http.Client
}

var telegramToken = regexp.MustCompile(`^[0-9]{5,20}:[A-Za-z0-9_-]{20,128}$`)

func (p *TelegramProvider) Name() string { return "telegram" }

func LoadTelegramProvider(path string) (*TelegramProvider, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open private Telegram configuration")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 65536 {
		return nil, fmt.Errorf("Telegram configuration must be a private regular file (0600 or stricter, at most 64 KiB)")
	}
	var p TelegramProvider
	decoder := json.NewDecoder(io.LimitReader(f, 65537))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&p) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("invalid Telegram configuration JSON")
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *TelegramProvider) validate() error {
	if !telegramToken.MatchString(p.BotToken) || p.ChatID == 0 || p.ThreadID < 0 {
		return fmt.Errorf("Telegram requires a bot token, nonzero numeric chat_id and nonnegative message_thread_id")
	}
	return nil
}

func (p *TelegramProvider) Send(alert Alert) error {
	if err := p.validate(); err != nil {
		return err
	}
	text := fmt.Sprintf("FleetDeck [%s]\n%s\nTarget: %s\nTime: %s\n\n%s", strings.ToUpper(alert.Level), alert.Title, alert.Target, alert.Timestamp.UTC().Format(time.RFC3339), alert.Message)
	// Telegram measures entity offsets in UTF-16. This conservative limit also
	// accommodates astral characters without splitting UTF-8 sequences.
	var bounded strings.Builder
	units := 0
	for _, r := range text {
		n := 1
		if r > 0xffff {
			n = 2
		}
		if units+n > 4000 {
			bounded.WriteString("\n[truncated]")
			break
		}
		bounded.WriteRune(r)
		units += n
	}
	body, _ := json.Marshal(struct {
		ChatID      int64  `json:"chat_id"`
		ThreadID    int64  `json:"message_thread_id,omitempty"`
		Text        string `json:"text"`
		Protect     bool   `json:"protect_content"`
		LinkPreview struct {
			Disabled bool `json:"is_disabled"`
		} `json:"link_preview_options"`
	}{p.ChatID, p.ThreadID, bounded.String(), true, struct {
		Disabled bool `json:"is_disabled"`
	}{true}})
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Post("https://api.telegram.org/bot"+p.BotToken+"/sendMessage", "application/json", bytes.NewReader(body))
	// Transport errors contain the request URL and thus the token. Never wrap
	// them or return the provider's response body (which can echo private data).
	if err != nil {
		return fmt.Errorf("Telegram delivery failed; check connectivity and credentials")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram delivery rejected (HTTP %d)", response.StatusCode)
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result) != nil || !result.OK {
		return fmt.Errorf("Telegram did not confirm delivery")
	}
	return nil
}
