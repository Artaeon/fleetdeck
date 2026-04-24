package remote

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Rclone implements Driver by shelling out to the `rclone` binary. Rclone
// is chosen as the default driver because it supports S3, B2, Cloudflare R2,
// GCS, Azure, SFTP, and WebDAV through a single CLI, and it's already
// present on most admin workstations.
//
// The driver assumes the operator has already run `rclone config` to create
// a named remote (e.g. "b2" or "r2"); Target in config.toml is the usual
// `remote:path` pair passed to rclone.
type Rclone struct {
	// Target is the rclone destination, e.g. "b2:my-fleet-backups" or
	// "r2:backups/production".
	Target string

	// Binary is the rclone executable. Exposed for tests; defaults to
	// "rclone" resolved via $PATH.
	Binary string
}

// NewRclone builds a Driver pointed at target. Target must be in
// `remote:path` form as expected by rclone.
func NewRclone(target string) *Rclone {
	return &Rclone{Target: target, Binary: "rclone"}
}

// Name implements Driver.
func (r *Rclone) Name() string { return "rclone(" + r.Target + ")" }

// remotePath joins the configured target with a backup ID, producing an
// rclone-style path like "b2:my-fleet-backups/<id>".
func (r *Rclone) remotePath(backupID string) string {
	// rclone's remote form is `name:path`. If the user wrote `name:` (no
	// subpath) we concatenate directly; otherwise we add a slash.
	t := r.Target
	if strings.HasSuffix(t, ":") || strings.HasSuffix(t, "/") {
		return t + backupID
	}
	return t + "/" + backupID
}

// Push implements Driver. Uses `rclone copy` so partial uploads are
// resumable and already-uploaded files are skipped.
func (r *Rclone) Push(ctx context.Context, localPath, backupID string) (string, error) {
	dest := r.remotePath(backupID)
	// --checksum keeps the transfer honest even if mtimes differ between
	// local fs and remote; --transfers=4 balances speed vs VPS bandwidth.
	cmd := exec.CommandContext(ctx, r.Binary, "copy",
		"--checksum",
		"--transfers=4",
		"--stats=0",
		localPath, dest,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rclone copy %s -> %s: %w (%s)", localPath, dest, err, strings.TrimSpace(stderr.String()))
	}
	return dest, nil
}

// List implements Driver by running `rclone lsd` against the configured
// target and returning the directory names (each backup is stored as its
// own directory named after the backup ID).
func (r *Rclone) List(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, r.Binary, "lsjson", r.Target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rclone lsjson %s: %w (%s)", r.Target, err, strings.TrimSpace(stderr.String()))
	}
	// Parse a minimal subset of rclone lsjson output to avoid pulling in
	// a struct tag just for two fields.
	var ids []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		name := extractLsjsonName(line)
		if name != "" {
			ids = append(ids, name)
		}
	}
	return ids, nil
}

// Delete implements Driver. Missing objects are silently ignored because
// retention enforcement runs repeatedly and must be idempotent.
func (r *Rclone) Delete(ctx context.Context, backupID string) error {
	dest := r.remotePath(backupID)
	cmd := exec.CommandContext(ctx, r.Binary, "purge", dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// rclone returns exit 3 for "directory not found" in most backends;
		// treat that as success so a double-delete doesn't fail the caller.
		if strings.Contains(stderr.String(), "directory not found") ||
			strings.Contains(stderr.String(), "not found") {
			return nil
		}
		return fmt.Errorf("rclone purge %s: %w (%s)", dest, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// extractLsjsonName pulls the Name field out of a single rclone lsjson
// line without importing encoding/json for one field. Lines look like:
//
//	[
//	{"Path":"abc","Name":"abc","Size":-1,"MimeType":"inode/directory","ModTime":"...","IsDir":true},
//	...
//	]
//
// We skip non-object lines (`[`, `]`) and non-directory entries.
func extractLsjsonName(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ",")
	if !strings.HasPrefix(line, "{") {
		return ""
	}
	if !strings.Contains(line, `"IsDir":true`) {
		return ""
	}
	const key = `"Name":"`
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
