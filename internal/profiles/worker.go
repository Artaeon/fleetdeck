package profiles

func init() {
	Register(&Profile{
		Name:        "worker",
		Description: "Background job runner with Redis queue. No HTTP exposure.",
		Services: []Service{
			{Name: "worker", Image: "custom", Description: "Background worker process", Required: true},
			{Name: "redis", Image: "redis:7-alpine", Description: "Redis job queue", Required: true},
			{Name: "postgres", Image: "postgres:{{.PostgresVersion}}", Description: "PostgreSQL database", Required: false},
		},
		Compose: `services:
  worker:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-worker
    restart: always
    environment:
      DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      REDIS_URL: redis://redis:6379
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
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

networks:
  {{.Name}}-internal:
    name: {{.Name}}-internal
    driver: bridge
`,
		EnvTemplate: `POSTGRES_USER={{.Name}}
POSTGRES_PASSWORD={{.Name}}_CHANGEME_$(openssl rand -hex 16)
POSTGRES_DB={{.Name}}
REDIS_URL=redis://redis:6379
`,
	})
}
