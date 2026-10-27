// Package sourcedeploy receives reviewed source from a trusted operator. It is
// not a sandbox for untrusted Compose files, Dockerfiles or build steps.
package sourcedeploy

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const MaxArchiveBytes int64 = 1 << 30
const MaxFiles = 100000

var composeNames = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}
var skipped = map[string]bool{".git": true, ".svn": true, ".ssh": true, ".aws": true, ".kube": true, ".fleetdeck": true, "node_modules": true, ".next": true, ".nuxt": true, ".venv": true, "venv": true, "__pycache__": true, "target": true, ".cache": true, "coverage": true, ".turbo": true, ".DS_Store": true}

func safeName(name string) bool {
	return filepath.IsLocal(name) && name != "." && !strings.ContainsAny(name, "\\\x00\r\n") && filepath.ToSlash(filepath.Clean(name)) == name
}
func sensitive(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		lower := strings.ToLower(part)
		if skipped[part] || strings.HasPrefix(lower, ".fleetdeck") || lower == ".env" || strings.HasPrefix(lower, ".env.") {
			return true
		}
		for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".jks"} {
			if strings.HasSuffix(lower, ext) {
				return true
			}
		}
	}
	return false
}
func protected(name string, paths []string) bool {
	if sensitive(name) {
		return true
	}
	for _, p := range paths {
		if name == p || strings.HasPrefix(name, p+"/") {
			return true
		}
	}
	return false
}

// Protected returns project-relative env_file, secrets/config files and writable
// bind paths. Dynamic/absolute references fail closed instead of guessing.
func Protected(dir string) (string, []string, error) {
	file := ""
	for _, n := range composeNames {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			if file != "" {
				return "", nil, fmt.Errorf("source deployment requires exactly one Compose file")
			}
			file = n
		} else if !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	if file == "" {
		return "", nil, fmt.Errorf("source deployment requires a Compose file")
	}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return "", nil, err
	}
	if len(data) > 4<<20 {
		return "", nil, fmt.Errorf("Compose file too large")
	}
	var c struct {
		Include  yaml.Node `yaml:"include"`
		Services map[string]struct {
			Env     yaml.Node `yaml:"env_file"`
			Volumes yaml.Node `yaml:"volumes"`
			Extends yaml.Node `yaml:"extends"`
		} `yaml:"services"`
		Secrets map[string]struct {
			File string `yaml:"file"`
		} `yaml:"secrets"`
		Configs map[string]struct {
			File string `yaml:"file"`
		} `yaml:"configs"`
	}
	if err = yaml.Unmarshal(data, &c); err != nil {
		return "", nil, fmt.Errorf("invalid Compose YAML")
	}
	if len(c.Services) == 0 || c.Include.Kind != 0 {
		return "", nil, fmt.Errorf("Compose must define services without include/extends")
	}
	paths := map[string]bool{}
	add := func(value string) error {
		clean := filepath.ToSlash(filepath.Clean(value))
		if strings.Contains(value, "$") || !safeName(clean) {
			return fmt.Errorf("protected paths must be literal, project-relative files/directories")
		}
		paths[clean] = true
		return nil
	}
	for _, s := range c.Services {
		if s.Extends.Kind != 0 {
			return "", nil, fmt.Errorf("Compose extends is not supported by source deployment")
		}
		if err = envPaths(s.Env, add); err != nil {
			return "", nil, err
		}
		if s.Volumes.Kind == 0 {
			continue
		}
		if s.Volumes.Kind != yaml.SequenceNode {
			return "", nil, fmt.Errorf("invalid Compose volumes")
		}
		for _, node := range s.Volumes.Content {
			switch node.Kind {
			case yaml.ScalarNode:
				parts := strings.Split(node.Value, ":")
				if len(parts) == 1 {
					return "", nil, fmt.Errorf("anonymous volumes require an explicit named volume")
				}
				if len(parts) > 3 {
					return "", nil, fmt.Errorf("unsupported volume declaration")
				}
				source := parts[0]
				if strings.Contains(source, "$") {
					return "", nil, fmt.Errorf("volume source interpolation is unsupported")
				}
				if strings.HasPrefix(source, "/") {
					return "", nil, fmt.Errorf("source deployments require project-relative bind mounts")
				}
				if strings.HasPrefix(source, ".") || strings.Contains(source, "/") {
					ro := len(parts) == 3 && contains(strings.Split(parts[2], ","), "ro")
					if !ro {
						if err = add(source); err != nil {
							return "", nil, err
						}
					}
				}
			case yaml.MappingNode:
				var v struct {
					Type     string `yaml:"type"`
					Source   string `yaml:"source"`
					ReadOnly bool   `yaml:"read_only"`
				}
				if node.Decode(&v) != nil {
					return "", nil, fmt.Errorf("invalid volume")
				}
				if v.Type == "bind" {
					if filepath.IsAbs(v.Source) || strings.Contains(v.Source, "$") {
						return "", nil, fmt.Errorf("bind mounts must be literal project-relative paths")
					}
					if !v.ReadOnly {
						if err = add(v.Source); err != nil {
							return "", nil, err
						}
					}
				} else if v.Type == "volume" && v.Source == "" {
					return "", nil, fmt.Errorf("anonymous volume unsupported")
				}
			default:
				return "", nil, fmt.Errorf("unsupported volume declaration")
			}
		}
	}
	for _, s := range c.Secrets {
		if s.File != "" {
			if err = add(s.File); err != nil {
				return "", nil, err
			}
		}
	}
	for _, s := range c.Configs {
		if s.File != "" {
			if err = add(s.File); err != nil {
				return "", nil, err
			}
		}
	}
	out := make([]string, 0, len(paths))
	for p := range paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return file, out, nil
}
func contains(items []string, item string) bool {
	for _, v := range items {
		if v == item {
			return true
		}
	}
	return false
}
func envPaths(n yaml.Node, add func(string) error) error {
	switch n.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		if n.Tag != "!!str" {
			return fmt.Errorf("invalid env_file")
		}
		return add(n.Value)
	case yaml.SequenceNode:
		for _, child := range n.Content {
			if err := envPaths(*child, add); err != nil {
				return err
			}
		}
		return nil
	case yaml.MappingNode:
		var ref struct {
			Path string `yaml:"path"`
		}
		if n.Decode(&ref) != nil || ref.Path == "" {
			return fmt.Errorf("invalid env_file path")
		}
		return add(ref.Path)
	default:
		return fmt.Errorf("invalid env_file")
	}
}

