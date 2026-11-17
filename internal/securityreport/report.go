// Package securityreport summarizes existing private observations. It performs
// no network probes, Docker inspection, upgrades or inference of missing checks.
package securityreport

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type Input struct{ Label, Path string }
type Node struct {
	Label          string                     `json:"label"`
	Status         string                     `json:"status"`
	CheckedAt      time.Time                  `json:"checked_at"`
	Critical       int                        `json:"critical"`
	Warnings       int                        `json:"warnings"`
	Unknown        int                        `json:"unknown"`
	RebootRequired bool                       `json:"reboot_required"`
	Actions        []string                   `json:"actions"`
	Previous       *securitywatch.Observation `json:"previous_observation,omitempty"`
}
type Report struct {
	Schema      int       `json:"schema"`
	GeneratedAt time.Time `json:"generated_at"`
	Nodes       []Node    `json:"nodes"`
	NotCovered  []string  `json:"not_covered"`
}

func Inputs(values []string) ([]Input, error) {
	if len(values) == 0 || len(values) > 64 {
		return nil, fmt.Errorf("provide 1–64 explicit label=state-file inputs")
	}
	seen := map[string]bool{}
	result := make([]Input, 0, len(values))
	for _, v := range values {
		label, path, ok := strings.Cut(v, "=")
		if !ok || !identifier.MatchString(label) || seen[label] || path == "" || strings.ContainsAny(path, "\x00\r\n") {
			return nil, fmt.Errorf("inputs require unique neutral labels and state-file paths")
		}
		seen[label] = true
		result = append(result, Input{label, path})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Label < result[j].Label })
	return result, nil
}

func Build(inputs []Input, now time.Time, maxAge time.Duration) (Report, error) {
	if maxAge < time.Minute || maxAge > 26*time.Hour {
		return Report{}, fmt.Errorf("max age must be between one minute and 26 hours")
	}
	// Validate even callers which do not use the CLI parser.
	values := make([]string, 0, len(inputs))
	for _, in := range inputs {
		values = append(values, in.Label+"="+in.Path)
	}
	inputs, err := Inputs(values)
	if err != nil {
		return Report{}, err
	}
	r := Report{Schema: 1, GeneratedAt: now.UTC(), NotCovered: []string{"access_and_mfa", "external_reachability", "offsite_backup_and_restore", "hoster_integrity"}}
	for _, in := range inputs {
		s, err := securitywatch.Load(in.Path)
		n := Node{Label: in.Label, Status: "unknown", Unknown: 1, Actions: []string{"restore_observation"}}
		if err == nil {
			n = summarize(in.Label, s, now, maxAge)
		}
		r.Nodes = append(r.Nodes, n)
	}
	return r, nil
}

