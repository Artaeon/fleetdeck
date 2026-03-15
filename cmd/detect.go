package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fleetdeck/fleetdeck/internal/detect"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var detectCmd = &cobra.Command{
	Use:   "detect [directory]",
	Short: "Auto-detect application type and recommend a deployment profile",
	Long: `Analyzes a project directory to detect:
- Application type (Node.js, Next.js, Python, Go, Rust, etc.)
- Framework (Express, FastAPI, Gin, etc.)
- Required services (PostgreSQL, Redis, S3)
- Recommended deployment profile

If no directory is specified, the current directory is used.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a valid directory", dir)
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")

		result, err := detect.Detect(dir)
		if err != nil {
			return fmt.Errorf("detection failed: %w", err)
		}

		if jsonOutput {
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		// Pretty output
		fmt.Println()
		ui.Success("Application detected!")
		fmt.Println()

		headers := []string{"Property", "Value"}
		rows := [][]string{
			{"Type", string(result.AppType)},
			{"Language", result.Language},
		}
		if result.Framework != "" {
			rows = append(rows, []string{"Framework", result.Framework})
		}
		if result.EntryPoint != "" {
			rows = append(rows, []string{"Entry Point", result.EntryPoint})
		}
		if result.Port > 0 {
			rows = append(rows, []string{"Port", fmt.Sprintf("%d", result.Port)})
		}
		rows = append(rows, []string{"Database", boolToYesNo(result.HasDB)})
		rows = append(rows, []string{"Redis", boolToYesNo(result.HasRedis)})
		rows = append(rows, []string{"Docker", boolToYesNo(result.HasDocker)})
		rows = append(rows, []string{"Confidence", fmt.Sprintf("%.0f%%", result.Confidence*100)})
		ui.Table(headers, rows)

		fmt.Println()
		ui.Info("Recommended profile: %s", ui.Bold(result.Profile))
		fmt.Println()
		ui.Info("Deploy with:")
		fmt.Printf("  fleetdeck deploy %s --profile %s --domain <your-domain>\n\n", dir, result.Profile)

		if len(result.Indicators) > 0 {
			ui.Info("Detection indicators:")
			for _, ind := range result.Indicators {
				fmt.Printf("  - %s\n", ind)
			}
			fmt.Println()
		}

		return nil
	},
}

func boolToYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func init() {
	detectCmd.Flags().Bool("json", false, "Output detection results as JSON")
	rootCmd.AddCommand(detectCmd)
}
