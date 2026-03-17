package generate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/detect"
)

func TestDockerfile_NextJS(t *testing.T) {
	result := &detect.Result{
		AppType:   detect.AppTypeNextJS,
		Framework: "Next.js",
		Port:      3000,
	}
	df := Dockerfile(result)

	assertContains(t, df, "FROM node:20-alpine AS deps")
	assertContains(t, df, "FROM node:20-alpine AS builder")
	assertContains(t, df, "FROM node:20-alpine AS runner")
	assertContains(t, df, "NEXT_TELEMETRY_DISABLED=1")
	assertContains(t, df, "adduser --system --uid 1001 nextjs")
	assertContains(t, df, "USER nextjs")
	assertContains(t, df, ".next/standalone")
	assertContains(t, df, ".next/static")
	assertContains(t, df, "EXPOSE 3000")
	assertContains(t, df, `CMD ["node", "server.js"]`)
	assertContains(t, df, "pnpm-lock.yaml")
	assertContains(t, df, "yarn.lock")
}

func TestDockerfile_NextJS_CustomPort(t *testing.T) {
	result := &detect.Result{
		AppType: detect.AppTypeNextJS,
		Port:    4000,
	}
	df := Dockerfile(result)

	assertContains(t, df, "EXPOSE 4000")
	assertContains(t, df, "ENV PORT=4000")
	assertContains(t, df, "http://127.0.0.1:4000/")
}

func TestDockerfile_Node(t *testing.T) {
	result := &detect.Result{
		AppType:    detect.AppTypeNode,
		Framework:  "Express",
		Port:       3000,
		EntryPoint: "src/server.js",
	}
	df := Dockerfile(result)

	assertContains(t, df, "FROM node:20-alpine AS builder")
	assertContains(t, df, "npm run build --if-present")
	assertContains(t, df, "npm prune --production")
	assertContains(t, df, "EXPOSE 3000")
	assertContains(t, df, `CMD ["node", "src/server.js"]`)
}

func TestDockerfile_Node_DefaultEntryPoint(t *testing.T) {
	result := &detect.Result{
		AppType: detect.AppTypeNode,
		Port:    3000,
	}
	df := Dockerfile(result)

	assertContains(t, df, `CMD ["node", "index.js"]`)
}

func TestDockerfile_NestJS(t *testing.T) {
	result := &detect.Result{
		AppType:   detect.AppTypeNestJS,
		Framework: "NestJS",
		Port:      3000,
	}
	df := Dockerfile(result)

	assertContains(t, df, "FROM node:20-alpine AS builder")
	assertContains(t, df, `CMD ["node", "dist/main.js"]`)
	assertContains(t, df, "EXPOSE 3000")
}

func TestDockerfile_Python_FastAPI(t *testing.T) {
	result := &detect.Result{
		AppType:   detect.AppTypePython,
		Framework: "FastAPI",
		Port:      8000,
	}
	df := Dockerfile(result)

	assertContains(t, df, "FROM python:3.12-slim AS builder")
	assertContains(t, df, "FROM python:3.12-slim")
	assertContains(t, df, "requirements.txt")
	assertContains(t, df, "pyproject.toml")
	assertContains(t, df, "EXPOSE 8000")
	assertContains(t, df, "uvicorn")
	assertContains(t, df, `"main:app"`)
}

func TestDockerfile_Python_Django(t *testing.T) {
	result := &detect.Result{
		AppType:   detect.AppTypePython,
		Framework: "Django",
		Port:      8000,
	}
	df := Dockerfile(result)

	assertContains(t, df, "gunicorn")
	assertContains(t, df, "config.wsgi:application")
}

func TestDockerfile_Python_Flask(t *testing.T) {
	result := &detect.Result{
		AppType:   detect.AppTypePython,
		Framework: "Flask",
		Port:      5000,
	}
	df := Dockerfile(result)

	assertContains(t, df, "flask")
	assertContains(t, df, "EXPOSE 5000")
	assertContains(t, df, "http://127.0.0.1:5000/health")
}

