package profiles

func init() {
	Register(&Profile{
		Name:        "server",
		Description: "App + PostgreSQL + Redis + automated backups. For APIs and backends.",
		Services: []Service{
			{Name: "app", Image: "custom", Description: "Your application", Required: true},
			{Name: "postgres", Image: "postgres:{{.PostgresVersion}}", Description: "PostgreSQL database", Required: true},
			{Name: "redis", Image: "redis:7-alpine", Description: "Redis cache/queue", Required: false},
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
      DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      REDIS_URL: redis://redis:6379
      PORT: "{{.Port}}"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port={{.Port}}"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    networks:
      - default
      - {{.Name}}-internal

  postgres:
    image: postgres:{{.PostgresVersion}}
    container_name: {{.Name}}-postgres
    restart: always
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes:
      - ./postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - {{.Name}}-internal

  redis:
    image: redis:7-alpine
    container_name: {{.Name}}-redis
    restart: always
    command: redis-server --appendonly yes --maxmemory 256mb --maxmemory-policy allkeys-lru
    volumes:
      - ./redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - {{.Name}}-internal

networks:
  default:
    name: traefik_default
    external: true
  {{.Name}}-internal:
    name: {{.Name}}-internal
    driver: bridge
`,
		EnvTemplate: `POSTGRES_USER={{.Name}}
POSTGRES_PASSWORD={{.Name}}_CHANGEME_$(openssl rand -hex 16)
POSTGRES_DB={{.Name}}
REDIS_URL=redis://redis:6379
PORT={{.Port}}
NODE_ENV=production
`,
	})
}
