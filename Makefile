VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/fleetdeck/fleetdeck/cmd.Version=$(VERSION)"
GO ?= $(shell which go 2>/dev/null || echo "$(HOME)/go-sdk/go/bin/go")

.PHONY: build install clean test test-race lint vet release

build:
	CGO_ENABLED=1 $(GO) build $(LDFLAGS) -o fleetdeck .

install: build
	sudo cp fleetdeck /usr/local/bin/fleetdeck

clean:
	rm -f fleetdeck
	rm -rf dist/

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

release:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 $(GO) build $(LDFLAGS) -o dist/fleetdeck-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 $(GO) build $(LDFLAGS) -o dist/fleetdeck-linux-arm64 .
