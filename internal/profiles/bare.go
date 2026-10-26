package profiles

func init() {
	Register(&Profile{
		Name:          "bare",
		Description:   "App container only with Traefik routing. No database, no extras.",
		DefaultCPU:    "1.0",
		DefaultMemory: "512M",
		Services: []Service{
			{Name: "app", Image: "custom", Description: "Your application", Required: true},
		},
		Compose: `x-fleetdeck:
  edge-network: fleetdeck-{{.Name}}-edge

services:
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
      - "traefik.docker.network=fleetdeck-{{.Name}}-edge"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=letsencrypt"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port={{.Port}}"
    networks:
      - default

networks:
  default:
    name: fleetdeck-{{.Name}}-edge
    external: true
`,
		EnvTemplate: `PORT={{.Port}}
NODE_ENV=production
`,
	})
}
