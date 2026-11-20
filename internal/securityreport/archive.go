package securityreport

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fleetdeck/fleetdeck/internal/deliverystate"
)

// Publish a fully written file under a content-addressed name. Link is
// create-only: an existing archive can never be replaced by a retry.
func archivePDF(dir string, p pendingReport) error {
	if !filepath.IsAbs(dir) || !validSHA256(p.PDFHash) {
		return fmt.Errorf("invalid private report archive")
	}
	path := filepath.Join(dir, p.Period+"-"+p.PDFHash+".pdf")
	if err := deliverystate.PrivateDir(path); err != nil {
		return err
	}
	existing, err := deliverystate.ReadBytes(path, 512<<10)
	if err == nil {
		if !bytes.Equal(existing, p.PDF) {
			return fmt.Errorf("archive content differs from queued PDF")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(dir, ".pending-report-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(p.PDF); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Link(f.Name(), path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		prev, e := deliverystate.ReadBytes(path, 512<<10)
		if e != nil || !bytes.Equal(prev, p.PDF) {
			return fmt.Errorf("conflicting archived report")
		}
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
