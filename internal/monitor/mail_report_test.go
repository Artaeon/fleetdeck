package monitor

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestWeeklyEmailIsReadableMIMEWithVerbatimAttachment(t *testing.T) {
	p := EmailProvider{To: []string{"ops@example.org"}}
	from, _ := mail.ParseAddress("sender@example.org")
	a := Alert{Title: "Wöchentlicher Sicherheitsbericht", Message: "Zwei Änderungen.\nBitte Wiederherstellung prüfen.", Timestamp: time.Now().UTC()}
	pdf := []byte("%PDF-1.4\n" + strings.Repeat("page evidence", 160) + "\n%%EOF\n")
	raw, err := p.message(a, from, pdf)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/mixed" {
		t.Fatal(mt, err)
	}
	parts := multipart.NewReader(msg.Body, params["boundary"])
	first, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(first)
	if err != nil || strings.ReplaceAll(string(body), "\r\n", "\n") != a.Message {
		t.Fatal("message body", err)
	}
	file, err := parts.NextPart()
	if err != nil || file.Header.Get("Content-Type") != "application/pdf" || !strings.HasSuffix(file.FileName(), ".pdf") {
		t.Fatal(err)
	}
	encoded, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(encoded), "\r\n") {
		if len(line) > 76 {
			t.Fatal("base64 MIME line too long")
		}
	}
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(encoded)))
	if err != nil || !bytes.Equal(pdf, decoded) {
		t.Fatal("attachment changed", err)
	}
	if _, err = parts.NextPart(); err != io.EOF {
		t.Fatal("unexpected attachment")
	}
	if err := p.SendReportPDF(a, []byte("<html>")); err == nil {
		t.Fatal("accepted non-PDF")
	}
}
