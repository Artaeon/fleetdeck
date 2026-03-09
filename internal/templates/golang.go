package templates

func init() {
	Register(&Template{
		Name:        "go",
		Description: "Go application with PostgreSQL",
		Dockerfile: `FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
`,
		Compose: `services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    restart: always
    environment:
      DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      PORT: "8080"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port=8080"
    depends_on:
      postgres:
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
      test: ["CMD-SHELL", "pg_isready"]
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
POSTGRES_PASSWORD={{.Name}}_CHANGEME
POSTGRES_DB={{.Name}}
`,
		GitIgnore: `*.exe
*.dll
*.so
*.dylib
.env
.env.*
*.log
/server
`,
		Workflow: SharedWorkflow,
	})
}
