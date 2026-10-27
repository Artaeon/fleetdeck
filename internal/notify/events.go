// Package notify projects trusted host events into minimal notification summaries.
package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

type JournalEntry struct {
	Cursor     string `json:"__CURSOR"`
	UID        string `json:"_UID"`
	Identifier string `json:"SYSLOG_IDENTIFIER"`
	Message    string `json:"MESSAGE"`
}

var eventName = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

// Classify deliberately excludes raw log text, usernames, addresses, command
// arguments, tokens and audit Details from external notifications.
func Classify(entry JournalEntry) string {
	if entry.UID != "0" {
		return ""
	}
	switch entry.Identifier {
	case "sshd", "sshd-session":
		if strings.HasPrefix(entry.Message, "Accepted ") {
			return "ssh.login.accepted"
		}
		for _, prefix := range []string{"Failed ", "Invalid user ", "error: PAM: Authentication failure", "maximum authentication attempts exceeded"} {
			if strings.HasPrefix(entry.Message, prefix) {
				return "ssh.authentication.rejected"
			}
		}
	case "fleetdeck-audit":
		var event struct {
			Action  string `json:"action"`
			Success *bool  `json:"success"`
		}
		if json.Unmarshal([]byte(entry.Message), &event) == nil && event.Success != nil && eventName.MatchString(event.Action) {
			if *event.Success {
				return "fleetdeck." + event.Action + ".succeeded"
			}
			return "fleetdeck." + event.Action + ".failed"
		}
	}
	return ""
}

func Summarize(entries []JournalEntry, label string) *monitor.Alert {
	counts := map[string]int{}
	for _, entry := range entries {
		if kind := Classify(entry); kind != "" {
			counts[kind]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(counts))
	level := "info"
	for key := range counts {
		keys = append(keys, key)
		if strings.HasSuffix(key, ".failed") || strings.HasSuffix(key, ".rejected") {
			level = "warning"
		}
	}
	sort.Strings(keys)
	var text strings.Builder
	text.WriteString("Host events since the previous delivered batch:\n")
	for _, key := range keys {
		fmt.Fprintf(&text, "%s: %d\n", key, counts[key])
	}
	text.WriteString("Counts describe journal events, not unique people. Inspect the private server journal for details. This notification does not approve access.")
	return &monitor.Alert{Level: level, Title: "Server activity", Message: text.String(), Target: label, Timestamp: time.Now().UTC()}
}
