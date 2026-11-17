package securitywatch

import "time"

// Observation retains bounded trends, not copies of scanner output or secrets.
type Observation struct {
	CheckedAt time.Time `json:"checked_at"`
	Critical  int       `json:"critical"`
	Warnings  int       `json:"warnings"`
	Unknown   int       `json:"unknown"`
}

func (s *State) Record(report Report) {
	s.Report = report
	if report.CheckedAt.IsZero() {
		return
	}
	if len(s.History) > 0 && !report.CheckedAt.After(s.History[len(s.History)-1].CheckedAt) {
		return
	}
	c, w, u := report.Counts()
	s.History = append(s.History, Observation{report.CheckedAt, c, w, u})
	if len(s.History) > 56 {
		s.History = append([]Observation(nil), s.History[len(s.History)-56:]...)
	}
}
