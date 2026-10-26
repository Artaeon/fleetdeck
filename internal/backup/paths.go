package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Backup paths are portable relative names, never command options or host paths.
func validateRelativePath(name string) error {
	if !filepath.IsLocal(name) || name == "." || strings.ContainsAny(name, "\\:\x00") {
		return fmt.Errorf("invalid relative path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "." || part == "" {
			return fmt.Errorf("invalid relative path %q", name)
		}
	}
	return nil
}

var dockerObjectName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var databaseComponentName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]* \((PostgreSQL|MySQL)\)$`)

func validateComponent(comp ComponentInfo) error {
	if err := validateRelativePath(comp.Path); err != nil {
		return err
	}
	switch comp.Type {
	case "config":
		return validateRelativePath(comp.Name)
	case "database":
		if !databaseComponentName.MatchString(comp.Name) {
			return fmt.Errorf("unsupported database component %q", comp.Name)
		}
	case "volume":
		if strings.HasPrefix(filepath.Base(comp.Path), "namedvol_") {
			name := strings.TrimSuffix(comp.Name, " (named volume)")
			if !strings.HasSuffix(comp.Name, " (named volume)") || !dockerObjectName.MatchString(name) || comp.RestorePath != "" {
				return fmt.Errorf("invalid named volume component %q", comp.Name)
			}
		} else if comp.RestorePath != "" {
			return validateRelativePath(comp.RestorePath)
		}
	default:
		return fmt.Errorf("unsupported component type %q", comp.Type)
	}
	return nil
}

// Check every existing path component. Actual file access also uses os.Root so
// an exchanged symlink cannot redirect it outside the selected directory.
func checkPath(root *os.Root, name string, directory, allowMissing bool) error {
	if err := validateRelativePath(name); err != nil {
		return err
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if os.IsNotExist(err) && allowMissing {
			return nil
		}
		if err != nil {
			return err
		}
		wantDir := i < len(parts)-1 || directory
		if info.Mode()&os.ModeSymlink != 0 || (wantDir && !info.IsDir()) || (!wantDir && !info.Mode().IsRegular()) {
			return fmt.Errorf("path %q contains a link or unexpected file type", name)
		}
	}
	return nil
}
