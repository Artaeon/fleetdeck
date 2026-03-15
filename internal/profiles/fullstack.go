package profiles

func init() {
	Register(&Profile{
		Name:        "fullstack",
		Description: "Frontend + Backend + DB + Redis + S3. For monorepo SaaS applications.",
		Services: []Service{
			{Name: "frontend", Image: "custom", Description: "Frontend application (Next.js, React, etc.)", Required: true},
			{Name: "backend", Image: "custom", Description: "Backend API server", Required: true},
			{Name: "postgres", Image: "postgres:{{.PostgresVersion}}", Description: "PostgreSQL database", Required: true},
			{Name: "redis", Image: "redis:7-alpine", Description: "Redis cache/sessions", Required: true},
			{Name: "minio", Image: "minio/minio:latest", Description: "S3-compatible object storage", Required: false},
		},
		Compose: `services:
  frontend:
    build:
      context: .
      dockerfile: Dockerfile.frontend
    image: {{.Name}}-frontend:local
    container_name: {{.Name}}-frontend
    restart: always
    environment:
      NEXT_PUBLIC_API_URL: https://api.{{.Domain}}
      PORT: "3000"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port=3000"
    depends_on:
      - backend
    networks:
      - default
      - {{.Name}}-internal

  backend:
    build:
      context: .
      dockerfile: Dockerfile.backend
    image: {{.Name}}-backend:local
    container_name: {{.Name}}-backend
    restart: always
    environment:
      DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      REDIS_URL: redis://redis:6379
      S3_ENDPOINT: http://minio:9000
      S3_ACCESS_KEY: ${MINIO_ROOT_USER}
      S3_SECRET_KEY: ${MINIO_ROOT_PASSWORD}
      S3_BUCKET: ${S3_BUCKET}
      CORS_ORIGIN: https://{{.Domain}}
      PORT: "{{.Port}}"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}-api.rule=Host(` + "`api.{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}-api.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}-api.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}-api.tls=true"
      - "traefik.http.services.{{.Name}}-api.loadbalancer.server.port={{.Port}}"
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
MINIO_ROOT_USER={{.Name}}_admin
MINIO_ROOT_PASSWORD={{.Name}}_minio_CHANGEME_$(openssl rand -hex 16)
S3_BUCKET={{.Name}}-uploads
PORT={{.Port}}
NODE_ENV=production
`,
	})
}