func summarize(label string, s securitywatch.State, now time.Time, maxAge time.Duration) Node {
	r := s.Report
	n := Node{Label: label, Status: "unknown", CheckedAt: r.CheckedAt, RebootRequired: r.Host.RebootRequired, Actions: []string{}}
	if r.CheckedAt.IsZero() || r.CheckedAt.After(now.Add(30*time.Second)) || now.Sub(r.CheckedAt) > maxAge {
		n.Unknown = 1
		n.Actions = append(n.Actions, "refresh_observation")
		if n.RebootRequired {
			n.Actions = append(n.Actions, "verify_pending_restart")
		}
		return n
	}
	n.Critical, n.Warnings, n.Unknown = r.Counts()
	if r.Inventory.Status == "checked" && !completeInventory(r) {
		n.Unknown++
		n.Actions = append(n.Actions, "restore_inventory")
	}
	for _, v := range []struct {
		check  securitywatch.Check
		action string
		valid  string
	}{{r.Release, "review_fleetdeck_update", "up_to_date"}, {r.Host, "review_host_updates", "up_to_date"}, {r.Inventory, "restore_inventory", "checked"}} {
		switch v.check.Status {
		case v.valid:
		case "update_available":
			if v.valid == "checked" {
				n.Unknown++
			}
			n.Actions = append(n.Actions, v.action)
		case "unknown":
			n.Actions = append(n.Actions, "restore_observation")
		default:
			n.Unknown++
			n.Actions = append(n.Actions, "restore_observation")
		}
	}
	if n.RebootRequired {
		n.Actions = append(n.Actions, "plan_restart")
	}
	for _, im := range r.Images {
		if im.Status != "checked" && im.Status != "unknown" {
			n.Unknown++
		}
		if im.Status != "checked" {
			n.Actions = append(n.Actions, "restore_image_scan")
		}
		if len(im.Findings) > 0 {
			n.Actions = append(n.Actions, "triage_image_findings")
		}
	}
	for _, c := range r.Containers {
		if len(c.Findings) > 0 {
			n.Actions = append(n.Actions, "review_runtime_permissions")
		}
	}
	n.Status = "checked"
	if n.Unknown > 0 {
		n.Status = "unknown"
	}
	if n.Critical+n.Warnings > 0 {
		n.Status = "attention"
	}
	seen := map[string]bool{}
	out := []string{}
	for _, a := range n.Actions {
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	n.Actions = out
	for i := len(s.History) - 1; i >= 0; i-- {
		h := s.History[i]
		if !h.CheckedAt.IsZero() && h.CheckedAt.Before(r.CheckedAt) && r.CheckedAt.Sub(h.CheckedAt) <= 26*time.Hour && h.Critical >= 0 && h.Warnings >= 0 && h.Unknown >= 0 {
			copy := h
			n.Previous = &copy
			break
		}
	}
	return n
}

// A successful inventory count alone does not prove that every running image
// was scanned. Missing or ambiguous identities must never produce a clean report.
func completeInventory(r securitywatch.Report) bool {
	if r.Inventory.Count != len(r.Containers) {
		return false
	}
	images := map[string]bool{}
	for _, im := range r.Images {
		if im.ID == "" || images[im.ID] {
			return false
		}
		images[im.ID] = true
	}
	containers := map[string]bool{}
	for _, c := range r.Containers {
		if c.ID == "" || containers[c.ID] || !images[c.ImageID] {
			return false
		}
		containers[c.ID] = true
	}
	return true
}

func (r Report) Level() string {
	level := "info"
	for _, n := range r.Nodes {
		if n.Critical > 0 {
			return "critical"
		}
		if n.Status != "checked" || n.Unknown > 0 {
			level = "warning"
		}
	}
	return level
}

func (r Report) Text(language string, compact bool) string {
	de := language == "de"
	var b strings.Builder
	if de {
		fmt.Fprintln(&b, "FleetDeck · Sicherheitsbericht")
	} else {
		fmt.Fprintln(&b, "FleetDeck · Security report")
	}
	fmt.Fprintln(&b, r.GeneratedAt.Format(time.RFC3339))
	nodes := r.Nodes
	limit := len(nodes)
	if compact && limit > 8 {
		limit = 8
		// Copy before sorting: presentation must not change enrollment identity.
		nodes = append([]Node(nil), nodes...)
		sort.SliceStable(nodes, func(i, j int) bool {
			a, b := nodes[i], nodes[j]
			if a.Critical != b.Critical {
				return a.Critical > b.Critical
			}
			if a.Unknown != b.Unknown {
				return a.Unknown > b.Unknown
			}
			if a.Warnings != b.Warnings {
				return a.Warnings > b.Warnings
			}
			return a.RebootRequired && !b.RebootRequired
		})
		var critical, warnings, unknown int
		for _, n := range nodes {
			critical += n.Critical
			warnings += n.Warnings
			unknown += n.Unknown
		}
		if de {
			fmt.Fprintf(&b, "Gesamt über %d Server: %d kritisch, %d Hinweise, %d ungeklärt.\n", len(nodes), critical, warnings, unknown)
		} else {
			fmt.Fprintf(&b, "Total across %d nodes: %d critical, %d findings/updates, %d incomplete.\n", len(nodes), critical, warnings, unknown)
		}
	}
	for _, n := range nodes[:limit] {
		if de {
			fmt.Fprintf(&b, "\n%s: %d kritisch, %d Hinweise, %d ungeklärte Prüfungen\n", n.Label, n.Critical, n.Warnings, n.Unknown)
		} else {
			fmt.Fprintf(&b, "\n%s: %d critical, %d findings/updates, %d incomplete checks\n", n.Label, n.Critical, n.Warnings, n.Unknown)
		}
		if !compact {
			if de {
				fmt.Fprint(&b, "Prüfstand: ")
			} else {
				fmt.Fprint(&b, "Observed: ")
			}
			if n.CheckedAt.IsZero() {
				fmt.Fprintln(&b, "unknown")
			} else {
				fmt.Fprintln(&b, n.CheckedAt.Format(time.RFC3339))
			}
			if n.Previous != nil {
				if de {
					fmt.Fprintf(&b, "Vorige Beobachtung: %d kritisch, %d Hinweise, %d ungeklärt (%s).\n", n.Previous.Critical, n.Previous.Warnings, n.Previous.Unknown, n.Previous.CheckedAt.Format(time.RFC3339))
				} else {
					fmt.Fprintf(&b, "Previous observation: %d critical, %d findings/updates, %d incomplete (%s).\n", n.Previous.Critical, n.Previous.Warnings, n.Previous.Unknown, n.Previous.CheckedAt.Format(time.RFC3339))
				}
			}
		}
		for i, a := range n.Actions {
			if compact && i >= 2 {
				break
			}
			fmt.Fprintln(&b, "- "+actionText(a, de))
		}
		if len(n.Actions) == 0 {
			if de {
				fmt.Fprintln(&b, "Keine Befunde innerhalb der erfassten Prüfungen.")
			} else {
				fmt.Fprintln(&b, "No findings within the observed checks.")
			}
		}
	}
	if limit < len(r.Nodes) {
		if de {
			fmt.Fprintf(&b, "\n+ %d Server im vollständigen Bericht. Höchste Priorität zuerst.\n", len(r.Nodes)-limit)
		} else {
			fmt.Fprintf(&b, "\n+ %d nodes in full report. Highest priority first.\n", len(r.Nodes)-limit)
		}
	}
	if de {
		fmt.Fprintln(&b, "\nNicht erfasst: SSH/MFA, externe Erreichbarkeit, Offsite-Sicherung/Wiederherstellung, Integrität des Hosters. Weniger Befunde allein belegen keine Behebung. Es wurde nichts aktualisiert.")
	} else {
		fmt.Fprintln(&b, "\nNot covered: SSH/MFA, external reachability, offsite backup/restore, provider integrity. Fewer findings alone do not prove remediation. No updates were installed.")
	}
	return b.String()
}

func actionText(code string, de bool) string {
	values := map[string][2]string{
		"restore_observation":        {"Restore the missing or incomplete check.", "Fehlende oder unvollständige Prüfung wiederherstellen."},
		"refresh_observation":        {"Refresh stale, missing or clock-invalid observations.", "Veraltete, fehlende oder zeitlich ungültige Messung erneuern."},
		"review_fleetdeck_update":    {"Review the available FleetDeck update.", "Verfügbares FleetDeck-Update prüfen."},
		"review_host_updates":        {"Review host updates and schedule maintenance.", "Systemupdates prüfen und Wartung planen."},
		"restore_inventory":          {"Restore the container inventory check.", "Container-Inventarprüfung wiederherstellen."},
		"plan_restart":               {"Plan the required restart with verified backup and recovery access.", "Fälligen Neustart mit geprüftem Backup und Wiederherstellungszugang planen."},
		"verify_pending_restart":     {"Verify the previously reported pending restart; its status is stale.", "Zuvor gemeldeten Neustartbedarf prüfen; sein Stand ist veraltet."},
		"restore_image_scan":         {"Restore the incomplete image scan.", "Unvollständige Image-Prüfung wiederherstellen."},
		"triage_image_findings":      {"Review affected images, applicability and available fixes.", "Betroffene Images, tatsächliche Betroffenheit und verfügbare Korrekturen prüfen."},
		"review_runtime_permissions": {"Review container privileges and isolation.", "Container-Rechte und Isolation prüfen."},
	}
	i := 0
	if de {
		i = 1
	}
	return values[code][i]
}
