package remote

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var validOTP = regexp.MustCompile(`^(?:[0-9]{6}|[0-9]{8})$`)

func totpChallenge(read func() (string, error)) ssh.KeyboardInteractiveChallenge {
	attempts := 0
	rounds := 0
	return func(user, instruction string, questions []string, echo []bool) ([]string, error) {
		rounds++
		if rounds > 8 {
			return nil, fmt.Errorf("too many SSH MFA protocol messages")
		}
		// PAM can send informational rounds with no prompts, including after
		// accepting the code. Acknowledge them without displaying untrusted
		// instructions or reading another secret from the terminal.
		if len(questions) == 0 && len(echo) == 0 {
			return []string{}, nil
		}
		attempts++
		// Never display a server-controlled prompt or feed codes to an arbitrary
		// password challenge. The host key is verified before authentication.
		if attempts > 2 || len(questions) != 1 || len(echo) != 1 || echo[0] || strings.TrimSpace(questions[0]) != "Verification code:" {
			return nil, fmt.Errorf("unsupported SSH MFA challenge; expected a non-echoed verification code")
		}
		code, err := read()
		if err != nil {
			return nil, err
		}
		code = strings.TrimSpace(code)
		if !validOTP.MatchString(code) {
			return nil, fmt.Errorf("enter a six-digit authenticator code or eight-digit recovery code")
		}
		return []string{code}, nil
	}
}

func readTOTP() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("SSH MFA needs an interactive terminal; unattended jobs cannot bypass it")
	}
	defer tty.Close()
	if !term.IsTerminal(int(tty.Fd())) {
		return "", fmt.Errorf("SSH MFA needs an interactive terminal")
	}
	fmt.Fprint(tty, "FleetDeck SSH verification code from your authenticator: ")
	code, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	if err != nil {
		return "", fmt.Errorf("could not read SSH verification code")
	}
	value := string(code)
	clear(code)
	return value, nil
}

func withMFA(methods []ssh.AuthMethod) []ssh.AuthMethod {
	if os.Getenv("FLEETDECK_SSH_MFA") == "totp" {
		return append(methods, ssh.KeyboardInteractive(totpChallenge(readTOTP)))
	}
	return methods
}
