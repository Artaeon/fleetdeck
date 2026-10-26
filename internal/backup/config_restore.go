package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Copy through a new inode and rename it inside the opened project root.
// Truncating an existing file would also overwrite any external hard links.
func restoreConfigFile(source, destination *os.Root, comp ComponentInfo) error {
	if err := checkPath(source, comp.Path, false, false); err != nil {
		return err
	}
	if err := checkPath(destination, comp.Name, false, true); err != nil {
		return err
	}
	in, err := source.Open(comp.Path)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("configuration source is not a regular file")
	}
	mode := info.Mode().Perm()
	if filepath.Base(comp.Name) == ".env" {
		mode = 0600
	}
	parent := filepath.Dir(comp.Name)
	if err := destination.MkdirAll(parent, 0755); err != nil {
		return err
	}
	temporary := filepath.Join(parent, ".fleetdeck-config-"+uuid.NewString())
	out, err := destination.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	defer destination.Remove(temporary)
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, hash), in); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != comp.Checksum {
		return fmt.Errorf("configuration checksum changed during restore")
	}
	if err := out.Chmod(mode); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return destination.Rename(temporary, comp.Name)
}
