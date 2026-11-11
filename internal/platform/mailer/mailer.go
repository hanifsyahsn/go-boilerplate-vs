// Package mailer sends the app's emails (password reset, email verification):
// a Mailer interface with an SMTP implementation, a Log one for development
// and a Fake for tests.
package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

// Message is one email to one recipient.
type Message struct {
	To      string
	Subject string
	Text    string // plain-text body (always)
	HTML    string // optional
}

// Mailer sends an email. Callers send off the request path (a background
// goroutine), so Send may take seconds.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// errLineBreak refuses a header value that could inject another header.
var errLineBreak = errors.New("mailer: header contains a line break")

// buildMessage renders m as an RFC 5322 message from `from`: From, To,
// Subject (RFC 2047 encoded), Date, Message-ID, MIME-Version and a
// quoted-printable text/plain body, or multipart/alternative when m.HTML is
// set. A header value with CR or LF is refused.
func buildMessage(from string, m Message, now time.Time) ([]byte, error) {
	for _, v := range []string{from, m.To, m.Subject} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errLineBreak
		}
	}
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("mailer: from %q: %w", from, err)
	}
	if _, err = mail.ParseAddress(m.To); err != nil {
		return nil, fmt.Errorf("mailer: recipient: %w", err)
	}

	var buf bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	header("From", sender.String())
	header("To", m.To)
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", messageID(sender.Address))
	header("MIME-Version", "1.0")

	if m.HTML == "" {
		header("Content-Type", "text/plain; charset=UTF-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		buf.WriteString("\r\n")
		writeQP(&buf, m.Text)
		return buf.Bytes(), nil
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	header("Content-Type", "multipart/alternative; boundary="+mw.Boundary())
	buf.WriteString("\r\n")
	for _, part := range []struct{ contentType, content string }{
		{"text/plain; charset=UTF-8", m.Text},
		{"text/html; charset=UTF-8", m.HTML},
	} {
		// Writes into a bytes.Buffer never fail.
		w, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.contentType},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		writeQP(w, part.content)
	}
	_ = mw.Close()
	buf.Write(body.Bytes())
	return buf.Bytes(), nil
}

// writeQP writes s quoted-printable (lines stay under the 76-character limit
// whatever s holds) into an in-memory writer, which never fails.
func writeQP(w io.Writer, s string) {
	qp := quotedprintable.NewWriter(w)
	_, _ = qp.Write([]byte(s))
	_ = qp.Close()
}

// messageID is a unique Message-ID on the sender's domain.
func messageID(address string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	domain := "localhost"
	if at := strings.LastIndex(address, "@"); at >= 0 {
		domain = address[at+1:]
	}
	return "<" + hex.EncodeToString(b) + "@" + domain + ">"
}

// Log is the development mailer (SMTP_HOST empty): it logs the recipient and
// subject at info and the text body at debug, and sends nothing.
type Log struct{}

func NewLog() *Log { return &Log{} }

func (*Log) Send(ctx context.Context, m Message) error {
	slog.InfoContext(ctx, "email not sent (development mailer)", "to", m.To, "subject", m.Subject)
	slog.DebugContext(ctx, "email body (development mailer)", "text", m.Text)
	return nil
}

// Fake records the messages it is given, for tests. Err, when set, is
// returned by every Send (the message is still recorded).
type Fake struct {
	mu   sync.Mutex
	sent []Message
	Err  error
}

func (f *Fake) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return f.Err
}

// Sent returns a copy of every message sent so far.
func (f *Fake) Sent() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Message(nil), f.sent...)
}
