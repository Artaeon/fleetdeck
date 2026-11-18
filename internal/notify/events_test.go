package notify

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

func TestTrustedEventsAndPrivateSummary(t *testing.T) {
	entries := []JournalEntry{
		{UID: "0", Identifier: "sshd", Message: "Accepted publickey for private-user from 192.0.2.4 port 1234 ssh2"},
		{UID: "0", Identifier: "sshd-session", Message: "Failed publickey for private-user from 192.0.2.4 port 1234 ssh2"},
		{UID: "1001", Identifier: "sshd", Message: "Accepted publickey forged"},
		{UID: "0", Identifier: "fleetdeck-audit", Message: `{"action":"backup.create","success":false,"details":"private-token","project":"private-project"}`},
		{UID: "0", Identifier: "fleetdeck-audit", Message: `{"action":"bad\ninjection","success":true}`},
	}
	alert := Summarize(entries, "vps-one")
	if alert == nil || alert.Level != "warning" {
		t.Fatal("missing security summary")
	}
	for _, want := range []string{"ssh.login.accepted: 1", "ssh.authentication.rejected: 1", "fleetdeck.backup.create.failed: 1"} {
		if !strings.Contains(alert.Message, want) {
			t.Fatal("missing event count", want)
		}
	}
	for _, private := range []string{"private-", "192.0.2.4", "injection", "forged"} {
		if strings.Contains(alert.Message, private) {
			t.Fatal("private journal data escaped")
		}
	}
}

type fakeProvider struct {
	name      string
	recipient string
	alerts    []monitor.Alert
	fail      bool
	sent      int
}

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) DestinationIdentity() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(p.name+p.recipient)))
}
func (p *fakeProvider) Send(a monitor.Alert) error {
	p.alerts = append(p.alerts, a)
	p.sent++
	if p.fail {
		return errors.New("private-error")
	}
	return nil
}

func TestIndependentDeliveryAndDurableRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := &relayState{Since: "initial", Cursor: "before", Pending: &pendingBatch{Cursor: "after", Delivered: map[string]bool{}}}
	telegram := &fakeProvider{name: "telegram", fail: true}
	email := &fakeProvider{name: "email"}
	state.Version = 2
	state.Pending.Alert.Timestamp = time.Now()
	state.Pending.Destinations, _ = monitor.BindProviders([]monitor.AlertProvider{telegram, email})
	if deliver(path, state, []monitor.AlertProvider{telegram, email}) == nil {
		t.Fatal("failed channel not reported")
	}
	if state.Cursor != "before" || email.sent != 1 || !state.Pending.Delivered["email"] {
		t.Fatal("delivery blocked another channel or lost cursor")
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("state not private")
	}
	telegram.fail = false
	if err := deliver(path, state, []monitor.AlertProvider{telegram, email}); err != nil {
		t.Fatal(err)
	}
	if email.sent != 1 || telegram.sent != 2 || state.Pending != nil || state.Cursor != "after" {
		t.Fatal("retry duplicated success or lost pending event")
	}
}

func TestRelayBindsBatchAcrossRestartAndMissingChannels(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "relay.json")
	mail, chat, extra := &fakeProvider{name: "email", fail: true}, &fakeProvider{name: "telegram"}, &fakeProvider{name: "added"}
	providers := []monitor.AlertProvider{chat, mail}
	destinations, err := monitor.BindProviders(providers)
	if err != nil {
		t.Fatal(err)
	}
	s := relayState{Version: 2, Since: "start", Cursor: "old", Pending: &pendingBatch{Cursor: "new", Destinations: destinations, Alert: monitor.Alert{Timestamp: time.Now().UTC()}}}
	if err := saveState(path, &s); err != nil {
		t.Fatal(err)
	}
	if deliver(path, &s, providers) == nil {
		t.Fatal("failure hidden")
	}
	s, err = readState(path)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	mail.recipient = "new"
	mail.fail = false
	if deliver(path, &s, providers) == nil || mail.sent != 1 {
		t.Fatal("redirected queued batch")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("rewrote queue on refusal")
	}
	if deliver(path, &s, []monitor.AlertProvider{chat, extra}) == nil || s.Cursor != "old" || extra.sent != 0 || chat.sent != 1 {
		t.Fatal("changed original recipient set")
	}
	mail.recipient = ""
	if err := deliver(path, &s, append(providers, extra)); err != nil || s.Cursor != "new" || s.Pending != nil || mail.sent != 2 || chat.sent != 1 || extra.sent != 0 {
		t.Fatal("failed to finish original batch", err)
	}
}

func TestWatchPreservesUnboundLegacyBatch(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	path := filepath.Join(dir, "relay.json")
	raw := `{"since":"start","cursor":"old","pending":{"cursor":"new","delivered":{"email":true}}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	p := &fakeProvider{name: "telegram"}
	err := Watch(context.Background(), path, "test", time.Minute, []monitor.AlertProvider{p})
	after, _ := os.ReadFile(path)
	if err == nil || !strings.Contains(err.Error(), "legacy") || p.sent != 0 || string(after) != raw {
		t.Fatal("legacy batch changed", err)
	}
}

func TestWatchQueuesNewActivityBeforeSendingAndResumesLegacyCursor(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "relay.json")
	if err := os.WriteFile(path, []byte(`{"since":"start","cursor":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	journal := `#!/bin/sh
for arg do
 if [ "$arg" = "--after-cursor=old" ]; then
  printf '%s\n' '{"__CURSOR":"new","_UID":"0","SYSLOG_IDENTIFIER":"sshd","MESSAGE":"Accepted publickey for fixture"}'
  exit 0
 fi
done
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(journal), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	p := &fakeProvider{name: "telegram", fail: true}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Watch(ctx, path, "fixture", time.Hour, []monitor.AlertProvider{p}); err == nil || p.sent != 1 {
		t.Fatal("watch did not retain attempted delivery", err)
	}
	view, err := ReadBacklog(path)
	if err != nil || len(view.Items) != 1 || view.Items[0].PendingSince == nil || view.Items[0].Channel != "telegram" {
		t.Fatal(view, err)
	}
	p.fail = false
	if err := Watch(ctx, path, "fixture", time.Hour, []monitor.AlertProvider{p}); err == nil || p.sent != 2 {
		t.Fatal("restart did not retry original activity", err)
	}
	s, err := readState(path)
	if err != nil || s.Cursor != "new" || s.Pending != nil {
		t.Fatal("journal cursor advanced incorrectly", err)
	}
}
