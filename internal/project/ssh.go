package project

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

func GenerateSSHKeypair(projectPath string) (privateKeyPath, publicKey string, err error) {
	sshDir := filepath.Join(projectPath, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", "", fmt.Errorf("creating .ssh dir: %w", err)
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generating keypair: %w", err)
	}

	privPEM, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", "", fmt.Errorf("marshaling private key: %w", err)
	}

	privKeyPath := filepath.Join(sshDir, "deploy_key")
	privKeyData := pem.EncodeToMemory(privPEM)
	if err := os.WriteFile(privKeyPath, privKeyData, 0600); err != nil {
		return "", "", fmt.Errorf("writing private key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", "", fmt.Errorf("creating SSH public key: %w", err)
	}

	pubKeyStr := string(ssh.MarshalAuthorizedKey(sshPub))

	pubKeyPath := filepath.Join(sshDir, "deploy_key.pub")
	if err := os.WriteFile(pubKeyPath, []byte(pubKeyStr), 0644); err != nil {
		return "", "", fmt.Errorf("writing public key: %w", err)
	}

	return privKeyPath, pubKeyStr, nil
}
