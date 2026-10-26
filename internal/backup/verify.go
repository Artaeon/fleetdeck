package backup

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// VerifyStatus represents the outcome of verifying a single backup component.
type VerifyStatus string

const (
	VerifyOK      VerifyStatus = "ok"
	VerifyMissing VerifyStatus = "missing"
	VerifyFailed  VerifyStatus = "failed"
)

// VerifyResult holds the verification outcome for one backup component.
type VerifyResult struct {
	Component ComponentInfo
	Status    VerifyStatus
	Error     error
}

// VerifyBackup reads the manifest from backupPath and verifies every component
// file exists and passes integrity checks. Config files are validated by
// recomputing SHA256 and comparing against the manifest checksum. Database
// dumps (.gz) and volume archives (.tar.gz) are validated by checking that
// they are valid gzip streams.
func VerifyBackup(backupPath string) ([]VerifyResult, error) {
	manifest, err := ReadManifest(backupPath)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	return verifyManifest(backupPath, manifest), nil
}

func verifyManifest(backupPath string, manifest *Manifest) []VerifyResult {
	var results []VerifyResult
	for _, comp := range manifest.Components {
		result := verifyComponent(backupPath, comp)
		results = append(results, result)
	}

	return results
}

func verifyComponent(backupPath string, comp ComponentInfo) VerifyResult {
	failed := func(err error) VerifyResult { return VerifyResult{Component: comp, Status: VerifyFailed, Error: err} }
	if err := validateComponent(comp); err != nil {
		return failed(err)
	}
	root, err := os.OpenRoot(backupPath)
	if err != nil {
		return failed(err)
	}
	defer root.Close()
	if err := checkPath(root, comp.Path, false, false); err != nil {
		if os.IsNotExist(err) {
			return VerifyResult{Component: comp, Status: VerifyMissing, Error: err}
		}
		return failed(err)
	}
	f, err := root.Open(comp.Path)
	if err != nil {
		return failed(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return failed(err)
	}
	if !info.Mode().IsRegular() {
		return failed(fmt.Errorf("component is not a regular file"))
	}
	if comp.Type == "config" {
		return verifyConfigFile(f, comp)
	}
	// Older volume/database components have no digest. When one is present,
	// validate it as well as gzip integrity: a different valid gzip is not the
	// snapshot originally recorded in the manifest.
	if comp.Checksum != "" {
		result := verifyConfigFile(f, comp)
		if result.Status != VerifyOK {
			return result
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return failed(err)
		}
	}
	return verifyGzipFile(f, comp)
}

// verifyConfigFile recomputes the SHA256 checksum and compares it to the
// manifest value. A missing checksum in the manifest is treated as a
// verification failure, not a silent pass — otherwise an attacker who
// tampered with both a config file AND the manifest (removing the
// Checksum field) would sail through integrity checks.
func verifyConfigFile(reader io.Reader, comp ComponentInfo) VerifyResult {
	if comp.Checksum == "" {
		return VerifyResult{
			Component: comp,
			Status:    VerifyFailed,
			Error:     fmt.Errorf("manifest has no checksum for %s — cannot verify integrity", comp.Name),
		}
	}

	h := sha256.New()
	_, err := io.Copy(h, reader)
	checksum := hex.EncodeToString(h.Sum(nil))
	if err != nil {
		return VerifyResult{
			Component: comp,
			Status:    VerifyFailed,
			Error:     fmt.Errorf("computing checksum: %w", err),
		}
	}

	if checksum != comp.Checksum {
		return VerifyResult{
			Component: comp,
			Status:    VerifyFailed,
			Error:     fmt.Errorf("checksum mismatch: expected %s, got %s", comp.Checksum, checksum),
		}
	}

	return VerifyResult{Component: comp, Status: VerifyOK}
}

// verifyGzipFile opens the file and reads through the gzip stream to confirm
// the archive is not corrupt.
func verifyGzipFile(reader io.Reader, comp ComponentInfo) VerifyResult {
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return VerifyResult{
			Component: comp,
			Status:    VerifyFailed,
			Error:     fmt.Errorf("invalid gzip: %w", err),
		}
	}
	defer gz.Close()

	// Read through the entire stream to verify integrity
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return VerifyResult{
			Component: comp,
			Status:    VerifyFailed,
			Error:     fmt.Errorf("corrupt gzip data: %w", err),
		}
	}

	return VerifyResult{Component: comp, Status: VerifyOK}
}

// fileSHA256 computes the hex-encoded SHA256 digest of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CountResults tallies verification outcomes by status.
func CountResults(results []VerifyResult) (total, ok, failed, missing int) {
	total = len(results)
	for _, r := range results {
		switch r.Status {
		case VerifyOK:
			ok++
		case VerifyFailed:
			failed++
		case VerifyMissing:
			missing++
		}
	}
	return
}

// HasFailures returns true if any result is not VerifyOK.
func HasFailures(results []VerifyResult) bool {
	for _, r := range results {
		if r.Status != VerifyOK {
			return true
		}
	}
	return false
}
