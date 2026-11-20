package notify

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTrustedBatchIncludesObservationTimeAndRecoveryImpact(t *testing.T) {
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	e := JournalEntry{UID: "0", Identifier: "fleetdeck-audit", Cursor: "private-journal-cursor", Realtime: strconv.FormatInt(at.UnixMicro(), 10), Message: `{"action":"backup.restore","success":false,"details":"private-note"}`}
	a := Summarize([]JournalEntry{e}, "node-a")
	b := Summarize([]JournalEntry{e}, "node-a")
	if a == nil || a.Level != "critical" || a.Incident == "" || a.Incident != b.Incident || !strings.Contains(a.Message, at.Format(time.RFC3339)) || strings.Contains(a.Message, "private-note") || strings.Contains(a.Incident, "private") {
		t.Fatal("missing safe, dated restore alert")
	}
}
