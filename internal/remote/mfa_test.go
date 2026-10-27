package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestMFARejectsUnexpectedPrompts(t *testing.T) {
	reads := 0
	challenge := totpChallenge(func() (string, error) { reads++; return "123456", nil })
	if _, err := challenge("", "", []string{"Password:"}, []bool{false}); err == nil || reads != 0 {
		t.Fatal("password prompt accepted")
	}
	if _, err := challenge("", "", []string{"Verification code:"}, []bool{true}); err == nil || reads != 0 {
		t.Fatal("echoed prompt accepted")
	}
}

func TestSSHRequiresKeyAndSecondFactor(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	for _, test := range []struct {
		name    string
		key     bool
		code    string
		success bool
	}{{"correct", true, "123456", true}, {"wrong-code", true, "000000", false}, {"key-only", true, "", false}, {"code-only", false, "123456", false}} {
		t.Run(test.name, func(t *testing.T) {
			server := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
				if string(k.Marshal()) != string(signer.PublicKey().Marshal()) {
					return nil, fmt.Errorf("key rejected")
				}
				return nil, &ssh.PartialSuccessError{Next: ssh.ServerAuthCallbacks{KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
					answers, err := challenge("", "", []string{"Verification code:"}, []bool{false})
					if err != nil || len(answers) != 1 || answers[0] != "123456" {
						return nil, fmt.Errorf("factor rejected")
					}
					return nil, nil
				}}}
			}}
			server.AddHostKey(signer)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				c, err := listener.Accept()
				if err != nil {
					return
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(5 * time.Second))
				s, _, _, err := ssh.NewServerConn(c, server)
				if err == nil {
					s.Close()
				}
			}()
			var methods []ssh.AuthMethod
			if test.key {
				methods = append(methods, ssh.PublicKeys(signer))
			}
			if test.code != "" {
				methods = append(methods, ssh.KeyboardInteractive(totpChallenge(func() (string, error) { return test.code, nil })))
			}
			client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "operator", Auth: methods, HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: 5 * time.Second})
			if client != nil {
				client.Close()
			}
			<-done
			if (err == nil) != test.success {
				t.Fatalf("authentication success=%v, want %v", err == nil, test.success)
			}
		})
	}
}
