package monitor

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// EmailProvider requires verified TLS before credentials or message data are sent.
type EmailProvider struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to,omitempty"`
}

func (p *EmailProvider) Name() string { return "email" }

func LoadEmailProvider(path string, recipients []string) (*EmailProvider, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening SMTP configuration: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 65536 {
		return nil, fmt.Errorf("SMTP configuration must be a private regular file (mode 0600 or stricter, at most 64 KiB)")
	}
	var provider EmailProvider
	decoder := json.NewDecoder(io.LimitReader(f, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provider); err != nil {
		return nil, fmt.Errorf("invalid SMTP configuration JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing SMTP configuration data")
	}
	if len(recipients) > 0 {
		provider.To = recipients
	}
	if err := provider.validate(); err != nil {
		return nil, err
	}
	return &provider, nil
}
func (p *EmailProvider) validate() error {
	if p.Host == "" || strings.ContainsAny(p.Host, "\r\n /:@") || p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("invalid SMTP host or port")
	}
	if p.Username == "" || p.Password == "" {
		return fmt.Errorf("authenticated SMTP credentials are required")
	}
	if len(p.To) == 0 {
		return fmt.Errorf("at least one notification recipient is required")
	}
	for _, value := range append([]string{p.From}, p.To...) {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("invalid email address")
		}
		if _, err := mail.ParseAddress(value); err != nil {
			return fmt.Errorf("invalid email address")
		}
	}
	return nil
}
func (p *EmailProvider) Send(alert Alert) error {
	return p.send(alert, &tls.Config{ServerName: p.Host, MinVersion: tls.VersionTLS12})
}
func (p *EmailProvider) send(alert Alert, config *tls.Config) error {
	if err := p.validate(); err != nil {
		return err
	}
	if strings.ContainsAny(alert.Title+alert.Level, "\r\n") {
		return fmt.Errorf("invalid notification subject")
	}
	from, _ := mail.ParseAddress(p.From)
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nTarget: %s\r\nLevel: %s\r\n\r\n%s\r\n", from.String(), strings.Join(p.To, ", "), mime.QEncoding.Encode("utf-8", fmt.Sprintf("[FleetDeck/%s] %s", strings.ToUpper(alert.Level), alert.Title)), alert.Timestamp.Format(time.RFC1123Z), alert.Target, alert.Level, alert.Message)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), 10*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to SMTP server: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return err
	}
	if p.Port == 465 {
		secure := tls.Client(conn, config)
		if err := secure.Handshake(); err != nil {
			return fmt.Errorf("SMTP TLS verification failed")
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, p.Host)
	if err != nil {
		return fmt.Errorf("SMTP greeting failed")
	}
	defer client.Close()
	if p.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not offer required STARTTLS")
		}
		if err := client.StartTLS(config); err != nil {
			return fmt.Errorf("SMTP TLS verification failed")
		}
	}
	if err := client.Auth(smtp.PlainAuth("", p.Username, p.Password, p.Host)); err != nil {
		return fmt.Errorf("SMTP authentication failed")
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("SMTP sender rejected")
	}
	for _, recipient := range p.To {
		address, _ := mail.ParseAddress(recipient)
		if err := client.Rcpt(address.Address); err != nil {
			return fmt.Errorf("SMTP recipient rejected")
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP message rejected")
	}
	if _, err := writer.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("SMTP message transfer failed")
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP message delivery not accepted")
	}
	// DATA acceptance confirms handoff. A lost QUIT response must not cause duplicates.
	_ = client.Quit()
	return nil
}
