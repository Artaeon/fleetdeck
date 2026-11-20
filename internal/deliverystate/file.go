// Package deliverystate persists private notification checkpoints.
package deliverystate

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func PrivateDir(path string) error {
	i, err := os.Lstat(filepath.Dir(path))
	if err != nil || !i.IsDir() || i.Mode().Perm()&0077 != 0 {
		return errors.New("delivery state requires a private directory")
	}
	return nil
}

func Read(path string, dst any, limit int64) error {
	b, err := ReadBytes(path, limit)
	if err != nil {
		return err
	}
	if json.Unmarshal(b, dst) != nil {
		return errors.New("invalid delivery state")
	}
	return nil
}

// ReadBytes applies the same privacy and size checks to detached evidence.
func ReadBytes(path string, limit int64) ([]byte, error) {
	if err := PrivateDir(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil || !i.Mode().IsRegular() || i.Mode().Perm()&0077 != 0 || i.Size() > limit {
		return nil, errors.New("invalid delivery state permissions or size")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("cannot read private file")
	}
	return b, nil
}

// Write replaces the checkpoint before the next irreversible send.
func Write(path string, state any, limit int64) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if int64(len(b)) > limit {
		return errors.New("delivery state exceeds size limit")
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".delivery-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
