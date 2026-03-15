# Contributing to FleetDeck

Contributions are welcome. Here's how to get started.

## Development Setup

```bash
git clone https://github.com/Artaeon/fleetdeck.git
cd fleetdeck
make build
```

**Requirements:** Go 1.23+, Docker with Compose v2 (for integration tests).

## Running Tests

```bash
# All unit tests
make test

# With race detection (what CI runs)
make test-race

# With coverage report
make test-cover
open coverage.html

# Integration tests (requires Docker)
FLEETDECK_INTEGRATION=true go test -tags=integration -v ./internal/deploy/...

# Specific package
go test -v ./internal/detect/...
```

The project has **883 test functions** across **59 test files**. All tests must pass before submitting a PR.

## Code Style

- Standard Go conventions (`gofmt`, `go vet`, `golangci-lint`)
- Short functions, clear names, minimal comments
- `fmt.Errorf("context: %w", err)` for error wrapping
- Table-driven tests
- Shell commands must use `shellQuote()` for user input
- Validate all user input at system boundaries

## Pull Request Process

1. Fork the repository and create a feature branch from `main`
2. Write tests for new functionality (aim for >80% coverage on new code)
3. Run `make test-race` and ensure all tests pass
4. Run `go vet ./...` with no warnings
5. Submit a PR with a clear description of what and why

## Adding a Deployment Profile

1. Create `internal/profiles/myprofile.go` with an `init()` function that calls `Register()`
2. Define the profile's services, Docker Compose template, and env template
3. Add tests in `internal/profiles/profiles_test.go` and `render_test.go`
4. Update the README profile table

## Adding a DNS Provider

1. Implement the `Provider` interface from `internal/dns/dns.go`
2. Create `internal/dns/hetzner.go` (or your provider)
3. Add the provider to the `GetProvider` factory in `dns.go`
4. Add tests using `httptest` for API mocking
5. Update the README

## Adding a Detection Rule

1. Add your detector function in `internal/detect/detect.go`
2. Insert it in the detector chain (order matters -- more specific first)
3. Add tests in `detect_test.go` or `integration_test.go`

## Project Structure

```
cmd/           CLI commands (Cobra)
internal/
  detect/      App type detection
  profiles/    Deployment profiles
  remote/      SSH client + file transfer
  bootstrap/   Server provisioning
  deploy/      Deployment strategies + locking
  monitor/     Health monitoring + alerting + state persistence
  dns/         DNS providers + multi-level TLD support
  environments/ Staging/production management
  backup/      Backup engine + retention
  config/      TOML configuration
  crypto/      AES-256-GCM encryption
  db/          SQLite + CRUD
  project/     Project operations (users, SSH, scaffold)
  server/      Web dashboard + REST API + webhooks
```

## Reporting Issues

[GitHub Issues](https://github.com/Artaeon/fleetdeck/issues). Include: FleetDeck version, OS, steps to reproduce, expected vs actual behavior.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
