package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/securityreport"
	"github.com/spf13/cobra"
)

func newSecurityReportCmd() *cobra.Command {
	c := &cobra.Command{Use: "report", Short: "Summarize explicitly supplied private security observations without running new scans", Args: cobra.NoArgs,
		// Pure reports must not initialize a registry, audit writer or templates.
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, _ []string) error {
			values, _ := cmd.Flags().GetStringArray("input")
			observerValues, _ := cmd.Flags().GetStringArray("observer-input")
			var inputs, observers []securityreport.Input
			var err error
			if len(values) > 0 || len(observerValues) == 0 {
				inputs, err = securityreport.Inputs(values)
			}
			if err == nil && len(observerValues) > 0 {
				observers, err = securityreport.Inputs(observerValues)
			}
			if err != nil {
				return err
			}
			language, _ := cmd.Flags().GetString("language")
			if language != "de" && language != "en" {
				return fmt.Errorf("language must be de or en")
			}
			format, _ := cmd.Flags().GetString("format")
			if format != "text" && format != "json" && format != "pdf" {
				return fmt.Errorf("format must be text, json or pdf")
			}
			schedule, err := reportSchedule(cmd)
			if err != nil {
				return err
			}
			if format == "pdf" && schedule.Edition != "weekly" {
				return fmt.Errorf("PDF requires --edition weekly")
			}
			age, _ := cmd.Flags().GetDuration("max-age")
			r, err := securityreport.BuildSources(inputs, observers, time.Now(), age)
			if err != nil {
				return err
			}
			controls, _ := cmd.Flags().GetString("control-evidence")
			key, _ := cmd.Flags().GetString("control-public-key")
			signature, _ := cmd.Flags().GetString("control-signature")
			if controls == "" && (key != "" || signature != "") {
				return fmt.Errorf("signature verification requires --control-evidence")
			}
			if controls != "" {
				if err = r.LoadControls(controls, key, signature); err != nil {
					return err
				}
			}
			if format == "pdf" {
				var data []byte
				data, err = r.WeeklyPDF(schedule, language)
				if err == nil {
					_, err = cmd.OutOrStdout().Write(data)
				}
			} else if format == "json" {
				if schedule.Edition == "weekly" {
					err = json.NewEncoder(cmd.OutOrStdout()).Encode(r.Week(schedule))
				} else {
					err = json.NewEncoder(cmd.OutOrStdout()).Encode(r)
				}
			} else {
				_, err = fmt.Fprint(cmd.OutOrStdout(), r.Text(language, false))
			}
			if err != nil {
				return err
			}
			notify, _ := cmd.Flags().GetBool("notify")
			if !notify {
				return nil
			}
			state, _ := cmd.Flags().GetString("delivery-state")
			if state == "" {
				return fmt.Errorf("notifications require a separate private --delivery-state")
			}
			absolute, err := filepath.Abs(state)
			if err != nil {
				return err
			}
			if controls != "" {
				controlPath, pathErr := filepath.Abs(controls)
				if pathErr != nil || controlPath == absolute {
					return fmt.Errorf("delivery state must be separate from control evidence")
				}
			}
			for _, in := range append(append([]securityreport.Input(nil), inputs...), observers...) {
				p, err := filepath.Abs(in.Path)
				if err != nil {
					return err
				}
				if p == absolute {
					return fmt.Errorf("delivery state must not overwrite a scan input")
				}
			}
			cfg, err = config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("cannot load notification configuration")
			}
			providers, err := alertProviders(cmd)
			if err != nil {
				return err
			}
			if schedule.Edition != "interval" {
				return securityreport.DeliverBriefing(state, r, language, schedule, providers)
			}
			interval, _ := cmd.Flags().GetDuration("interval")
			return securityreport.Deliver(state, r, language, interval, providers)
		},
	}
	c.Flags().StringArray("observer-input", nil, "Enrolled node=/private/observer-record.json from capture-observer (repeatable)")
	c.Flags().StringArray("input", nil, "Explicit neutral-label=/private/scanner-state.json (repeat for each expected host)")
	c.Flags().String("language", "en", "Report language: en or de")
	c.Flags().String("format", "text", "Output: text, json or weekly pdf")
	c.Flags().String("edition", "interval", "Delivery policy: interval, daily changes, or weekly audit")
	c.Flags().String("timezone", "UTC", "IANA timezone for calendar editions")
	c.Flags().Int("hour", 8, "Local delivery hour (0–23); delayed runs catch up")
	c.Flags().String("week-start", "Monday", "First day of the weekly reporting period")
	c.Flags().String("channel", "all", "Send this report via email, telegram, or all configured channels")
	c.Flags().String("first-date", "", "Earliest local period start eligible for a new delivery (YYYY-MM-DD)")
	c.Flags().String("archive-dir", "", "Private create-only archive for weekly email PDFs")
	c.Flags().String("control-public-key", "", "Private file containing a pinned Ed25519 public key (PEM)")
	c.Flags().String("control-signature", "", "Detached binary signature of the exact control evidence file")
	c.Flags().String("control-evidence", "", "Private, dated control-evidence JSON for enrolled hosts")
	c.Flags().Duration("max-age", 26*time.Hour, "Maximum acceptable scan age (at most 26h)")
	c.Flags().Bool("notify", false, "Send a compact report through configured channels")
	c.Flags().String("delivery-state", "", "Separate private report-delivery state file")
	c.Flags().Duration("interval", 7*24*time.Hour, "Minimum interval per successful notification channel")
	c.Flags().String("telegram-config", "", "Private Telegram JSON file")
	c.Flags().String("smtp-config", "", "Private SMTP JSON file")
	return c
}

func reportSchedule(cmd *cobra.Command) (securityreport.Schedule, error) {
	edition, _ := cmd.Flags().GetString("edition")
	archive, _ := cmd.Flags().GetString("archive-dir")
	first, _ := cmd.Flags().GetString("first-date")
	zone, _ := cmd.Flags().GetString("timezone")
	hour, _ := cmd.Flags().GetInt("hour")
	weekday, _ := cmd.Flags().GetString("week-start")
	channel, _ := cmd.Flags().GetString("channel")
	if channel != "all" && channel != "email" && channel != "telegram" {
		return securityreport.Schedule{}, fmt.Errorf("channel must be all, email or telegram")
	}
	if archive != "" && (edition != "weekly" || channel == "telegram" || !filepath.IsAbs(archive)) {
		return securityreport.Schedule{}, fmt.Errorf("PDF archives require a weekly email and an absolute private directory")
	}
	if edition == "interval" {
		return securityreport.Schedule{Edition: edition}, nil
	}
	if zone == "" || zone == "Local" {
		return securityreport.Schedule{}, fmt.Errorf("use an explicit IANA timezone")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return securityreport.Schedule{}, fmt.Errorf("unknown report timezone")
	}
	day := time.Weekday(7)
	for i := time.Sunday; i <= time.Saturday; i++ {
		if strings.EqualFold(i.String(), weekday) {
			day = i
			break
		}
	}
	if edition == "daily" && !cmd.Flags().Changed("hour") {
		hour = 18
	}
	if cmd.Flags().Changed("interval") {
		return securityreport.Schedule{}, fmt.Errorf("--interval belongs to the interval edition")
	}
	s := securityreport.Schedule{FirstDate: first, ArchiveDir: archive, Edition: edition, Hour: hour, Weekday: day, Location: loc}
	return s, s.Validate()
}

func init() { securityCmd.AddCommand(newSecurityReportCmd()) }
