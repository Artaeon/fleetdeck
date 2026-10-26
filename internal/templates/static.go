package templates

func init() {
	Register(&Template{
		Name:        "static",
		Description: "Static site served by Nginx",
		Dockerfile: `FROM node:24-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
`,
		Compose: `x-fleetdeck:
  edge-network: fleetdeck-{{.Name}}-edge

services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    restart: always
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - NET_RAW
    pids_limit: 512
    labels:
      - "traefik.enable=true"
      - "traefik.docker.network=fleetdeck-{{.Name}}-edge"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=letsencrypt"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port=80"
    networks:
      - default

networks:
  default:
    name: fleetdeck-{{.Name}}-edge
    external: true
`,
		EnvTemplate: `# No secrets needed for static sites
`,
		GitIgnore: `node_modules/
dist/
.env
*.log
`,
		Workflow: SharedWorkflow,
	})
}
