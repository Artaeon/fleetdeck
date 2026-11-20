package monitor

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

// ValidateReportPDF bounds private queue and transport sizes. The report
// producer is trusted; the attachment never comes from an arbitrary path.
func ValidateReportPDF(pdf []byte) error {
	if len(pdf) > 512<<10 || !bytes.HasPrefix(pdf, []byte("%PDF-1.")) || !bytes.HasSuffix(bytes.TrimSpace(pdf), []byte("%%EOF")) {
		return fmt.Errorf("invalid or oversized report PDF")
	}
	return nil
}

func (p *EmailProvider) message(a Alert, from *mail.Address, pdf []byte) ([]byte, error) {
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\n", from.String(), strings.Join(p.To, ", "), mime.QEncoding.Encode("utf-8", fmt.Sprintf("[FleetDeck/%s] %s", strings.ToUpper(a.Level), a.Title)), a.Timestamp.Format(time.RFC1123Z))
	if pdf == nil {
		fmt.Fprintf(&msg, "Content-Type: text/plain; charset=UTF-8\r\n\r\nTarget: %s\r\nLevel: %s\r\n\r\n%s\r\n", a.Target, a.Level, a.Message)
		return msg.Bytes(), nil
	}
	if err := ValidateReportPDF(pdf); err != nil {
		return nil, err
	}
	mixed := multipart.NewWriter(&msg)
	fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", mixed.Boundary())
	part, err := mixed.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
	if err != nil {
		return nil, err
	}
	text := quotedprintable.NewWriter(part)
	if _, err = text.Write([]byte(a.Message)); err != nil {
		return nil, err
	}
	if err = text.Close(); err != nil {
		return nil, err
	}
	filename := "fleetdeck-security-" + a.Timestamp.UTC().Format(time.DateOnly) + ".pdf"
	part, err = mixed.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/pdf"}, "Content-Disposition": {mime.FormatMediaType("attachment", map[string]string{"filename": filename})}, "Content-Transfer-Encoding": {"base64"}})
	if err != nil {
		return nil, err
	}
	// 57 source bytes encode to one 76-character MIME line.
	for len(pdf) > 0 {
		n := min(57, len(pdf))
		if _, err = fmt.Fprint(part, base64.StdEncoding.EncodeToString(pdf[:n]), "\r\n"); err != nil {
			return nil, err
		}
		pdf = pdf[n:]
	}
	if err = mixed.Close(); err != nil {
		return nil, err
	}
	return msg.Bytes(), nil
}
