package securitywatch

import (
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

func TestNotificationsIgnoreContainerReplacementAndHealthyInventory(t *testing.T) {
	p := &provider{name: "telegram"}
	s := State{Version: 1}
	r := Report{CheckedAt: time.Now(), Release: Check{Status: "up_to_date"}, Host: Check{Status: "up_to_date"}, Inventory: Check{Status: "checked", Count: 1},
		Images:     []Image{{ID: "image-a", Status: "checked", Findings: []Finding{{ID: "CVE-example", Severity: "high"}}}},
		Containers: []Container{{ID: "old-container", ImageID: "image-a", Findings: []Finding{{ID: "root", Severity: "warning"}, {ID: "privileged", Severity: "critical"}}}}}
	providers := []monitor.AlertProvider{p}
	if err := Notify(&s, r, "node-a", providers); err != nil {
		t.Fatal(err)
	}
	r.Containers[0].ID = "replacement-container"
	r.Containers[0].Findings[0], r.Containers[0].Findings[1] = r.Containers[0].Findings[1], r.Containers[0].Findings[0]
	r.Images = append([]Image{{ID: "healthy-image", Status: "checked"}}, r.Images...)
	r.Inventory.Count = 2
	r.CheckedAt = r.CheckedAt.Add(time.Hour)
	if err := Notify(&s, r, "node-a", providers); err != nil {
		t.Fatal(err)
	}
	if len(p.sent) != 1 {
		t.Fatal("unchanged risk repeated after deployment")
	}
	if s.Report.Containers[0].ID != "replacement-container" {
		t.Fatal("raw evidence was not retained")
	}
	r.Containers = append(r.Containers, r.Containers[0])
	if err := Notify(&s, r, "node-a", providers); err != nil {
		t.Fatal(err)
	}
	if len(p.sent) != 2 {
		t.Fatal("additional affected replica hidden")
	}
	r.Images[1].Findings[0].Severity = "critical"
	if err := Notify(&s, r, "node-a", providers); err != nil {
		t.Fatal(err)
	}
	if len(p.sent) != 3 {
		t.Fatal("severity escalation hidden")
	}
	r.Images[1].ID = "changed-artifact"
	if err := Notify(&s, r, "node-a", providers); err != nil {
		t.Fatal(err)
	}
	if len(p.sent) != 4 {
		t.Fatal("artifact change hidden")
	}
}
