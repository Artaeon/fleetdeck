// Package publication sends static release artifacts to a pre-enrolled project receiver.
package publication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/sys/unix"
)

const MaxArtifact = 64 << 20

var id = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var releaseID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,126}$`)

type Host struct {
	Address string `toml:"address"`
	HostKey string `toml:"host_key"`
}
type Project struct {
	Host        string `toml:"host"`
	Account     string `toml:"account"`
	Environment string `toml:"environment"`
	AgentSocket string `toml:"agent_socket"`
	PublicKey   string `toml:"public_key"`
	ArtifactDir string `toml:"artifact_dir"`
}
type Catalog struct {
	Version  int                `toml:"version"`
	Hosts    map[string]Host    `toml:"hosts"`
	Projects map[string]Project `toml:"projects"`
}

func publicKey(text string) (ssh.PublicKey, error) {
	k, _, opts, rest, err := ssh.ParseAuthorizedKey([]byte(text))
	if err != nil || len(opts) > 0 || len(bytes.TrimSpace(rest)) > 0 || k.Type() != ssh.KeyAlgoED25519 {
		return nil, fmt.Errorf("expected one Ed25519 public key")
	}
	return k, nil
}

func Load(path string) (Catalog, error) {
	var c Catalog
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return c, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 128<<10 {
		return c, fmt.Errorf("publication catalog must be a private regular file of at most 128 KiB")
	}
	parent, err := os.Lstat(filepath.Dir(path))
	owner, ok := st.Sys().(*syscall.Stat_t)
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0022 != 0 || !ok || int(owner.Uid) != os.Geteuid() {
		return c, fmt.Errorf("publication catalog and its directory must be protected by the workstation owner")
	}
	data, err := io.ReadAll(io.LimitReader(f, (128<<10)+1))
	if err != nil || len(data) > 128<<10 {
		return c, fmt.Errorf("cannot read publication catalog")
	}
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&c); err != nil {
		return c, fmt.Errorf("invalid publication catalog")
	}
	if c.Version != 1 || len(c.Hosts) == 0 {
		return c, fmt.Errorf("unsupported or empty publication catalog")
	}
	for name, h := range c.Hosts {
		address, port, err := net.SplitHostPort(h.Address)
		n, pe := strconv.Atoi(port)
		if !id.MatchString(name) || err != nil || address == "" || pe != nil || n < 1 || n > 65535 {
			return c, fmt.Errorf("invalid enrolled host")
		}
		if _, err := publicKey(h.HostKey); err != nil {
			return c, err
		}
	}
	for name, p := range c.Projects {
		_, exists := c.Hosts[p.Host]
		if !id.MatchString(name) || !exists || !id.MatchString(p.Account) || p.Account == "root" || !id.MatchString(p.Environment) || !filepath.IsAbs(p.AgentSocket) || !filepath.IsAbs(p.ArtifactDir) {
			return c, fmt.Errorf("invalid project publication target")
		}
		if _, err := publicKey(p.PublicKey); err != nil {
			return c, err
		}
	}
	return c, nil
}

type Listing struct {
	Host        string `json:"host"`
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
}

func (c Catalog) List() []Listing {
	var rows []Listing
	used := map[string]bool{}
	for n, p := range c.Projects {
		rows = append(rows, Listing{p.Host, n, p.Environment})
		used[p.Host] = true
	}
	for n := range c.Hosts {
		if !used[n] {
			rows = append(rows, Listing{Host: n})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Host != rows[j].Host {
			return rows[i].Host < rows[j].Host
		}
		return rows[i].Project < rows[j].Project
	})
	return rows
}

type header struct {
	Version     int    `json:"version"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Previous    string `json:"previous"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}
type Receipt struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Release     string `json:"release"`
	Status      string `json:"status"`
	SHA256      string `json:"sha256,omitempty"`
	Code        string `json:"code,omitempty"`
}

func openArtifact(dir, path string) (*os.File, int64, string, error) {
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, 0, "", err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, 0, "", err
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil || parent != want || filepath.Ext(path) != ".gz" {
		return nil, 0, "", fmt.Errorf("select a gzip release directly inside the enrolled artifact directory")
	}
	f, err := os.OpenFile(filepath.Join(parent, filepath.Base(path)), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, 0, "", err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() < 1 || st.Size() > MaxArtifact {
		f.Close()
		return nil, 0, "", fmt.Errorf("invalid release artifact size or type")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, st.Size()+1))
	if err != nil || n != st.Size() {
		f.Close()
		return nil, 0, "", fmt.Errorf("artifact changed while hashing")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, 0, "", err
	}
	return f, st.Size(), hex.EncodeToString(h.Sum(nil)), nil
}

type call func(action string, input io.Reader) (Receipt, error)

func (c Catalog) Run(ctx context.Context, project, action, path, expected string) (Receipt, error) {
	p, ok := c.Projects[project]
	if !ok {
		return Receipt{}, fmt.Errorf("project has no enrolled publication identity")
	}
	conn, err := connect(ctx, c.Hosts[p.Host], p)
	if err != nil {
		return Receipt{}, err
	}
	defer conn.Close()
	return transaction(project, p, action, path, expected, func(action string, r io.Reader) (Receipt, error) { return request(conn, action, r) })
}
func transaction(project string, p Project, action, path, expected string, call call) (Receipt, error) {
	var zero Receipt
	if action != "status" && action != "verify" && action != "apply" {
		return zero, fmt.Errorf("unknown publication action")
	}
	match := func(r Receipt) bool {
		return r.Code == "" && r.Project == project && r.Environment == p.Environment && releaseID.MatchString(r.Release)
	}
	current, err := call("status", nil)
	if err != nil {
		return zero, err
	}
	if !match(current) || current.Status != "current" {
		return zero, fmt.Errorf("receiver identity or scope differs from the enrolled project")
	}
	if action == "status" {
		return current, nil
	}
	if expected != "" && expected != current.Release {
		return zero, fmt.Errorf("active release differs from the expected predecessor")
	}
	f, size, sha, err := openArtifact(p.ArtifactDir, path)
	if err != nil {
		return zero, err
	}
	defer f.Close()
	var line bytes.Buffer
	if err := json.NewEncoder(&line).Encode(header{1, project, p.Environment, current.Release, size, sha}); err != nil {
		return zero, err
	}
	receipt, err := call(action, io.MultiReader(&line, io.LimitReader(f, size)))
	if err != nil {
		return zero, err
	}
	want := "verified"
	if action == "apply" {
		want = "active"
	}
	if !match(receipt) || receipt.Status != want || receipt.SHA256 != sha {
		return zero, fmt.Errorf("invalid or unexpected publication receipt")
	}
	return receipt, nil
}

func connect(ctx context.Context, h Host, p Project) (*ssh.Client, error) {
	identity, err := publicKey(p.PublicKey)
	if err != nil {
		return nil, err
	}
	hostKey, err := publicKey(h.HostKey)
	if err != nil {
		return nil, err
	}
	var d = net.Dialer{Timeout: 8 * time.Second}
	a, err := d.DialContext(ctx, "unix", p.AgentSocket)
	if err != nil {
		return nil, fmt.Errorf("project signing agent is unavailable")
	}
	defer a.Close()
	a.SetDeadline(time.Now().Add(90 * time.Second))
	signers, err := agent.NewClient(a).Signers()
	if err != nil {
		return nil, fmt.Errorf("cannot query the project signing agent")
	}
	var chosen ssh.Signer
	for _, s := range signers {
		if bytes.Equal(s.PublicKey().Marshal(), identity.Marshal()) {
			chosen = s
			break
		}
	}
	if chosen == nil {
		return nil, fmt.Errorf("the signing agent does not offer this project's enrolled key")
	}
	tcp, err := d.DialContext(ctx, "tcp", h.Address)
	if err != nil {
		return nil, err
	}
	tcp.SetDeadline(time.Now().Add(95 * time.Second))
	cfg := &ssh.ClientConfig{User: p.Account, Auth: []ssh.AuthMethod{ssh.PublicKeys(chosen)}, HostKeyCallback: ssh.FixedHostKey(hostKey), HostKeyAlgorithms: []string{ssh.KeyAlgoED25519}}
	conn, ch, req, err := ssh.NewClientConn(tcp, h.Address, cfg)
	if err != nil {
		tcp.Close()
		return nil, fmt.Errorf("project SSH authentication or pinned server verification failed")
	}
	tcp.SetDeadline(time.Now().Add(8 * time.Minute))
	return ssh.NewClient(conn, ch, req), nil
}

type limitedReply struct{ buf bytes.Buffer }

func (b *limitedReply) Write(p []byte) (int, error) {
	if len(p) > 4096-b.buf.Len() {
		return 0, fmt.Errorf("receiver reply too large")
	}
	return b.buf.Write(p)
}
func request(c *ssh.Client, action string, input io.Reader) (Receipt, error) {
	var receipt Receipt
	s, err := c.NewSession()
	if err != nil {
		return receipt, err
	}
	defer s.Close()
	var out limitedReply
	s.Stdin = input
	s.Stdout = &out
	s.Stderr = io.Discard
	runErr := s.Run(action)
	dec := json.NewDecoder(bytes.NewReader(out.buf.Bytes()))
	dec.DisallowUnknownFields()
	valid := dec.Decode(&receipt) == nil && dec.Decode(new(any)) == io.EOF
	if valid && receipt.Status == "rejected" {
		switch receipt.Code {
		case "stale_predecessor", "digest_mismatch", "release_conflict", "proxy_review_required", "space_required", "archive_limits", "wrong_target", "release_validation_failed":
			return Receipt{}, fmt.Errorf("publication refused: %s", receipt.Code)
		}
	}
	if runErr != nil {
		return Receipt{}, fmt.Errorf("project receiver did not confirm %s; recheck project status before retrying", action)
	}
	if !valid {
		return Receipt{}, fmt.Errorf("invalid receiver reply")
	}
	return receipt, nil
}
