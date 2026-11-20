package publication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func pair(t *testing.T) (ed25519.PrivateKey, ssh.Signer) {
	t.Helper()
	_, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s, e := ssh.NewSignerFromKey(k)
	if e != nil {
		t.Fatal(e)
	}
	return k, s
}

func TestProjectPublicationWithBoundSSHIdentity(t *testing.T) {
	dir := t.TempDir()
	// Unix-domain socket paths are short even when test artifacts live on an external volume.
	socketDir, err := os.MkdirTemp("/tmp", "fd-pub-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "agent")
	a, e := net.Listen("unix", socket)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	realKey, project := pair(t)
	other, _ := pair(t)
	_, server := pair(t)
	ring := agent.NewKeyring()
	for _, key := range []ed25519.PrivateKey{other, realKey} {
		if e := ring.Add(agent.AddedKey{PrivateKey: key}); e != nil {
			t.Fatal(e)
		}
	}
	go func() {
		for {
			c, err := a.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _ = agent.ServeAgent(ring, c) }()
		}
	}()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if c.User() != "site-deploy" || !bytes.Equal(key.Marshal(), project.PublicKey().Marshal()) {
			return nil, fmt.Errorf("wrong project key")
		}
		return nil, nil
	}}
	cfg.AddHostKey(server)
	content := []byte("a disposable test release")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	file := filepath.Join(dir, "site.gz")
	if err := os.WriteFile(file, content, 0600); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			errs <- err
			return
		}
		defer c.Close()
		conn, chans, requests, err := ssh.NewServerConn(c, cfg)
		if err != nil {
			errs <- err
			return
		}
		defer conn.Close()
		go ssh.DiscardRequests(requests)
		for _, want := range []string{"status", "apply"} {
			in := <-chans
			if in == nil || in.ChannelType() != "session" {
				errs <- fmt.Errorf("session missing")
				return
			}
			ch, requests, err := in.Accept()
			if err != nil {
				errs <- err
				return
			}
			req := <-requests
			var command struct{ Command string }
			if req == nil || req.Type != "exec" || ssh.Unmarshal(req.Payload, &command) != nil || command.Command != want {
				errs <- fmt.Errorf("unexpected action")
				return
			}
			req.Reply(true, nil)
			body, err := io.ReadAll(ch)
			if err != nil {
				errs <- err
				return
			}
			result := Receipt{Project: "site", Environment: "production", Status: "current", Release: "release-1"}
			if want == "apply" {
				line, data, ok := bytes.Cut(body, []byte{'\n'})
				var h header
				if !ok || json.Unmarshal(line, &h) != nil || h.SHA256 != digest || h.Previous != "release-1" || h.Project != "site" || h.Environment != "production" || h.Size != int64(len(content)) || h.Version != 1 || !bytes.Equal(content, data) {
					errs <- fmt.Errorf("artifact contract mismatch")
					return
				}
				result.SHA256 = digest
				result.Status = "active"
				result.Release = "release-2"
			}
			if err := json.NewEncoder(ch).Encode(result); err != nil {
				errs <- err
				return
			}
			ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
			ch.Close()
		}
		errs <- nil
	}()
	c := Catalog{Version: 1, Hosts: map[string]Host{"node": {l.Addr().String(), string(ssh.MarshalAuthorizedKey(server.PublicKey()))}}, Projects: map[string]Project{"site": {Host: "node", Account: "site-deploy", Environment: "production", ArtifactDir: dir, AgentSocket: socket, PublicKey: string(ssh.MarshalAuthorizedKey(project.PublicKey()))}}}
	result, err := c.Run(context.Background(), "site", "apply", file, "release-1")
	if err != nil || result.Status != "active" {
		t.Fatalf("%v %v", result, err)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
}

func TestEnrollmentPreconditionsAndBoundaries(t *testing.T) {
	dir := t.TempDir()
	_, key := pair(t)
	pub := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key.PublicKey())))
	config := fmt.Sprintf("version = 1\n[hosts.node]\naddress = '127.0.0.1:22'\nhost_key = '%s'\n[projects.site]\nhost = 'node'\naccount = 'site-deploy'\nenvironment = 'production'\npublic_key = '%s'\nagent_socket = '/private/socket'\nartifact_dir = '%s'\n", pub, pub, dir)
	p := filepath.Join(dir, "publication.toml")
	if err := os.WriteFile(p, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil || len(c.List()) != 1 {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(config, "site-deploy", "root", 1), strings.Replace(config, "[projects.site]", "[projects.site]\nunknown = true", 1)} {
		os.WriteFile(p, []byte(bad), 0600)
		if _, err := Load(p); err == nil {
			t.Fatal("invalid enrollment accepted")
		}
	}
	os.WriteFile(p, []byte(config), 0600)
	os.Chmod(p, 0644)
	if _, err := Load(p); err == nil {
		t.Fatal("public catalog accepted")
	}
	outside := filepath.Join(t.TempDir(), "private.gz")
	os.WriteFile(outside, []byte("outside"), 0600)
	link := filepath.Join(dir, "link.gz")
	os.Symlink(outside, link)
	for _, name := range []string{outside, link} {
		if f, _, _, err := openArtifact(dir, name); err == nil {
			f.Close()
			t.Fatal("read outside enrolled artifact storage")
		}
	}
	for _, test := range []struct{ target, prev string }{{"another-project", "release-1"}, {"site", "release-0"}} {
		calls := 0
		_, err := transaction("site", c.Projects["site"], "apply", outside, test.prev, func(action string, _ io.Reader) (Receipt, error) {
			calls++
			return Receipt{Project: test.target, Environment: "production", Release: "release-1", Status: "current"}, nil
		})
		if err == nil || calls != 1 {
			t.Fatal("accepted mismatched target or predecessor")
		}
	}
	var out limitedReply
	if _, err := io.Copy(&out, strings.NewReader(strings.Repeat("x", 4097))); err == nil {
		t.Fatal("unbounded receiver reply")
	}
}
