package templates

func init() {
	Register(&Template{
		Name:        "nestjs",
		Description: "NestJS application with Prisma and PostgreSQL",
		Dockerfile: `FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
COPY prisma ./prisma
RUN npm ci
RUN npx prisma generate
COPY . .
RUN npm run build

FROM node:20-alpine
WORKDIR /app
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/prisma ./prisma
RUN npm ci --omit=dev
RUN npx prisma generate
COPY --from=builder /app/dist ./dist
EXPOSE 3000
CMD ["node", "dist/main.js"]
`,
		Compose: `services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    restart: always
    environment:
      DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      NODE_ENV: production
      PORT: "3000"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port=3000"
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
		GitIgnore: `node_modules/
dist/
.env
.env.*
*.log
`,
		Workflow: SharedWorkflow,
	})
}
