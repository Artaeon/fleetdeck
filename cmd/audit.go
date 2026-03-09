package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "View audit log entries",
}

var auditShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show recent audit log entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		projectFilter, _ := cmd.Flags().GetString("project")

		logPath := cfg.Audit.LogPath
		if logPath == "" {
			logPath = audit.DefaultLogPath
		}

		entries, err := readAuditEntries(logPath, limit, projectFilter)
		if err != nil {
			return fmt.Errorf("reading audit log: %w", err)
		}

		if len(entries) == 0 {
			ui.Info("No audit entries found")
			return nil
		}

		headers := []string{"TIMESTAMP", "ACTION", "PROJECT", "USER", "SUCCESS", "DETAILS"}
		var rows [][]string
		for _, e := range entries {
			success := "yes"
			if !e.Success {
				success = "NO"
			}
			details := e.Details
			if len(details) > 60 {
				details = details[:57] + "..."
			}
			rows = append(rows, []string{
				e.Timestamp.Format("2006-01-02 15:04:05"),
				e.Action,
				e.Project,
				e.User,
				success,
				details,
			})
		}

		ui.Table(headers, rows)
		return nil
	},
}

// readAuditEntries reads the last N entries from the audit log, optionally filtered by project.
func readAuditEntries(path string, limit int, projectFilter string) ([]audit.AuditEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var all []audit.AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry audit.AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if projectFilter != "" && entry.Project != projectFilter {
			continue
		}
		all = append(all, entry)
	}

	// Return the last `limit` entries
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}

	return all, nil
}

func init() {
	auditShowCmd.Flags().Int("limit", 50, "Number of entries to show")
	auditShowCmd.Flags().String("project", "", "Filter by project name")

	auditCmd.AddCommand(auditShowCmd)
	rootCmd.AddCommand(auditCmd)
}
