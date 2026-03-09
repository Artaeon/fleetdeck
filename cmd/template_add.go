package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fleetdeck/fleetdeck/internal/templates"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var templateAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a custom template from a directory",
	Long: `Imports a custom template from a directory. The directory should contain:
- Dockerfile
- docker-compose.yml (with {{.Name}} and {{.Domain}} placeholders)
- .github/workflows/deploy.yml (optional)
- .env.template (optional)
- .gitignore (optional)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		from, _ := cmd.Flags().GetString("from")

		if from == "" {
			return fmt.Errorf("--from is required")
		}

		if _, err := os.Stat(from); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", from)
		}

		tmpl := &templates.Template{
			Name:        name,
			Description: fmt.Sprintf("Custom template: %s", name),
		}

		// Read each file if it exists
		if data, err := os.ReadFile(filepath.Join(from, "Dockerfile")); err == nil {
			tmpl.Dockerfile = string(data)
		} else {
			return fmt.Errorf("Dockerfile is required in template directory")
		}

		if data, err := os.ReadFile(filepath.Join(from, "docker-compose.yml")); err == nil {
			tmpl.Compose = string(data)
		} else {
			return fmt.Errorf("docker-compose.yml is required in template directory")
		}

		if data, err := os.ReadFile(filepath.Join(from, ".github", "workflows", "deploy.yml")); err == nil {
			tmpl.Workflow = string(data)
		} else {
			tmpl.Workflow = templates.SharedWorkflow
		}

		if data, err := os.ReadFile(filepath.Join(from, ".env.template")); err == nil {
			tmpl.EnvTemplate = string(data)
		} else {
			tmpl.EnvTemplate = "# Add your environment variables here\n"
		}

		if data, err := os.ReadFile(filepath.Join(from, ".gitignore")); err == nil {
			tmpl.GitIgnore = string(data)
		} else {
			tmpl.GitIgnore = ".env\n*.log\n"
		}

		templates.Register(tmpl)

		// Save to disk for persistence
		templateDir := filepath.Join(cfg.Server.BasePath, "templates", name)
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			return fmt.Errorf("creating template directory: %w", err)
		}

		files := map[string]string{
			"Dockerfile":         tmpl.Dockerfile,
			"docker-compose.yml": tmpl.Compose,
			"deploy.yml":         tmpl.Workflow,
			".env.template":      tmpl.EnvTemplate,
			".gitignore":         tmpl.GitIgnore,
		}

		for fname, content := range files {
			if err := os.WriteFile(filepath.Join(templateDir, fname), []byte(content), 0644); err != nil {
				return fmt.Errorf("writing %s: %w", fname, err)
			}
		}

		ui.Success("Template %s added from %s", ui.Bold(name), from)
		ui.Info("Saved to %s", templateDir)
		return nil
	},
}

func init() {
	templateAddCmd.Flags().String("from", "", "Directory to import template from (required)")
	_ = templateAddCmd.MarkFlagRequired("from")

	// Create a "template" parent command
	templateCmd := &cobra.Command{
		Use:     "template",
		Aliases: []string{"tpl"},
		Short:   "Manage templates",
	}
	templateCmd.AddCommand(templateAddCmd)
	rootCmd.AddCommand(templateCmd)
}
