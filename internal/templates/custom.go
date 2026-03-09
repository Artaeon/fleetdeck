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
		Compose: `services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    restart: always
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port=8080"
    networks:
      - default

networks:
  default:
    name: traefik_default
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
