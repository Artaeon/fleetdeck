VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/fleetdeck/fleetdeck/cmd.Version=$(VERSION)"

.PHONY: build install clean test

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o fleetdeck .

install: build
	sudo cp fleetdeck /usr/local/bin/fleetdeck

clean:
	rm -f fleetdeck

test:
	go test ./...

release:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build $(LDFLAGS) -o dist/fleetdeck-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build $(LDFLAGS) -o dist/fleetdeck-linux-arm64 .
