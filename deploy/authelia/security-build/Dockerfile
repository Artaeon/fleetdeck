# syntax=docker/dockerfile:1
# Reviewed stable source; no application/identity configuration in this context.
FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS source
ADD --checksum=sha256:00dce2ae35720e60b99ed390b148e80b7a5abeba26bdb71193eae6b4b3959423 https://codeload.github.com/authelia/authelia/tar.gz/8da42b2348b0351c6284a2940a9c5eec3d4cd3fd /tmp/source.tar.gz
WORKDIR /src
RUN tar -xzf /tmp/source.tar.gz --strip-components=1 && rm /tmp/source.tar.gz

FROM --platform=$BUILDPLATFORM node:22-alpine3.24@sha256:fe4f6d74e20370acb04ad889077d14d0f64a9c8ea6a42bc60cddf1e844699914 AS frontend
RUN npm install -g pnpm@12.16.1
WORKDIR /src
COPY --from=source /src/web ./web
COPY --from=source /src/internal/server/public_html ./internal/server/public_html
RUN cd web && pnpm install --frozen-lockfile --ignore-scripts && pnpm build

FROM source AS builder
# Go authenticates module downloads with its checksum database. Keep the update
# narrow: stable Authelia source with the patched HTTP/2 library and toolchain.
# Upstream also has a replace directive; go get alone would retain vulnerable code.
RUN go mod edit -dropreplace=golang.org/x/net \
    && go get golang.org/x/net@v0.60.0 \
    && test "$(go list -m -f '{{.Version}}' golang.org/x/net)" = v0.60.0 \
    && test "$(go list -m -f '{{if .Replace}}replaced{{end}}' golang.org/x/net)" = ""
COPY --from=frontend /src/internal/server/public_html ./internal/server/public_html
ENV GOGC=50 CGO_ENABLED=1 CGO_CPPFLAGS="-D_FORTIFY_SOURCE=2 -fstack-protector-strong" CGO_LDFLAGS="-Wl,-z,relro,-z,now"
# Match the official glibc runtime. An Alpine/musl binary cannot run in it.
RUN --mount=type=cache,id=fleetdeck-authelia-go,target=/root/.cache/go-build go build -p 2 -buildmode=pie -trimpath -o /out/authelia \
    -ldflags="-linkmode=external -s -w -X github.com/authelia/authelia/v4/internal/utils.BuildTag=v4.39.28 -X 'github.com/authelia/authelia/v4/internal/utils.BuildState=tagged clean' -X github.com/authelia/authelia/v4/internal/utils.BuildExtra=fleetdeck-security.1 -X github.com/authelia/authelia/v4/internal/utils.BuildCommit=8da42b2348b0351c6284a2940a9c5eec3d4cd3fd" ./cmd/authelia

FROM authelia/authelia:4.39.28@sha256:bd97cff4fcbf715b5ff1f9ae286afbe6033afce385302520b0368122d43a6f54
COPY --from=builder --chmod=0755 /out/authelia /app/authelia
LABEL org.opencontainers.image.description="Operator-built Authelia 4.39.28 with Go 1.27.3 and x/net 0.60.0; not an upstream release"
