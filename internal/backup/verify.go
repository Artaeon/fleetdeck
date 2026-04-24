package backup

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	var results []VerifyResult
	for _, comp := range manifest.Components {
		result := verifyComponent(backupPath, comp)
		results = append(results, result)
	}

	return results, nil
}

func verifyComponent(backupPath string, comp ComponentInfo) VerifyResult {
	fullPath := filepath.Join(backupPath, comp.Path)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return VerifyResult{
			Component: comp,
			Status:    VerifyMissing,
			Error:     fmt.Errorf("file not found: %s", comp.Path),
		}
	}

	switch comp.Type {
	case "config":
		return verifyConfigFile(fullPath, comp)
	case "database":
		return verifyGzipFile(fullPath, comp)
	case "volume":
		return verifyGzipFile(fullPath, comp)
	default:
		// Unknown type — just check existence (already passed above)
		return VerifyResult{Component: comp, Status: VerifyOK}
	}
}

// verifyConfigFile recomputes the SHA256 checksum and compares it to the
// manifest value. A missing checksum in the manifest is treated as a
// verification failure, not a silent pass — otherwise an attacker who
// tampered with both a config file AND the manifest (removing the
// Checksum field) would sail through integrity checks.
func verifyConfigFile(path string, comp ComponentInfo) VerifyResult {
	if comp.Checksum == "" {
		return VerifyResult{
			Component: comp,
			Status:    VerifyFailed,
			Error:     fmt.Errorf("manifest has no checksum for %s — cannot verify integrity", comp.Name),
		}
	}

	checksum, err := fileSHA256(path)
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
func verifyGzipFile(path string, comp ComponentInfo) VerifyResult {
	f, err := os.Open(path)
	if err != nil {
		return VerifyResult{
			Component: comp,
			Status:    VerifyFailed,
			Error:     fmt.Errorf("opening file: %w", err),
		}
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
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
