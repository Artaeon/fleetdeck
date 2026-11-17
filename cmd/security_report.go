package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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
			inputs, err := securityreport.Inputs(values)
			if err != nil {
				return err
			}
			language, _ := cmd.Flags().GetString("language")
			if language != "de" && language != "en" {
				return fmt.Errorf("language must be de or en")
			}
			format, _ := cmd.Flags().GetString("format")
			if format != "text" && format != "json" {
				return fmt.Errorf("format must be text or json")
			}
			age, _ := cmd.Flags().GetDuration("max-age")
			r, err := securityreport.Build(inputs, time.Now(), age)
			if err != nil {
				return err
			}
			if format == "json" {
				err = json.NewEncoder(cmd.OutOrStdout()).Encode(r)
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
			for _, in := range inputs {
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
			interval, _ := cmd.Flags().GetDuration("interval")
			return securityreport.Deliver(state, r, language, interval, providers)
		},
	}
	c.Flags().StringArray("input", nil, "Explicit neutral-label=/private/scanner-state.json (repeat for each expected host)")
	c.Flags().String("language", "en", "Report language: en or de")
	c.Flags().String("format", "text", "Output: text or json")
	c.Flags().Duration("max-age", 26*time.Hour, "Maximum acceptable scan age (at most 26h)")
	c.Flags().Bool("notify", false, "Send a compact report through configured channels")
	c.Flags().String("delivery-state", "", "Separate private report-delivery state file")
	c.Flags().Duration("interval", 7*24*time.Hour, "Minimum interval per successful notification channel")
	c.Flags().String("telegram-config", "", "Private Telegram JSON file")
	c.Flags().String("smtp-config", "", "Private SMTP JSON file")
	return c
}

func init() { securityCmd.AddCommand(newSecurityReportCmd()) }
