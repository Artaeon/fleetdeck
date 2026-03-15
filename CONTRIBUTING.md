# Contributing to FleetDeck

Thank you for your interest in contributing to FleetDeck! This document provides guidelines and information for contributors.

## Development Setup

```bash
git clone https://github.com/Artaeon/fleetdeck.git
cd fleetdeck
make build
```

**Requirements:** Go 1.23+, Docker with Compose v2 (for integration tests).

## Running Tests

```bash
# All tests
make test

# Specific package
go test ./internal/detect/...
go test ./internal/profiles/...

# With verbose output
go test -v ./...

# With race detection
go test -race ./...
```

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Use `fmt.Errorf("context: %w", err)` for error wrapping
- Prefer table-driven tests
- No unnecessary comments -- code should be self-explanatory

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `main`
3. Write tests for new functionality
4. Ensure all tests pass
5. Submit a pull request with a clear description

## Adding a New Deployment Profile

1. Create a new file in `internal/profiles/` (e.g., `myprofile.go`)
2. Register the profile in an `init()` function using `Register()`
3. Add tests in `internal/profiles/profiles_test.go`
4. Update the README profile table

## Adding a New DNS Provider

1. Implement the `Provider` interface from `internal/dns/dns.go`
2. Create a new file in `internal/dns/` (e.g., `hetzner.go`)
3. Register the provider in the `GetProvider` factory function
4. Add tests

## Reporting Issues

Use [GitHub Issues](https://github.com/Artaeon/fleetdeck/issues) to report bugs or request features. Please include:

- FleetDeck version
- Operating system and version
- Steps to reproduce
- Expected vs actual behavior

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
