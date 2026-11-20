package securityreport

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// A fixed-layout PDF with built-in fonts. All text is encoded as hexadecimal
// WinAnsi strings. It has no links, scripts, embedded files or external assets.
type pdfPage struct{ bytes.Buffer }

const ink = "0.09 0.16 0.20"
const teal = "0.01 0.34 0.32"
const muted = "0.26 0.33 0.36"

func (p *pdfPage) text(x, top, size int, bold bool, color, text string) {
	font := "F1"
	if bold {
		font = "F2"
	}
	data := make([]byte, 0, len(text))
	for _, r := range text {
		switch {
		case r >= 32 && r <= 255:
			data = append(data, byte(r))
		case r == '–':
			data = append(data, 150)
		case r == '—':
			data = append(data, 151)
		default:
			data = append(data, '?')
		}
	}
	fmt.Fprintf(p, "BT /%s %d Tf %s rg 1 0 0 1 %d %d Tm <%s> Tj ET\n", font, size, color, x, 842-top, hex.EncodeToString(data))
}
func (p *pdfPage) rect(x, top, w, h int, color string) {
	fmt.Fprintf(p, "%s rg %d %d %d %d re f\n", color, x, 842-top-h, w, h)
}
func (p *pdfPage) rule(top int) { p.rect(42, top, 511, 1, "0.83 0.87 0.86") }