func TestDockerfile_Go(t *testing.T) {
	result := &detect.Result{
		AppType: detect.AppTypeGo,
		Port:    8080,
	}
	df := Dockerfile(result)

	assertContains(t, df, "FROM golang:1.23-alpine AS builder")
	assertContains(t, df, "FROM alpine:3.20")
	assertContains(t, df, "go mod download")
	assertContains(t, df, "CGO_ENABLED=0 GOOS=linux")
	assertContains(t, df, `-ldflags="-s -w"`)
	assertContains(t, df, "ca-certificates")
	assertContains(t, df, "EXPOSE 8080")
	assertContains(t, df, `CMD ["./server"]`)
}

func TestDockerfile_Rust(t *testing.T) {
	result := &detect.Result{
		AppType: detect.AppTypeRust,
		Port:    8080,
	}
	df := Dockerfile(result)

	assertContains(t, df, "FROM rust:1.80-alpine AS builder")
	assertContains(t, df, "FROM alpine:3.20")
	assertContains(t, df, "musl-dev")
	assertContains(t, df, "cargo build --release")
	assertContains(t, df, "ca-certificates")
	assertContains(t, df, "EXPOSE 8080")
	assertContains(t, df, `CMD ["./server"]`)
	// Verify dependency caching trick
	assertContains(t, df, `echo "fn main() {}"`)
}

func TestDockerfile_Static(t *testing.T) {
	result := &detect.Result{
		AppType: detect.AppTypeStatic,
		Port:    80,
	}
	df := Dockerfile(result)

	assertContains(t, df, "FROM nginx:alpine")
	assertContains(t, df, "/usr/share/nginx/html")
	assertContains(t, df, "EXPOSE 80")
}

func TestDockerfile_Unknown(t *testing.T) {
	result := &detect.Result{
		AppType: detect.AppTypeUnknown,
		Port:    8080,
	}
	df := Dockerfile(result)

	if df != "" {
		t.Errorf("expected empty string for unknown type, got:\n%s", df)
	}
}

func TestDockerfile_HealthcheckUses127(t *testing.T) {
	types := []struct {
		appType detect.AppType
		port    int
	}{
		{detect.AppTypeNextJS, 3000},
		{detect.AppTypeNode, 3000},
		{detect.AppTypeNestJS, 3000},
		{detect.AppTypePython, 8000},
		{detect.AppTypeGo, 8080},
		{detect.AppTypeRust, 8080},
		{detect.AppTypeStatic, 80},
	}

	for _, tc := range types {
		t.Run(string(tc.appType), func(t *testing.T) {
			result := &detect.Result{
				AppType: tc.appType,
				Port:    tc.port,
			}
			df := Dockerfile(result)

			if strings.Contains(df, "localhost") {
				t.Error("healthcheck must not use 'localhost' — use 127.0.0.1 instead (Alpine IPv6 issue)")
			}
			assertContains(t, df, "127.0.0.1")
			assertContains(t, df, "HEALTHCHECK")
		})
	}
}

func TestDockerfile_CorrectPortFromResult(t *testing.T) {
	types := []detect.AppType{
		detect.AppTypeNextJS,
		detect.AppTypeNode,
		detect.AppTypePython,
		detect.AppTypeGo,
		detect.AppTypeRust,
		detect.AppTypeStatic,
	}

	for _, appType := range types {
		t.Run(string(appType), func(t *testing.T) {
			port := 9999
			result := &detect.Result{
				AppType: appType,
				Port:    port,
			}
			df := Dockerfile(result)

			assertContains(t, df, "EXPOSE 9999")
			assertContains(t, df, fmt.Sprintf("127.0.0.1:%d", port))
		})
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q, but it did not.\nFull output:\n%s", needle, haystack)
	}
}
