package securityreport

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

func calendar(t *testing.T, edition string) Schedule {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Vienna")
	if err != nil {
		t.Fatal(err)
	}
	return Schedule{Edition: edition, Location: loc, Hour: 8, Weekday: time.Monday}
}
func privateState(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.Chmod(d, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(d, "state.json")
}

func TestWeekUsesLocalDaysAndRecordsGaps(t *testing.T) {
	s := calendar(t, "weekly")
	// The local clock changed during this seven-day period.
	now := time.Date(2025, 3, 31, 8, 1, 0, 0, s.Location)
	obs := func(at time.Time, count int) securitywatch.Observation {
		return securitywatch.Observation{CheckedAt: at, Critical: count}
	}
	sun := now.AddDate(0, 0, -1)
	r := Report{GeneratedAt: now, Nodes: []Node{{Label: "app-a", history: []securitywatch.Observation{
		obs(sun, 2), obs(sun.Add(2*time.Hour), 3), obs(now.AddDate(0, 0, -9), 20), obs(now.Add(time.Hour), 99), obs(now, 6),
	}}, {Label: "app-b"}}}
	p := r.period(s)
	if len(p.days) != 7 || p.days[6].observed != 1 || p.days[6].critical != 3 || p.days[0].observed != 0 || p.byHost["app-b"] != 0 || p.end.Sub(p.first) != 167*time.Hour {
		t.Fatalf("invalid period: %+v", p)
	}
}

type pdfChannel struct {
	testProvider
	copies [][]byte
}

func (p *pdfChannel) SendReportPDF(a monitor.Alert, data []byte) error {
	p.copies = append(p.copies, bytes.Clone(data))
	return p.Send(a)
}

func TestWeeklyPDFRecoveryAndRecipientBinding(t *testing.T) {
	s := calendar(t, "weekly")
	at := time.Date(2025, 3, 24, 7, 59, 0, 0, s.Location)
	r := Report{GeneratedAt: at, Nodes: []Node{{Label: "app-a", Status: "unknown", Unknown: 1, Actions: []string{"refresh_observation"}}}}
	path := privateState(t)
	mail := &pdfChannel{testProvider: testProvider{name: "email", fail: true, recipient: "a"}}
	tg := &testProvider{name: "telegram"}
	all := []monitor.AlertProvider{tg, mail}
	if err := DeliverBriefing(path, r, "de", s, all); err != nil || mail.sent != 0 {
		t.Fatal(err, mail.sent)
	}
	r.GeneratedAt = at.Add(time.Minute)
	if err := DeliverBriefing(path, r, "de", s, all); err == nil || mail.sent != 1 || tg.sent != 1 {
		t.Fatal(err)
	}
	backlog, err := ReadBacklog(path)
	if err != nil || len(backlog.Items) != 1 {
		t.Fatal(err, backlog)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r.GeneratedAt = r.GeneratedAt.AddDate(0, 0, 7)
	r.Nodes[0].Unknown = 10
	mail.recipient = "other"
	mail.fail = false
	if err := DeliverBriefing(path, r, "en", s, all); err == nil {
		t.Fatal("pending file redirected")
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(raw, after) {
		t.Fatal("rejected send changed queue")
	}
	mail.recipient = "a"
	if err := DeliverBriefing(path, r, "en", s, all); err != nil || len(mail.copies) != 2 || !bytes.Equal(mail.copies[0], mail.copies[1]) || mail.alerts[0] != mail.alerts[1] {
		t.Fatal(err)
	}
	if err := DeliverBriefing(path, r, "en", s, all); err != nil || mail.sent != 3 || tg.sent != 2 || bytes.Equal(mail.copies[2], mail.copies[0]) {
		t.Fatal("new period not delivered", err)
	}
	if err := DeliverBriefing(path, r, "en", s, all); err != nil || mail.sent != 3 || tg.sent != 2 {
		t.Fatal("period duplicated", err)
	}
	s.Hour++
	if err := DeliverBriefing(path, r, "en", s, all); err == nil {
		t.Fatal("reused schedule state")
	}
}

func TestDailyOnlyMaterialChangesIncludingEqualCountsAndRecovery(t *testing.T) {
	s := calendar(t, "daily")
	at := time.Date(2025, 10, 23, 18, 0, 0, 0, s.Location)
	r := Report{GeneratedAt: at, Nodes: []Node{{Label: "app-a", Status: "checked"}}}
	p := &testProvider{name: "telegram"}
	path := privateState(t)
	run := func(want int) {
		t.Helper()
		if err := DeliverBriefing(path, r, "en", s, []monitor.AlertProvider{p}); err != nil || p.sent != want {
			t.Fatal(err, p.sent, want)
		}
	}
	run(0)
	r.GeneratedAt = r.GeneratedAt.AddDate(0, 0, 1)
	r.Nodes[0].Status = "attention"
	r.Nodes[0].Critical = 1
	r.Nodes[0].evidence = "finding-a"
	run(1)
	r.GeneratedAt = r.GeneratedAt.AddDate(0, 0, 1)
	r.Nodes[0].CheckedAt = r.GeneratedAt
	run(1)
	r.GeneratedAt = r.GeneratedAt.AddDate(0, 0, 1)
	r.Nodes[0].evidence = "finding-b"
	run(2)
	run(2)
	r.GeneratedAt = r.GeneratedAt.AddDate(0, 0, 1)
	r.Nodes[0] = Node{Label: "app-a", Status: "checked"}
	run(3)
	if !strings.Contains(p.alerts[2].Message, "clear") {
		t.Fatal("missing recovery")
	}
}

func TestControlEvidenceExpiresAndCannotBeRenewedByScan(t *testing.T) {
	now := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
	path := privateState(t)
	c := ControlEvidence{Host: "app-a", Control: "restore_test", Source: "isolated-restore", Observed: now.Add(-10 * time.Hour), Expires: now.Add(time.Hour), Result: "pass", Reference: fmt.Sprintf("%x", sha256.Sum256([]byte("local evidence")))}
	write := func(cs ...ControlEvidence) {
		t.Helper()
		data, _ := json.Marshal(controlFile{1, cs})
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(c)
	newReport := func(at time.Time) Report {
		return Report{GeneratedAt: at, Nodes: []Node{{Label: "app-a", Status: "checked", CheckedAt: at}}}
	}
	r := newReport(now)
	if err := r.WithControls(path); err != nil || r.Nodes[0].Controls[3].Result != "pass" || r.Nodes[0].Unknown != 3 {
		t.Fatal(err, r.Nodes[0])
	}
	late := newReport(now.Add(2 * time.Hour))
	if err := late.WithControls(path); err != nil || late.Nodes[0].Controls[3].Result != "expired" || late.Nodes[0].Unknown != 4 {
		t.Fatal(err, late.Nodes[0])
	}
	conflict := c
	conflict.Result = "fail"
	write(c, conflict)
	if newR := newReport(now); newR.WithControls(path) == nil {
		t.Fatal("accepted conflicting evidence")
	}
	c.Expires = now.AddDate(3, 0, 0)
	write(c)
	if newR := newReport(now); newR.WithControls(path) == nil {
		t.Fatal("unbounded evidence lifetime")
	}
}

func TestWeeklyPDFAllEnrolledHostsAndBoundedSize(t *testing.T) {
	s := calendar(t, "weekly")
	r := Report{GeneratedAt: time.Now()}
	for i := 0; i < 64; i++ {
		n := Node{Label: fmt.Sprintf("node-%02d", i), Status: "unknown", Unknown: 1}
		for _, k := range controlNames {
			n.Controls = append(n.Controls, ControlEvidence{Host: n.Label, Control: k.key, Result: "missing"})
		}
		r.Nodes = append(r.Nodes, n)
	}
	pdf, err := r.WeeklyPDF(s, "de")
	if err != nil || monitor.ValidateReportPDF(pdf) != nil {
		t.Fatal(err, len(pdf))
	}
	if !bytes.Contains(pdf, []byte(fmt.Sprintf("%x", "node-63"))) || !bytes.Contains(pdf, []byte("xref")) {
		t.Fatal("missing last host or PDF structure")
	}
	pdf2, _ := r.WeeklyPDF(s, "de")
	if !bytes.Equal(pdf, pdf2) {
		t.Fatal("non-reproducible document")
	}
}
