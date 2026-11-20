package securityreport

import "time"

type WeekDay struct {
	Date          string `json:"date"`
	ObservedHosts int    `json:"observed_hosts"`
	Critical      *int   `json:"critical"`
	Warnings      *int   `json:"findings_or_updates"`
	Unknown       *int   `json:"incomplete"`
}
type WeeklyReview struct {
	Report
	FirstDay    string         `json:"first_day"`
	LastDay     string         `json:"last_day"`
	Timezone    string         `json:"timezone"`
	Days        []WeekDay      `json:"daily_evidence"`
	DaysPerHost map[string]int `json:"observed_days_per_host"`
}

func (r Report) Week(s Schedule) WeeklyReview {
	p := r.period(s)
	review := WeeklyReview{Report: r, FirstDay: p.first.Format(time.DateOnly), LastDay: p.end.AddDate(0, 0, -1).Format(time.DateOnly), Timezone: s.Location.String(), DaysPerHost: p.byHost}
	for _, d := range p.days {
		day := WeekDay{Date: d.date.Format(time.DateOnly), ObservedHosts: d.observed}
		if d.observed > 0 {
			day.Critical = &d.critical
			day.Warnings = &d.warnings
			day.Unknown = &d.unknown
		}
		review.Days = append(review.Days, day)
	}
	return review
}
