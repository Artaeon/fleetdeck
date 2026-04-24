// Package remote defines the plug-in surface for pushing FleetDeck backups
// to off-server storage (S3, B2, GCS, SFTP, etc.). Keeping a copy off the
// server is what turns "I have backups" into actual disaster recovery:
// without it, losing the VPS loses both the running app and every backup
// ever taken of it.
//
// The package intentionally describes a very small interface. Real-world
// object-store SDKs are large and each adds megabytes to the fleetdeck
// binary; by delegating to `rclone` (the initial driver) we get ~50 cloud
// providers for the cost of one tiny subprocess wrapper.
package remote

import (
	"context"
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

// Driver is the contract every off-server storage backend implements.
// Implementations are expected to be stateless and safe to construct once
// per command invocation.
type Driver interface {
	// Name returns a short identifier used in log/audit output.
	Name() string

	// Push uploads the backup at localPath to the remote, placing it under
	// a path derived from the backup ID. The context governs cancellation
	// and timeout; drivers MUST respect ctx.Done().
	Push(ctx context.Context, localPath, backupID string) (remoteLocation string, err error)

	// List returns backup IDs currently present on the remote. Used to
	// detect drift between local records and remote storage.
	List(ctx context.Context) ([]string, error)

	// Delete removes a previously-pushed backup from the remote. Missing
	// objects must NOT be treated as an error — idempotency matters for
	// retention enforcement.
	Delete(ctx context.Context, backupID string) error
}

// ErrNoDriver is returned by Open when the supplied config has an empty
// Driver field, signalling that no remote is configured. Callers should
// treat this as "skip silently" rather than a failure.
var ErrNoDriver = fmt.Errorf("no remote backup driver configured")

// Open constructs a Driver from the backup remote config. It returns
// ErrNoDriver when no driver is configured so callers can branch cleanly
// between "remote disabled" and "remote misconfigured".
func Open(cfg config.BackupRemoteConfig) (Driver, error) {
	if cfg.Driver == "" {
		return nil, ErrNoDriver
	}
	if cfg.Target == "" {
		return nil, fmt.Errorf("backup.remote.driver=%q requires backup.remote.target to be set", cfg.Driver)
	}
	switch cfg.Driver {
	case "rclone":
		return NewRclone(cfg.Target), nil
	default:
		return nil, fmt.Errorf("unknown backup.remote.driver %q (supported: rclone)", cfg.Driver)
	}
}
