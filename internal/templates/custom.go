package templates

func init() {
	Register(&Template{
		Name:        "custom",
		Description: "Minimal template — bring your own Dockerfile",
		Dockerfile: `FROM alpine:3.19
WORKDIR /app
COPY . .
EXPOSE 8080
CMD ["./start.sh"]
`,
		Compose: `x-fleetdeck:
  edge-network: fleetdeck-{{.Name}}-edge

services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    restart: always
    labels:
      - "traefik.enable=true"
      - "traefik.docker.network=fleetdeck-{{.Name}}-edge"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=letsencrypt"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port=8080"
    networks:
      - default

networks:
  default:
    name: fleetdeck-{{.Name}}-edge
    external: true
`,
		EnvTemplate: `# Add your environment variables here
`,
		GitIgnore: `.env
.env.*
*.log
`,
		Workflow: SharedWorkflow,
	})
}
