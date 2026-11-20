// Package notify projects trusted host events into minimal notification summaries.
package notify

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

type JournalEntry struct {
	Cursor     string `json:"__CURSOR"`
	Realtime   string `json:"__REALTIME_TIMESTAMP"`
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
	var first, last time.Time
	var lastCursor string
	now := time.Now().UTC()
	for _, entry := range entries {
		if kind := Classify(entry); kind != "" {
			counts[kind]++
			lastCursor = entry.Cursor
			if us, err := strconv.ParseInt(entry.Realtime, 10, 64); err == nil && us > 0 {
				at := time.UnixMicro(us).UTC()
				if !at.After(now.Add(30 * time.Second)) {
					if first.IsZero() || at.Before(first) {
						first = at
					}
					if at.After(last) {
						last = at
					}
				}
			}
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
	if !first.IsZero() {
		fmt.Fprintf(&text, "Observed %s", first.Format(time.RFC3339))
		if !last.Equal(first) {
			fmt.Fprintf(&text, " – %s", last.Format(time.RFC3339))
		}
		text.WriteString("\n")
	}
	title := "Server activity"
	switch {
	case counts["fleetdeck.backup.restore.failed"] > 0:
		level = "critical"
		title = "Restore failed"
		text.WriteString("Verify the last successful isolated restore and investigate the failed recovery.")
	case counts["fleetdeck.backup.create.failed"] > 0:
		title = "Backup failed"
		text.WriteString("Check the last completed backup and the next scheduled retry.")
	case counts["ssh.login.accepted"] > 0:
		title = "SSH access accepted"
		text.WriteString("Match the login time with planned access. Check the private SSH journal if unrecognized.")
	case counts["ssh.authentication.rejected"] > 0:
		title = "SSH authentication rejected"
		text.WriteString("Check for repeated attempts in the private SSH journal.")
	default:
		text.WriteString("Review the private host audit for affected operations.")
	}
	a := &monitor.Alert{Level: level, Title: title, Message: text.String(), Target: label, Timestamp: now}
	if lastCursor != "" {
		sum := sha256.Sum256([]byte(label + "/" + lastCursor))
		a.Incident = fmt.Sprintf("FD-%x", sum[:6])
	}
	return a
}