func encodePDF(pages []*pdfPage) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>"}
	kids := []string{}
	for _, p := range pages {
		id := len(objects) + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", id))
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", id+1), fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", p.Len(), p.String()))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))
	var file bytes.Buffer
	file.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, file.Len())
		fmt.Fprintf(&file, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := file.Len()
	fmt.Fprintf(&file, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&file, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&file, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return file.Bytes()
}

// WeeklyPDF separates the latest risk picture from dated historical evidence.
func (r Report) WeeklyPDF(s Schedule, language string) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Edition != "weekly" || (language != "en" && language != "de") || len(r.Nodes) == 0 || len(r.Nodes) > 64 || r.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("weekly PDF requires a dated, bounded weekly report")
	}
	de := language == "de"
	tr := func(en, german string) string { return words(de, en, german) }
	period := r.period(s)
	window := period.first.Format(time.DateOnly) + " – " + period.end.AddDate(0, 0, -1).Format(time.DateOnly)
	issued := r.GeneratedAt.In(s.Location).Format("2006-01-02 15:04 MST")
	var pages []*pdfPage
	newPage := func(title string) *pdfPage {
		p := &pdfPage{}
		pages = append(pages, p)
		p.rect(0, 0, 595, 8, teal)
		p.text(42, 42, 11, true, teal, "FleetDeck")
		p.text(42, 92, 29, true, ink, title)
		p.text(42, 118, 11, false, muted, window+"   /   "+s.Location.String())
		p.rule(137)
		return p
	}
	p := newPage(tr("Weekly security review", "Wöchentlicher Sicherheitsbericht"))
	scanned, critical, warnings, incomplete := r.currentCounts()
	criticalText, warningText := fmt.Sprint(critical), fmt.Sprint(warnings)
	if scanned == 0 {
		criticalText = "–"
		if warnings == 0 {
			warningText = "–"
		}
	}
	var seen int
	for _, d := range period.days {
		seen += d.observed
	}
	p.text(42, 164, 11, true, ink, tr("Current position", "Aktueller Prüfstand"))
	p.text(42, 184, 10, false, muted, tr("Compiled ", "Erstellt ")+issued)
	x := 42
	for _, card := range []struct {
		value string
		label string
	}{
		{fmt.Sprintf("%d/%d", scanned, len(r.Nodes)), tr("Current scans", "Aktuelle Scans")},
		{criticalText, tr("Critical findings", "Kritische Befunde")},
		{warningText, tr("Findings / updates", "Hinweise / Updates")},
		{fmt.Sprint(incomplete), tr("Incomplete checks", "Ungeklärte Prüfungen")},
	} {
		p.rect(x, 203, 121, 65, "0.93 0.96 0.95")
		p.text(x+9, 231, 21, true, teal, card.value)
		p.text(x+9, 252, 9, false, ink, card.label)
		x += 130
	}
	p.text(42, 304, 15, true, ink, tr("Recommended actions", "Nächste Schritte"))
	y := 329
	actions := 0
	for _, n := range r.priorities() {
		if len(n.Actions) > 0 {
			a := n.Actions[0]
			p.text(42, y, 9, true, teal, n.Label)
			p.text(42, y+16, 10, false, ink, actionText(a, de))
			y += 43
			actions++
		}
		if actions == 3 {
			break
		}
	}
	if actions == 0 {
		p.text(42, y, 10, false, ink, tr("No current findings in the observed scope.", "Aktuell keine Befunde in den erfassten Prüfungen."))
	}
	p.rule(464)
	p.text(42, 493, 15, true, ink, tr("Evidence over seven days", "Nachweise für sieben Tage"))
	p.text(42, 514, 10, false, muted, fmt.Sprintf(tr("%d of %d possible host-days have an observation.", "Für %d von %d möglichen Host-Tagen liegt eine Beobachtung vor."), seen, 7*len(r.Nodes)))
	tableX := []int{42, 180, 288, 385, 476}
	for i, v := range []string{tr("Day", "Datum"), tr("Hosts seen", "Hosts erfasst"), tr("Critical", "Kritisch"), tr("Warnings", "Hinweise"), tr("Incomplete", "Ungeklärt")} {
		p.text(tableX[i], 548, 9, true, teal, v)
	}
	for i, d := range period.days {
		y := 569 + i*24
		p.rule(y + 7)
		vals := []string{d.date.Format(time.DateOnly), fmt.Sprintf("%d / %d", d.observed, len(r.Nodes)), "–", "–", "–"}
		if d.observed > 0 {
			vals[2] = fmt.Sprint(d.critical)
			vals[3] = fmt.Sprint(d.warnings)
			vals[4] = fmt.Sprint(d.unknown)
		}
		for j, v := range vals {
			p.text(tableX[j], y, 10, false, ink, v)
		}
	}
	p.text(42, 750, 9, false, muted, tr("Daily totals use the last observation for each reporting host.", "Tagessummen verwenden die jeweils letzte Beobachtung pro erfasstem Host."))
	p.text(42, 766, 9, false, muted, tr("A lower count alone cannot confirm a fix; missing days have no measured value.", "Zur Bestätigung einer Behebung ist ein Einzelnachweis nötig. Fehlende Tage bleiben offen."))
	for start := 0; start < len(r.Nodes); start += 8 {
		p = newPage(tr("Host evidence", "Prüfnachweise je Host"))
		p.text(42, 164, 10, false, muted, tr("Last scan, current findings and recorded days in this reporting period.", "Letzter Scan, aktuelle Befunde und belegte Tage im Berichtszeitraum."))
		y := 200
		end := min(start+8, len(r.Nodes))
		for _, n := range r.Nodes[start:end] {
			p.text(42, y, 12, true, ink, n.Label)
			at := tr("no observation", "keine Messung")
			if !n.CheckedAt.IsZero() {
				at = n.CheckedAt.In(s.Location).Format("2006-01-02 15:04 MST")
				if n.Version != "" {
					at += "  ·  " + n.Version[:min(18, len(n.Version))]
				}
			}
			status := map[string]string{"checked": tr("checked", "geprüft"), "unknown": tr("unknown", "ungeklärt"), "attention": tr("attention", "Handlungsbedarf")}[n.Status]
			p.text(42, y+17, 10, false, muted, fmt.Sprintf("%s  ·  %s  ·  %d / 7 %s", at, status, period.byHost[n.Label], tr("days", "Tage")))
			line := fmt.Sprintf(tr("%d critical  ·  %d findings / updates  ·  %d incomplete", "%d kritisch  ·  %d Hinweise / Updates  ·  %d ungeklärt"), n.Critical, n.Warnings, n.Unknown)
			if n.evidence == "" {
				line = fmt.Sprintf(tr("Scan findings unavailable  ·  %d incomplete checks", "Scanbefunde offen  ·  %d ungeklärte Prüfungen"), n.Unknown)
			}
			if n.RebootRequired {
				line += "  ·  " + tr("restart indicated", "Neustart gemeldet")
			}
			if n.Source == "observer" && n.evidence != "" {
				line += "  ·  " + tr("observer totals", "Beobachtersummen")
			}
			p.text(42, y+33, 10, false, ink, line)
			p.rule(y + 43)
			y += 68
		}
	}
	p = newPage(tr("Scope and controls", "Prüfumfang und Kontrollen"))
	p.text(42, 167, 15, true, ink, tr("Watcher evidence", "Nachweise des Sicherheitsmonitors"))
	for i, v := range []string{tr("FleetDeck updates and host package status, including restart requirements.", "FleetDeck-Version, Systempakete und gemeldeter Neustartbedarf."), tr("Running container inventory, image findings and runtime permissions.", "Inventar laufender Container, Image-Befunde und Laufzeitberechtigungen."), tr("Private, explicitly enrolled sources. Source age and scan gaps are evaluated.", "Privat eingebundene Quellen. Alter der Messungen und Erfassungslücken werden ausgewiesen.")} {
		p.text(42, 195+i*19, 10, false, ink, v)
	}
	p.rule(254)
	p.text(42, 284, 15, true, ink, tr("Additional controls", "Weitere Kontrollen"))
	p.text(42, 308, 10, false, muted, tr("An unverified control requires separate evidence.", "Für ungeprüfte Kontrollen ist ein eigener Nachweis erforderlich."))
	if len(r.Nodes[0].Controls) == 0 {
		for i, v := range []string{tr("SSH access and multifactor authentication", "SSH-Zugänge und Mehrfaktoranmeldung"), tr("External reachability and firewall rules", "Externe Erreichbarkeit und Firewallregeln"), tr("Offsite backups and successful restoration", "Externe Sicherung und erfolgreiche Wiederherstellung"), tr("Hosting provider integrity", "Integrität des Hostinganbieters")} {
			y := 348 + i*41
			p.text(42, y, 11, false, ink, v)
			p.text(456, y, 10, true, muted, tr("Unverified", "Ungeprüft"))
			p.rule(y + 14)
		}
	} else {
		for i, k := range controlNames {
			passed, failed := 0, 0
			for _, n := range r.Nodes {
				for _, c := range n.Controls {
					if c.Control == k.key {
						if c.Result == "pass" {
							passed++
						}
						if c.Result == "fail" {
							failed++
						}
					}
				}
			}
			y := 348 + i*41
			p.text(42, y, 11, false, ink, tr(k.en, k.de))
			p.text(305, y, 10, true, muted, fmt.Sprintf(tr("%d verified / %d failed / %d unverified", "%d bestätigt / %d fehlerhaft / %d offen"), passed, failed, len(r.Nodes)-passed-failed))
			p.rule(y + 14)
		}
		p.text(42, 521, 10, false, muted, tr("Hosting provider integrity remains unverified.", "Die Integrität des Hostinganbieters bleibt ungeprüft."))
	}
	p.text(42, 556, 15, true, ink, tr("Interpretation", "Einordnung"))
	for i, v := range []string{tr("Counts describe finding occurrences and available updates across observed hosts.", "Die Zahlen erfassen Vorkommen von Befunden und Updates auf den beobachteten Hosts."), tr("One issue may affect multiple hosts or images. Each observation has its own timestamp.", "Ein Befund kann mehrere Hosts oder Images betreffen. Jede Messung hat einen Prüfzeitpunkt."), tr("Host owners can trace findings to the private watcher state and retained source records.", "Einzelbefunde lassen sich über die privaten Prüfdateien und archivierten Nachweise zuordnen."), tr("This operational report does not attest compliance with a certification framework.", "Für eine formale Zertifizierung ist ein gesondertes Audit nach dem jeweiligen Standard nötig.")} {
		p.text(42, 584+i*19, 10, false, ink, v)
	}
	if len(r.Nodes[0].Controls) > 0 {
		rows := 0
		for _, n := range r.Nodes {
			for j, c := range n.Controls {
				if rows%10 == 0 {
					p = newPage(tr("Control evidence", "Kontrollnachweise"))
					p.text(42, 163, 10, false, muted, tr("References identify private evidence by its SHA-256 prefix.", "Referenzen bezeichnen den SHA-256-Präfix des privaten Nachweises."))
				}
				y := 198 + (rows%10)*57
				if j >= len(controlNames) {
					return nil, fmt.Errorf("invalid number of control records")
				}
				kind := controlNames[j]
				p.text(42, y, 10, true, ink, n.Label+"  /  "+tr(kind.en, kind.de))
				p.text(421, y, 9, true, teal, controlStatus(c.Result, de))
				if !c.Observed.IsZero() {
					p.text(42, y+16, 9, false, muted, c.Observed.In(s.Location).Format("2006-01-02 15:04 MST")+"  /  "+tr("valid until ", "gültig bis ")+c.Expires.In(s.Location).Format("2006-01-02 15:04 MST"))
					p.text(42, y+29, 9, false, muted, c.Source+"  /  "+c.Reference[:min(16, len(c.Reference))])
				}
				p.rule(y + 38)
				rows++
			}
		}
	}
	for i, p := range pages {
		p.rule(797)
		p.text(42, 817, 9, false, muted, tr("Internal   /   FleetDeck security operations", "Intern   /   FleetDeck Sicherheitsbetrieb"))
		p.text(499, 817, 9, true, teal, fmt.Sprintf("%d / %d", i+1, len(pages)))
	}
	return encodePDF(pages), nil
}
