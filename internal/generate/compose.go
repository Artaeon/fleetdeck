package generate

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/fleetdeck/fleetdeck/internal/detect"
)

// ComposeOptions holds configuration for docker-compose.yml generation.
type ComposeOptions struct {
	ProjectName string
	Domain      string
	Port        int
	HasDB       bool
	AppType     detect.AppType
}

const composeTmpl = `services:
  app:
    build: .
    restart: unless-stopped
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.ProjectName}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.ProjectName}}.entrypoints=websecure"
      - "traefik.http.routers.{{.ProjectName}}.tls.certresolver=letsencrypt"
      - "traefik.http.routers.{{.ProjectName}}.tls=true"
      - "traefik.http.services.{{.ProjectName}}.loadbalancer.server.port={{.Port}}"
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:{{.Port}}/"]
      interval: 30s
      timeout: 5s
      start_period: 15s
      retries: 3
{{- if .HasVolume}}
    volumes:
      - app-data:/data
{{- end}}
    networks:
      - default

{{- if .HasVolume}}

volumes:
  app-data:
{{- end}}

networks:
  default:
    name: traefik_default
    external: true
`

// composeData is the template rendering context.
type composeData struct {
	ComposeOptions
	HasVolume bool
}

// Compose generates a docker-compose.yml string from the given options.
func Compose(opts ComposeOptions) string {
	needsVolume := opts.HasDB && isSQLiteLikeApp(opts.AppType)

	data := composeData{
		ComposeOptions: opts,
		HasVolume:      needsVolume,
	}

	tmpl, err := template.New("compose").Parse(composeTmpl)
	if err != nil {
		panic(fmt.Sprintf("compose template parse error: %v", err))
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("compose template execute error: %v", err))
	}

	return buf.String()
}

// isSQLiteLikeApp returns true for app types that commonly use SQLite
// or file-based databases and benefit from a persistent volume.
func isSQLiteLikeApp(appType detect.AppType) bool {
	switch appType {
	case detect.AppTypeNextJS, detect.AppTypeNode, detect.AppTypeNestJS:
		return true
	default:
		return false
	}
}
