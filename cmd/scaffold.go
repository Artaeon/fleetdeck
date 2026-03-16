package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fleetdeck/fleetdeck/internal/detect"
	"github.com/fleetdeck/fleetdeck/internal/templates"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var scaffoldCmd = &cobra.Command{
	Use:   "scaffold [directory]",
	Short: "Generate Dockerfile and docker-compose.yml for a project",
	Long: `Analyzes your project and generates deployment-ready files:

  - Dockerfile (multi-stage, optimized for the detected framework)
  - docker-compose.yml (with Traefik routing and HTTPS)
  - .env template
  - .gitignore
  - GitHub Actions deploy workflow

The generated files make the project ready for 'fleetdeck deploy'.

Examples:
  fleetdeck scaffold                          # scaffold current directory
  fleetdeck scaffold ./my-app                 # scaffold specific directory
  fleetdeck scaffold --template nextjs        # override auto-detection
  fleetdeck scaffold --domain app.example.com # set domain in compose`,
	Aliases: []string{"init-project"},
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		absDir, err := filepath.Abs(dir)
		if err != nil {
			return err
		}

		info, err := os.Stat(absDir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a valid directory", dir)
		}

		templateName, _ := cmd.Flags().GetString("template")
		domain, _ := cmd.Flags().GetString("domain")
		name, _ := cmd.Flags().GetString("name")
		force, _ := cmd.Flags().GetBool("force")

		// Derive project name from directory
		if name == "" {
			name = filepath.Base(absDir)
		}
		if domain == "" {
			domain = name + ".example.com"
		}

		// Step 1: Detect app type
		ui.Step(1, 3, "Analyzing project...")
		result, err := detect.Detect(absDir)
		if err != nil {
			return fmt.Errorf("detection failed: %w", err)
		}
		ui.Success("Detected: %s %s (confidence: %.0f%%)", result.Language, result.Framework, result.Confidence*100)

		// Determine template
		if templateName == "" {
			switch result.AppType {
			case detect.AppTypeNextJS:
				templateName = "nextjs"
			case detect.AppTypeNestJS:
				templateName = "nestjs"
			case detect.AppTypeNode:
				templateName = "node"
			case detect.AppTypePython:
				templateName = "python"
			case detect.AppTypeGo:
				templateName = "golang"
			case detect.AppTypeStatic:
				templateName = "static"
			default:
				templateName = "custom"
			}
			ui.Info("Auto-selected template: %s", templateName)
		}

		tmpl, err := templates.Get(templateName)
		if err != nil {
			return fmt.Errorf("template %q not found; available templates: nextjs, nestjs, node, python, golang, static, custom", templateName)
		}

		// Step 2: Check for existing files
		ui.Step(2, 3, "Checking existing files...")
		existingFiles := []string{}
		filesToWrite := map[string]bool{
			"Dockerfile":        tmpl.Dockerfile != "",
			"docker-compose.yml": tmpl.Compose != "",
			".gitignore":        tmpl.GitIgnore != "",
		}

		for file, shouldWrite := range filesToWrite {
			if !shouldWrite {
				continue
			}
			path := filepath.Join(absDir, file)
			if _, err := os.Stat(path); err == nil {
				existingFiles = append(existingFiles, file)
			}
		}

		if len(existingFiles) > 0 && !force {
			ui.Warn("The following files already exist:")
			for _, f := range existingFiles {
				fmt.Printf("  - %s\n", f)
			}
			return fmt.Errorf("use --force to overwrite existing files")
		}

		// Step 3: Generate files
		ui.Step(3, 3, "Generating deployment files...")

		data := templates.TemplateData{
			Name:            name,
			Domain:          domain,
			PostgresVersion: "16",
		}
		if cfg != nil && cfg.Defaults.PostgresVersion != "" {
			data.PostgresVersion = cfg.Defaults.PostgresVersion
		}

		filesWritten := 0

		// Write Dockerfile
		if tmpl.Dockerfile != "" {
			content, err := templates.Render(tmpl.Dockerfile, data)
			if err != nil {
				return fmt.Errorf("rendering Dockerfile: %w", err)
			}
			if err := os.WriteFile(filepath.Join(absDir, "Dockerfile"), []byte(content), 0644); err != nil {
				return fmt.Errorf("writing Dockerfile: %w", err)
			}
			ui.Success("Created Dockerfile")
			filesWritten++
		}

		// Write docker-compose.yml
		if tmpl.Compose != "" {
			content, err := templates.Render(tmpl.Compose, data)
			if err != nil {
				return fmt.Errorf("rendering docker-compose.yml: %w", err)
			}
			if err := os.WriteFile(filepath.Join(absDir, "docker-compose.yml"), []byte(content), 0644); err != nil {
				return fmt.Errorf("writing docker-compose.yml: %w", err)
			}
			ui.Success("Created docker-compose.yml")
			filesWritten++
		}

		// Write .env
		if tmpl.EnvTemplate != "" {
			envPath := filepath.Join(absDir, ".env")
			if _, err := os.Stat(envPath); os.IsNotExist(err) || force {
				content, err := templates.Render(tmpl.EnvTemplate, data)
				if err != nil {
					return fmt.Errorf("rendering .env: %w", err)
				}
				if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
					return fmt.Errorf("writing .env: %w", err)
				}
				ui.Success("Created .env")
				filesWritten++
			} else {
				ui.Info("Skipped .env (already exists)")
			}
		}

		// Write .gitignore
		if tmpl.GitIgnore != "" {
			content := tmpl.GitIgnore
			if err := os.WriteFile(filepath.Join(absDir, ".gitignore"), []byte(content), 0644); err != nil {
				return fmt.Errorf("writing .gitignore: %w", err)
			}
			ui.Success("Created .gitignore")
			filesWritten++
		}

		// Write GitHub Actions workflow
		if tmpl.Workflow != "" {
			workflowDir := filepath.Join(absDir, ".github", "workflows")
			if err := os.MkdirAll(workflowDir, 0755); err != nil {
				return fmt.Errorf("creating workflow directory: %w", err)
			}
			if err := os.WriteFile(filepath.Join(workflowDir, "deploy.yml"), []byte(tmpl.Workflow), 0644); err != nil {
				return fmt.Errorf("writing deploy workflow: %w", err)
			}
			ui.Success("Created .github/workflows/deploy.yml")
			filesWritten++
		}

		fmt.Println()
		ui.Success("Scaffolded %d files for %s (%s template)", filesWritten, name, templateName)
		fmt.Println()
		ui.Info("Next steps:")
		fmt.Printf("  1. Review and customize the generated files\n")
		fmt.Printf("  2. Update .env with your actual values\n")
		fmt.Printf("  3. Deploy: fleetdeck deploy %s --server user@host --domain %s\n", dir, domain)
		fmt.Println()

		return nil
	},
}

func init() {
	scaffoldCmd.Flags().String("template", "", "Template to use (auto-detected if not set)")
	scaffoldCmd.Flags().String("domain", "", "Domain for docker-compose.yml (defaults to <name>.example.com)")
	scaffoldCmd.Flags().String("name", "", "Project name (defaults to directory name)")
	scaffoldCmd.Flags().Bool("force", false, "Overwrite existing files")

	rootCmd.AddCommand(scaffoldCmd)
}