// Archive excludes known secrets, Compose private references and writable data.
// It never follows symlinks or includes credentials implicitly.
func Archive(dir string, out io.Writer) error { return ArchiveWithEnv(dir, out, "") }

// ArchiveWithEnv explicitly seeds .env on first deployment; the receiver always
// preserves an existing server file, even when another seed is supplied.
func ArchiveWithEnv(dir string, out io.Writer, envFile string) error {
	_, paths, err := Protected(dir)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(out)
	var total int64
	count := 0
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if protected(rel, paths) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !safeName(rel) {
			return fmt.Errorf("invalid source path")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source links and special files are unsupported: %s", rel)
		}
		count++
		total += info.Size()
		if count > MaxFiles || total > MaxArchiveBytes {
			return fmt.Errorf("source archive exceeds limits")
		}
		in, err := root.Open(rel)
		if err != nil {
			return err
		}
		current, err := in.Stat()
		if err != nil || !os.SameFile(info, current) {
			return fmt.Errorf("source changed during upload")
		}
		if err = tw.WriteHeader(&tar.Header{Name: rel, Size: info.Size(), Mode: int64(info.Mode().Perm() & 0777), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = io.CopyN(tw, in, info.Size())
		in.Close()
		return err
	})
	if err != nil {
		return err
	}
	if envFile != "" {
		info, err := os.Lstat(envFile)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return fmt.Errorf("env-file must be a regular file under 1 MiB")
		}
		data, err := os.ReadFile(envFile)
		if err != nil {
			return err
		}
		if err = tw.WriteHeader(&tar.Header{Name: ".env", Size: int64(len(data)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err = tw.Write(data); err != nil {
			return err
		}
	}
	return tw.Close()
}

func Extract(input io.Reader, dir string) ([]string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	limited := &io.LimitedReader{R: input, N: MaxArchiveBytes + (MaxFiles * 1024) + 1}
	tr := tar.NewReader(limited)
	var files []string
	seen := map[string]bool{}
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid source archive: %w", err)
		}
		if !safeName(h.Name) || (sensitive(h.Name) && h.Name != ".env") || seen[h.Name] || h.Typeflag != tar.TypeReg || h.Size < 0 {
			return nil, fmt.Errorf("unsafe or duplicate source archive entry")
		}
		seen[h.Name] = true
		total += h.Size
		if len(files) >= MaxFiles || total > MaxArchiveBytes || limited.N <= 0 {
			return nil, fmt.Errorf("source archive exceeds limits")
		}
		if err = root.MkdirAll(filepath.Dir(h.Name), 0700); err != nil {
			return nil, err
		}
		f, err := root.OpenFile(h.Name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode)&0777)
		if err != nil {
			return nil, err
		}
		_, copyErr := io.CopyN(f, tr, h.Size)
		closeErr := f.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		files = append(files, h.Name)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("empty source archive")
	}
	return files, nil
}
