package securityreport

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

func TestSignedControlsAndExpiry(t *testing.T) {
	public, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := privateState(t)
	dir := filepath.Dir(path)
	key, sig := filepath.Join(dir, "key.pem"), filepath.Join(dir, "controls.sig")
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	put := func(p string, b []byte) {
		t.Helper()
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	put(key, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	data := []byte("{\"version\":1,\"records\":[]}\n")
	put(path, data)
	put(sig, ed25519.Sign(secret, data))
	r := Report{GeneratedAt: time.Now(), Nodes: []Node{{Label: "app-a", Status: "checked"}}}
	if err = r.LoadControls(path, key, sig); err != nil || r.Nodes[0].Unknown != 4 {
		t.Fatal(err)
	}
	put(path, append(data, ' '))
	if r.LoadControls(path, key, sig) == nil || r.LoadControls(path, key, "") == nil {
		t.Fatal("accepted tampered or unsigned data")
	}
}

func TestCreateOnlyPDFArchiveAndIndependentHeartbeat(t *testing.T) {
	s := calendar(t, "weekly")
	path := privateState(t)
	s.ArchiveDir = filepath.Dir(privateState(t))
	now := time.Date(2025, 3, 31, 9, 0, 0, 0, s.Location)
	r := Report{GeneratedAt: now, Nodes: []Node{{Label: "node-a", Status: "unknown", Unknown: 1}}}
	mail := &pdfChannel{testProvider: testProvider{name: "email", fail: true}}
	send := func() error { return DeliverBriefing(path, r, "en", s, []monitor.AlertProvider{mail}) }
	if send() == nil {
		t.Fatal("failed transport reported success")
	}
	matches, err := filepath.Glob(filepath.Join(s.ArchiveDir, "*.pdf"))
	if err != nil || len(matches) != 1 {
		t.Fatal(err, matches)
	}
	original, err := os.ReadFile(matches[0])
	if err != nil || !bytes.Equal(original, mail.copies[0]) {
		t.Fatal("archived wrong attachment")
	}
	if !strings.Contains(matches[0], fmt.Sprintf("%x", sha256.Sum256(original))) {
		t.Fatal("missing archive digest")
	}
	status, err := Status(path, now, time.Hour)
	if err != nil || status.Status != "pending" || status.Pending != 1 || status.OldestPending == nil {
		t.Fatal(err, status)
	}
	if err = os.WriteFile(matches[0], []byte("altered"), 0600); err != nil {
		t.Fatal(err)
	}
	mail.fail = false
	if send() == nil || len(mail.copies) != 1 {
		t.Fatal("overwrote archive or sent unverifiable PDF")
	}
	if data, _ := os.ReadFile(matches[0]); string(data) != "altered" {
		t.Fatal("existing archive overwritten")
	}
	if err = os.WriteFile(matches[0], original, 0600); err != nil {
		t.Fatal(err)
	}
	if err = send(); err != nil {
		t.Fatal(err)
	}
	status, err = Status(path, now, time.Hour)
	if err != nil || status.Status != "current" || status.Pending != 0 {
		t.Fatal(err, status)
	}
	status, err = Status(path, now.Add(2*time.Hour), time.Hour)
	if err != nil || status.Status != "stale" {
		t.Fatal("missing run hidden", err, status)
	}
	b, _ := json.Marshal(status)
	if bytes.Contains(b, []byte("node-a")) || bytes.Contains(b, []byte("%PDF")) {
		t.Fatal("status leak")
	}
}

func TestRejectDamagedAttachmentBeforeAnySend(t *testing.T) {
	s := calendar(t, "weekly")
	path := privateState(t)
	r := Report{GeneratedAt: time.Date(2025, 3, 31, 10, 0, 0, 0, s.Location), Nodes: []Node{{Label: "node-a"}}}
	p := &pdfChannel{testProvider: testProvider{name: "email", fail: true}}
	if DeliverBriefing(path, r, "de", s, []monitor.AlertProvider{p}) == nil {
		t.Fatal("expected pending")
	}
	state, err := readState(path)
	if err != nil {
		t.Fatal(err)
	}
	queued := state.Pending["email"]
	queued.PDF[16] ^= 1
	state.Pending["email"] = queued
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	p.fail = false
	if DeliverBriefing(path, r, "de", s, []monitor.AlertProvider{p}) == nil || p.sent != 1 {
		t.Fatal("corrupt PDF sent")
	}
}
