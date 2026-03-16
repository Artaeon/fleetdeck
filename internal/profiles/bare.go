package profiles

func init() {
	Register(&Profile{
		Name:        "bare",
		Description: "App container only with Traefik routing. No database, no extras.",
		Services: []Service{
			{Name: "app", Image: "custom", Description: "Your application", Required: true},
		},
		Compose: `services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    restart: always
    deploy:
      resources:
        limits:
          cpus: '{{.CPULimit}}'
          memory: {{.MemoryLimit}}
    environment:
      PORT: "{{.Port}}"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port={{.Port}}"
    networks:
      - default

networks:
  default:
    name: traefik_default
    external: true
`,
		EnvTemplate: `PORT={{.Port}}
NODE_ENV=production
`,
	})
}
