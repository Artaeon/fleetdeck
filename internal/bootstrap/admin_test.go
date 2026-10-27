package bootstrap

import (
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"testing"
)

func TestPrepareAdminRejectsAmbiguousCredentialsBeforeMutation(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	public := ssh.MarshalAuthorizedKey(signer.PublicKey())
	for _, tc := range []struct {
		name string
		key  []byte
	}{
		{"root", public}, {"operator;id", public}, {"-option", public},
		{"operator", []byte("bad key")},
		{"operator", append(append([]byte{}, public...), public...)},
		{"operator", append([]byte("command=\"id\" "), public...)},
	} {
		r := newMockRunner()
		if err := PrepareAdmin(r, tc.name, tc.key); err == nil {
			t.Fatal("unsafe identity accepted")
		}
		if len(r.commands) != 0 {
			t.Fatal("mutated before validation")
		}
	}
}

func TestPrepareAdminFailureIsFatal(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	r := newMockRunner()
	r.failOn["visudo"] = errTestAdmin{}
	if err := PrepareAdmin(r, "operator", ssh.MarshalAuthorizedKey(signer.PublicKey())); err == nil {
		t.Fatal("failed provisioning accepted")
	}
}

type errTestAdmin struct{}

func (errTestAdmin) Error() string { return "invalid sudo policy" }
