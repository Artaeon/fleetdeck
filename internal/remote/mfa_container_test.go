package remote

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// Exercise the production challenge handler against OpenSSH/PAM, including
// informational exchanges that a simplified SSH test server can omit.
func TestContainerSSHMFAClient(t *testing.T) {
	if os.Getenv("FLEETDECK_SSH_MFA_CONTAINER_TEST") != "1" {
		t.Skip("opt-in disposable Linux SSH/PAM test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	name := fmt.Sprintf("fleetdeck-mfa-client-%d", time.Now().UnixNano())
	docker := func(input string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("SSH fixture operation failed: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	docker("", "run", "-d", "--rm", "--name", name, "--memory=384m", "--cpus=1", "-p", "127.0.0.1::2222", "debian:bookworm-slim", "sleep", "300")
	defer exec.Command("docker", "rm", "-f", name).Run()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	seed := make([]byte, 20)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	script := `set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq openssh-server libpam-google-authenticator >/dev/null
mkdir -p /run/sshd
useradd -m -s /bin/sh testoperator
passwd -d testoperator >/dev/null
install -d -m 0700 -o testoperator -g testoperator /home/testoperator/.ssh
cat > /home/testoperator/.ssh/authorized_keys <<'KEY'
` + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + `
KEY
chown testoperator:testoperator /home/testoperator/.ssh/authorized_keys
chmod 0600 /home/testoperator/.ssh/authorized_keys
umask 077
cat > /run/totp-secret <<'SEED'
` + base32.StdEncoding.EncodeToString(seed) + `
" TOTP_AUTH
" DISALLOW_REUSE
" RATE_LIMIT 3 30
" WINDOW_SIZE 3
SEED
sed -i 's|^@include common-auth$|auth required pam_google_authenticator.so secret=/run/totp-secret user=root|' /etc/pam.d/sshd
cat > /etc/ssh/sshd_config <<'CONFIG'
Port 2222
ListenAddress 0.0.0.0
HostKey /etc/ssh/ssh_host_ed25519_key
UsePAM yes
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication yes
PubkeyAuthentication yes
AuthenticationMethods publickey,keyboard-interactive:pam
LoginGraceTime 120
CONFIG
/usr/sbin/sshd
`
	docker(script, "exec", "-i", name, "sh")
	hostKeyText := docker("", "exec", name, "cat", "/etc/ssh/ssh_host_ed25519_key.pub")
	hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKeyText))
	if err != nil {
		t.Fatal(err)
	}
	address := docker("", "port", name, "2222/tcp")
	dial := func(methods ...ssh.AuthMethod) error {
		client, err := ssh.Dial("tcp", address, &ssh.ClientConfig{User: "testoperator", Auth: methods, HostKeyCallback: ssh.FixedHostKey(hostKey), Timeout: 10 * time.Second})
		if client != nil {
			client.Close()
		}
		return err
	}
	if err := dial(ssh.PublicKeys(signer)); err == nil {
		t.Fatal("key-only client bypassed MFA")
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, seed)
	mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	code := fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1000000)
	reads := 0
	challenge := totpChallenge(func() (string, error) { reads++; return code, nil })
	if err := dial(ssh.PublicKeys(signer), ssh.KeyboardInteractive(challenge)); err != nil {
		t.Fatalf("real PAM key-plus-code authentication failed: %v", err)
	}
	if reads != 1 {
		t.Fatalf("PAM login read %d codes; want exactly one", reads)
	}
	if err := dial(ssh.PublicKeys(signer), ssh.KeyboardInteractive(totpChallenge(func() (string, error) { return code, nil }))); err == nil {
		t.Fatal("real PAM accepted a reused code")
	}
}
