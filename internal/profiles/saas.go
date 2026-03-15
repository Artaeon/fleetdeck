package profiles

func init() {
	Register(&Profile{
		Name:        "saas",
		Description: "Full SaaS stack: App + PostgreSQL + Redis + S3 (MinIO) + email relay + cron.",
		Services: []Service{
			{Name: "app", Image: "custom", Description: "Your application", Required: true},
			{Name: "postgres", Image: "postgres:{{.PostgresVersion}}", Description: "PostgreSQL database", Required: true},
			{Name: "redis", Image: "redis:7-alpine", Description: "Redis cache/sessions/queue", Required: true},
			{Name: "minio", Image: "minio/minio:latest", Description: "S3-compatible object storage", Required: false},
			{Name: "mailpit", Image: "axllent/mailpit:latest", Description: "Email testing (dev) / SMTP relay", Required: false},
		},
		Compose: `services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    restart: always
    environment:
      DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      REDIS_URL: redis://redis:6379
      S3_ENDPOINT: http://minio:9000
      S3_ACCESS_KEY: ${MINIO_ROOT_USER}
      S3_SECRET_KEY: ${MINIO_ROOT_PASSWORD}
      S3_BUCKET: ${S3_BUCKET}
      SMTP_HOST: mailpit
      SMTP_PORT: "1025"
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
      minio:
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
    command: redis-server --appendonly yes --maxmemory 512mb --maxmemory-policy allkeys-lru
    volumes:
      - ./redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - {{.Name}}-internal

  minio:
    image: minio/minio:latest
    container_name: {{.Name}}-minio
    restart: always
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
    volumes:
      - ./minio_data:/data
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 10s
      timeout: 5s
      retries: 5
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}-s3.rule=Host(` + "`s3.{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}-s3.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}-s3.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}-s3.tls=true"
      - "traefik.http.services.{{.Name}}-s3.loadbalancer.server.port=9000"
    networks:
      - default
      - {{.Name}}-internal

  mailpit:
    image: axllent/mailpit:latest
    container_name: {{.Name}}-mailpit
    restart: always
    environment:
      MP_SMTP_AUTH_ACCEPT_ANY: "true"
      MP_SMTP_AUTH_ALLOW_INSECURE: "true"
    volumes:
      - ./mailpit_data:/data
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}-mail.rule=Host(` + "`mail.{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}-mail.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}-mail.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}-mail.tls=true"
      - "traefik.http.services.{{.Name}}-mail.loadbalancer.server.port=8025"
    networks:
      - default
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
MINIO_ROOT_USER={{.Name}}_admin
MINIO_ROOT_PASSWORD={{.Name}}_minio_CHANGEME_$(openssl rand -hex 16)
S3_BUCKET={{.Name}}-uploads
SMTP_HOST=mailpit
SMTP_PORT=1025
PORT={{.Port}}
NODE_ENV=production
`,
	})
}
