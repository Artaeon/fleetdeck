package securityreport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

// Schedule uses local calendar boundaries, including clock changes. The scheduler
// can retry every hour. Weekly evidence covers the seven *completed* local days.
type Schedule struct {
	FirstDate  string
	ArchiveDir string
	Edition    string
	Location   *time.Location
	Hour       int
	Weekday    time.Weekday
}

func (s Schedule) Validate() error {
	if s.FirstDate != "" {
		if _, err := time.Parse(time.DateOnly, s.FirstDate); err != nil {
			return fmt.Errorf("first delivery date must be YYYY-MM-DD")
		}
	}
	if (s.Edition != "daily" && s.Edition != "weekly") || s.Location == nil || s.Hour < 0 || s.Hour > 23 || s.Weekday < time.Sunday || s.Weekday > time.Saturday {
		return fmt.Errorf("provide a daily or weekly edition, timezone, weekday and hour (0–23)")
	}
	return nil
}

func (s Schedule) boundary(now time.Time) time.Time {
	local := now.In(s.Location)
	delta := 0
	if s.Edition == "weekly" {
		delta = (int(local.Weekday()) - int(s.Weekday) + 7) % 7
	}
	d := local.AddDate(0, 0, -delta)
	return time.Date(d.Year(), d.Month(), d.Day(), s.Hour, 0, 0, 0, s.Location)
}

type dayCounts struct {
	date                        time.Time
	observed                    int
	critical, warnings, unknown int
}
type periodEvidence struct {
	first, end time.Time
	days       []dayCounts
	byHost     map[string]int
}

func (r Report) period(s Schedule) periodEvidence {
	end := s.boundary(r.GeneratedAt)
	if end.After(r.GeneratedAt) {
		end = end.AddDate(0, 0, -7)
	}
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, s.Location)
	first := end.AddDate(0, 0, -7)
	p := periodEvidence{first: first, end: end, byHost: map[string]int{}}
	for d := first; d.Before(end); d = d.AddDate(0, 0, 1) {
		p.days = append(p.days, dayCounts{date: d})
	}
	for _, n := range r.Nodes {
		latest := map[string]securitywatch.Observation{}
		add := func(h securitywatch.Observation) {
			if h.CheckedAt.Before(first) || !h.CheckedAt.Before(end) || h.CheckedAt.After(r.GeneratedAt) || h.Critical < 0 || h.Warnings < 0 || h.Unknown < 0 {
				return
			}
			key := h.CheckedAt.In(s.Location).Format(time.DateOnly)
			if old, ok := latest[key]; !ok || h.CheckedAt.After(old.CheckedAt) {
				latest[key] = h
			}
		}
		for _, h := range n.history {
			add(h)
		}
		// Legacy states can have a scan without a history entry. Only use
		// assessed current counts as history when the observation is fresh.
		if n.evidence != "" && len(n.Controls) == 0 && n.Source != "observer" {
			add(securitywatch.Observation{CheckedAt: n.CheckedAt, Critical: n.Critical, Warnings: n.Warnings, Unknown: n.Unknown})
		}
		for i := range p.days {
			d := &p.days[i]
			if h, ok := latest[d.date.Format(time.DateOnly)]; ok {
				d.observed++
				d.critical += h.Critical
				d.warnings += h.Warnings
				d.unknown += h.Unknown
				p.byHost[n.Label]++
			}
		}
	}
	return p
}

// materialState deliberately omits timestamps and history which change on a quiet day.
func (r Report) materialState() map[string]string {
	out := map[string]string{}
	for _, n := range r.Nodes {
		states := map[string]string{}
		for _, c := range n.Controls {
			states[c.Control] = c.Result
		}
		b, _ := json.Marshal([]any{n.Status, n.Critical, n.Warnings, n.Unknown, n.RebootRequired, n.Actions, n.evidence, states})
		sum := sha256.Sum256(b)
		out[n.Label] = hex.EncodeToString(sum[:])
	}
	return out
}

func words(de bool, en, german string) string {
	if de {
		return german
	}
	return en
}

func (r Report) priorities() []Node {
	nodes := append([]Node(nil), r.Nodes...)
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		switch {
		case a.Critical != b.Critical:
			return a.Critical > b.Critical
		case a.Unknown != b.Unknown:
			return a.Unknown > b.Unknown
		case a.Warnings != b.Warnings:
			return a.Warnings > b.Warnings
		case a.RebootRequired != b.RebootRequired:
			return a.RebootRequired
		}
		return a.Label < b.Label
	})
	for i := range nodes {
		n := &nodes[i]
		n.Actions = append([]string(nil), n.Actions...)
		rank := func(a string) int {
			switch a {
			case "triage_image_findings", "review_observer_findings":
				if n.Critical > 0 {
					return 0
				}
				return 3
			case "restore_observation", "refresh_observation", "restore_image_scan", "restore_inventory":
				return 1
			case "review_runtime_permissions", "check_restore_test", "check_offsite_backup":
				return 2
			default:
				return 4
			}
		}
		sort.SliceStable(n.Actions, func(i, j int) bool { return rank(n.Actions[i]) < rank(n.Actions[j]) })
	}
	return nodes
}

func (r Report) briefingText(s Schedule, language string, changed map[string]bool) string {
	de := language == "de"
	var b strings.Builder
	if s.Edition == "weekly" {
		p := r.period(s)
		fmt.Fprintf(&b, "%s – %s (%s)\n", p.first.Format(time.DateOnly), p.end.AddDate(0, 0, -1).Format(time.DateOnly), s.Location)
	} else {
		fmt.Fprintf(&b, "%s · %s\n", r.GeneratedAt.In(s.Location).Format(time.DateOnly), s.Location)
	}
	var c, w, u int
	for _, n := range r.Nodes {
		c += n.Critical
		w += n.Warnings
		u += n.Unknown
	}
	fmt.Fprintf(&b, words(de, "%d hosts · %d critical · %d findings/updates · %d incomplete\n", "%d Hosts · %d kritisch · %d Hinweise/Updates · %d ungeklärte Prüfungen\n"), len(r.Nodes), c, w, u)
	lines := 0
	for _, n := range r.priorities() {
		if changed != nil && !changed[n.Label] {
			continue
		}
		if changed == nil && len(n.Actions) == 0 && n.Status == "checked" {
			continue
		}
		if lines == 3 {
			break
		}
		action := words(de, "Checks clear on the latest observation.", "Letzte Prüfung ohne Befund.")
		if len(n.Actions) > 0 {
			action = actionText(n.Actions[0], de)
		}
		fmt.Fprintf(&b, "\n%s · %s", n.Label, action)
		if changed != nil && n.Previous != nil {
			fmt.Fprintf(&b, words(de, "\nLast two scans: %d → %d critical, %d → %d findings/updates.", "\nLetzte zwei Scans: %d → %d kritisch, %d → %d Hinweise/Updates."), n.Previous.Critical, n.Critical, n.Previous.Warnings, n.Warnings)
		}
		lines++
	}
	if changed != nil && len(changed) > lines {
		fmt.Fprintf(&b, words(de, "\n+%d more changed hosts.", "\n+%d weitere Hosts mit Änderungen."), len(changed)-lines)
	}
	return strings.TrimSpace(b.String())
}
