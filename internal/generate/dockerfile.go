package generate

import (
	"fmt"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/detect"
)

// Dockerfile generates a production-ready, multi-stage Dockerfile based on the
// detected application type. It returns an empty string for unknown types.
func Dockerfile(result *detect.Result) string {
	port := result.Port

	switch result.AppType {
	case detect.AppTypeNextJS:
		return dockerfileNextJS(port)
	case detect.AppTypeNestJS:
		return dockerfileNode(port, "dist/main.js")
	case detect.AppTypeNode:
		return dockerfileNode(port, entryPointOrDefault(result.EntryPoint, "index.js"))
	case detect.AppTypePython:
		return dockerfilePython(port, result.Framework)
	case detect.AppTypeGo:
		return dockerfileGo(port)
	case detect.AppTypeRust:
		return dockerfileRust(port)
	case detect.AppTypeStatic:
		return dockerfileStatic(port)
	default:
		return ""
	}
}

func entryPointOrDefault(ep, def string) string {
	if ep != "" {
		return ep
	}
	return def
}

func healthcheck(port int, path string) string {
	return fmt.Sprintf(
		"HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \\\n"+
			"  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:%d%s || exit 1",
		port, path,
	)
}

func dockerfileNextJS(port int) string {
	var b strings.Builder
	b.WriteString(`FROM node:20-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* yarn.lock* pnpm-lock.yaml* ./
RUN if [ -f package-lock.json ]; then npm ci; \
    elif [ -f yarn.lock ]; then yarn install --frozen-lockfile; \
    elif [ -f pnpm-lock.yaml ]; then corepack enable && pnpm install --frozen-lockfile; \
    else npm install; fi

FROM node:20-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
USER nextjs
`)
	fmt.Fprintf(&b, "EXPOSE %d\n", port)
	fmt.Fprintf(&b, "ENV PORT=%d\n", port)
	b.WriteString("ENV HOSTNAME=\"0.0.0.0\"\n")
	b.WriteString(healthcheck(port, "/"))
	b.WriteString("\nCMD [\"node\", \"server.js\"]\n")
	return b.String()
}

func dockerfileNode(port int, entrypoint string) string {
	var b strings.Builder
	b.WriteString(`FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* yarn.lock* pnpm-lock.yaml* ./
RUN if [ -f package-lock.json ]; then npm ci; \
    elif [ -f yarn.lock ]; then yarn install --frozen-lockfile; \
    elif [ -f pnpm-lock.yaml ]; then corepack enable && pnpm install --frozen-lockfile; \
    else npm install; fi
COPY . .
RUN npm run build --if-present

FROM node:20-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app ./
RUN npm prune --production
`)
	fmt.Fprintf(&b, "EXPOSE %d\n", port)
	b.WriteString(healthcheck(port, "/"))
	fmt.Fprintf(&b, "\nCMD [\"node\", \"%s\"]\n", entrypoint)
	return b.String()
}

func dockerfilePython(port int, framework string) string {
	var b strings.Builder
	b.WriteString(`FROM python:3.12-slim AS builder
WORKDIR /app
COPY requirements.txt* pyproject.toml* ./
RUN pip install --no-cache-dir -r requirements.txt 2>/dev/null || pip install --no-cache-dir .

FROM python:3.12-slim
WORKDIR /app
COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY . .
`)
	fmt.Fprintf(&b, "EXPOSE %d\n", port)
	b.WriteString(pythonHealthcheck(port))
	b.WriteString("\n")
	b.WriteString(pythonCMD(port, framework))
	b.WriteString("\n")
	return b.String()
}

func pythonHealthcheck(port int) string {
	return fmt.Sprintf(
		"HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \\\n"+
			"  CMD python -c \"import urllib.request; urllib.request.urlopen('http://127.0.0.1:%d/health')\" || exit 1",
		port,
	)
}

func pythonCMD(port int, framework string) string {
	switch framework {
	case "Django":
		return fmt.Sprintf("CMD [\"gunicorn\", \"--bind\", \"0.0.0.0:%d\", \"config.wsgi:application\"]", port)
	case "Flask":
		return fmt.Sprintf("CMD [\"flask\", \"run\", \"--host\", \"0.0.0.0\", \"--port\", \"%d\"]", port)
	default:
		// FastAPI or generic Python
		return fmt.Sprintf("CMD [\"uvicorn\", \"main:app\", \"--host\", \"0.0.0.0\", \"--port\", \"%d\"]", port)
	}
}

func dockerfileGo(port int) string {
	var b strings.Builder
	b.WriteString(`FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server .

FROM alpine:3.20
WORKDIR /app
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/server .
`)
	fmt.Fprintf(&b, "EXPOSE %d\n", port)
	b.WriteString(healthcheck(port, "/health"))
	b.WriteString("\nCMD [\"./server\"]\n")
	return b.String()
}

func dockerfileRust(port int) string {
	var b strings.Builder
	b.WriteString(`FROM rust:1.80-alpine AS builder
WORKDIR /app
RUN apk add --no-cache musl-dev
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo "fn main() {}" > src/main.rs && cargo build --release && rm -rf src
COPY . .
RUN cargo build --release

FROM alpine:3.20
WORKDIR /app
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/target/release/* /app/server
`)
	fmt.Fprintf(&b, "EXPOSE %d\n", port)
	b.WriteString(healthcheck(port, "/health"))
	b.WriteString("\nCMD [\"./server\"]\n")
	return b.String()
}

func dockerfileStatic(port int) string {
	var b strings.Builder
	b.WriteString("FROM nginx:alpine\n")
	b.WriteString("COPY . /usr/share/nginx/html\n")
	fmt.Fprintf(&b, "EXPOSE %d\n", port)
	b.WriteString(fmt.Sprintf(
		"HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \\\n"+
			"  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:%d/ || exit 1\n",
		port,
	))
	return b.String()
}
