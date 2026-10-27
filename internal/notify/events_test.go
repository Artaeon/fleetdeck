package notify

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	name string
	fail bool
	sent int
}

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) Send(monitor.Alert) error {
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
