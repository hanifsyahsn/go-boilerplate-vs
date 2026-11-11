package mailer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

const from = "App <no-reply@example.com>"

// readBody decodes the quoted-printable body of a single-part message.
func readBody(t *testing.T, msg *mail.Message) string {
	t.Helper()
	b, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	require.NoError(t, err)
	return string(b)
}

func TestBuildMessage_U(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	long := strings.Repeat("Ä long line ", 20)

	testCases := []struct {
		name    string
		from    string
		m       Message
		wantErr string
		check   func(t *testing.T, msg *mail.Message)
	}{
		{
			name: "a text email: the headers, an encoded subject and a quoted-printable body with long lines kept intact",
			from: from,
			m:    Message{To: "jane@example.com", Subject: "Réinitialiser", Text: "Open:\n\nhttps://x/?token=abc\n" + long},
			check: func(t *testing.T, msg *mail.Message) {
				require.Equal(t, `"App" <no-reply@example.com>`, msg.Header.Get("From"))
				require.Equal(t, "jane@example.com", msg.Header.Get("To"))
				subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
				require.NoError(t, err)
				require.Equal(t, "Réinitialiser", subject)
				require.Equal(t, "Wed, 11 Nov 2026 09:00:00 +0000", msg.Header.Get("Date"))
				require.Regexp(t, `^<[0-9a-f]{32}@example\.com>$`, msg.Header.Get("Message-ID"))
				require.Equal(t, "1.0", msg.Header.Get("MIME-Version"))
				require.Equal(t, "text/plain; charset=UTF-8", msg.Header.Get("Content-Type"))
				require.Equal(t, "Open:\r\n\r\nhttps://x/?token=abc\r\n"+long, readBody(t, msg), "line ends are sent as CRLF")
			},
		},
		{
			name: "with HTML: multipart/alternative, the text part first",
			from: from,
			m:    Message{To: "jane@example.com", Subject: "Hi", Text: "plain", HTML: "<p>rich</p>"},
			check: func(t *testing.T, msg *mail.Message) {
				mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
				require.NoError(t, err)
				require.Equal(t, "multipart/alternative", mediaType)
				r := multipart.NewReader(msg.Body, params["boundary"])
				var got []string
				for {
					p, err := r.NextRawPart()
					if errors.Is(err, io.EOF) {
						break
					}
					require.NoError(t, err)
					b, err := io.ReadAll(quotedprintable.NewReader(p))
					require.NoError(t, err)
					got = append(got, p.Header.Get("Content-Type")+"|"+string(b))
				}
				require.Equal(t, []string{"text/plain; charset=UTF-8|plain", "text/html; charset=UTF-8|<p>rich</p>"}, got)
			},
		},
		{name: "a line break in the subject is refused (header injection)", from: from,
			m: Message{To: "jane@example.com", Subject: "Hi\r\nBcc: evil@example.com"}, wantErr: "mailer: header contains a line break"},
		{name: "a line break in the recipient is refused", from: from,
			m: Message{To: "jane@example.com\nBcc: evil@example.com"}, wantErr: "mailer: header contains a line break"},
		{name: "a line break in the sender is refused", from: "a@example.com\r\nX: y",
			m: Message{To: "jane@example.com"}, wantErr: "mailer: header contains a line break"},
		{name: "a recipient that isn't an address is refused", from: from, m: Message{To: "jane"}, wantErr: "mailer: recipient"},
		{name: "a sender that isn't an address is refused", from: "nobody", m: Message{To: "jane@example.com"}, wantErr: "mailer: from"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := buildMessage(tc.from, tc.m, now)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			for _, line := range strings.Split(string(raw), "\r\n") {
				require.LessOrEqual(t, len(line), 998, "no line longer than RFC 5322 allows")
			}
			msg, err := mail.ReadMessage(bytes.NewReader(raw))
			require.NoError(t, err)
			tc.check(t, msg)
		})
	}
}

func TestLogAndFake_U(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{name: "the development mailer logs to and subject at info, the body only at debug, and sends nothing", run: func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			require.NoError(t, NewLog().Send(context.Background(), Message{To: "jane@example.com", Subject: "Hi", Text: "link"}))
			var records []map[string]any
			for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
				var rec map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &rec))
				records = append(records, rec)
			}
			require.Len(t, records, 2)
			require.Equal(t, "INFO", records[0]["level"])
			require.Equal(t, "jane@example.com", records[0]["to"])
			require.Equal(t, "Hi", records[0]["subject"])
			require.Nil(t, records[0]["text"], "the body (a link with a token) is never logged at info")
			require.Equal(t, "DEBUG", records[1]["level"])
			require.Equal(t, "link", records[1]["text"])
		}},
		{name: "the fake records every message and returns a copy", run: func(t *testing.T) {
			f := &Fake{}
			require.NoError(t, f.Send(context.Background(), Message{To: "a@example.com"}))
			f.Err = errors.New("down")
			require.EqualError(t, f.Send(context.Background(), Message{To: "b@example.com"}), "down")
			sent := f.Sent()
			require.Equal(t, []Message{{To: "a@example.com"}, {To: "b@example.com"}}, sent)
			sent[0].To = "changed"
			require.Equal(t, "a@example.com", f.Sent()[0].To)
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.run)
	}
}

// fakeSMTP is a minimal SMTP server for one connection: it greets, answers
// EHLO (advertising STARTTLS when starttls), upgrades on STARTTLS, takes
// AUTH PLAIN (rejecting it when authFails), MAIL, RCPT, DATA and QUIT, and
// records what it received.
type fakeSMTP struct {
	ln        net.Listener
	cert      tls.Certificate
	implicit  bool // TLS from the first byte (port 465 style)
	starttls  bool
	authFails bool
	// failOn is the step the server refuses: "GREETING", "STARTTLS",
	// "MAIL", "RCPT", "DATA", "END" (the message itself) or "QUIT".
	failOn string

	mu       sync.Mutex
	auth     string
	mailFrom string
	rcptTo   string
	data     string
	sawTLS   bool
	done     chan struct{}
}

func newFakeSMTP(t *testing.T, implicit, starttls, authFails bool, failOn string) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	s := &fakeSMTP{ln: ln, cert: selfSigned(t), implicit: implicit, starttls: starttls, authFails: authFails, failOn: failOn,
		done: make(chan struct{})}
	go s.serve()
	return s
}

func (s *fakeSMTP) serve() {
	defer close(s.done)
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12}
	if s.implicit {
		conn = tls.Server(conn, tlsCfg)
		s.sawTLS = true
	}
	r := bufio.NewReader(conn)
	say := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }
	if s.failOn == "GREETING" {
		say("554 go away")
		return
	}
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		if verb == s.failOn {
			say("554 refused")
			continue
		}
		switch verb {
		case "EHLO":
			if s.starttls && !s.sawTLS {
				say("250-fake")
				say("250 STARTTLS")
			} else {
				say("250-fake")
				say("250 AUTH PLAIN")
			}
		case "STARTTLS":
			say("220 go ahead")
			conn = tls.Server(conn, tlsCfg)
			r = bufio.NewReader(conn)
			s.mu.Lock()
			s.sawTLS = true
			s.mu.Unlock()
		case "AUTH":
			s.mu.Lock()
			s.auth = line
			s.mu.Unlock()
			if s.authFails {
				say("535 5.7.8 authentication failed")
			} else {
				say("235 ok")
			}
		case "MAIL":
			s.mu.Lock()
			s.mailFrom = line
			s.mu.Unlock()
			say("250 ok")
		case "RCPT":
			s.mu.Lock()
			s.rcptTo = line
			s.mu.Unlock()
			say("250 ok")
		case "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			if s.failOn == "END" {
				say("554 message refused")
				continue
			}
			s.mu.Lock()
			s.data = b.String()
			s.mu.Unlock()
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("502 unknown")
		}
	}
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestSMTP_Send_U(t *testing.T) {
	const password = "s3cret-pw"
	msg := Message{To: "Jane <jane@example.com>", Subject: "Reset your password", Text: "link"}

	testCases := []struct {
		name                         string
		port                         int
		implicit, starttls, authFail bool
		failOn                       string
		user                         string
		msg                          Message
		wantErr                      string
	}{
		{name: "STARTTLS: upgraded before AUTH, then the message delivered", port: 587, starttls: true, user: "mailer", msg: msg},
		{name: "port 465: TLS from the first byte", port: 465, implicit: true, user: "mailer", msg: msg},
		{name: "no username: no AUTH", port: 465, implicit: true, msg: msg},
		{name: "a server without STARTTLS is refused, nothing sent in plain text", port: 587, msg: msg,
			wantErr: "doesn't offer STARTTLS"},
		{name: "a refused login: the error never carries the password", port: 465, implicit: true, authFail: true,
			user: "mailer", msg: msg, wantErr: "mailer: auth as mailer"},
		{name: "a bad message is refused before dialling", port: 587, msg: Message{To: "nobody"}, wantErr: "mailer: recipient"},
		{name: "a refused greeting", port: 465, implicit: true, failOn: "GREETING", msg: msg, wantErr: "mailer: greeting"},
		{name: "a refused STARTTLS", port: 587, starttls: true, failOn: "STARTTLS", msg: msg, wantErr: "mailer: STARTTLS"},
		{name: "a refused sender", port: 465, implicit: true, failOn: "MAIL", msg: msg, wantErr: "mailer: MAIL FROM"},
		{name: "a refused recipient", port: 465, implicit: true, failOn: "RCPT", msg: msg, wantErr: "mailer: RCPT TO"},
		{name: "a refused DATA", port: 465, implicit: true, failOn: "DATA", msg: msg, wantErr: "mailer: DATA"},
		{name: "a refused message", port: 465, implicit: true, failOn: "END", msg: msg, wantErr: "mailer: end of message"},
		{name: "a refused QUIT after delivery is an error", port: 465, implicit: true, failOn: "QUIT", msg: msg, wantErr: "mailer: QUIT"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newFakeSMTP(t, tc.implicit, tc.starttls, tc.authFail, tc.failOn)
			s := NewSMTP(SMTPConfig{Host: "localhost", Port: tc.port, Username: tc.user, Password: password, From: from})
			s.addr = srv.ln.Addr().String()
			pool := x509.NewCertPool()
			leaf, err := x509.ParseCertificate(srv.cert.Certificate[0])
			require.NoError(t, err)
			pool.AddCert(leaf)
			s.tlsConfig = &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12}

			err = s.Send(context.Background(), tc.msg)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.NotContains(t, err.Error(), password)
				if tc.failOn != "QUIT" {
					srv.mu.Lock()
					require.Empty(t, srv.data, "nothing delivered")
					srv.mu.Unlock()
				}
				return
			}
			require.NoError(t, err)
			<-srv.done
			srv.mu.Lock()
			defer srv.mu.Unlock()
			require.True(t, srv.sawTLS, "the conversation was encrypted")
			require.Equal(t, "MAIL FROM:<no-reply@example.com>", srv.mailFrom)
			require.Equal(t, "RCPT TO:<jane@example.com>", srv.rcptTo)
			require.Contains(t, srv.data, "Subject: Reset your password")
			require.Equal(t, tc.user != "", srv.auth != "", "AUTH only with a username")
		})
	}
}

func TestSMTP_Defaults_U(t *testing.T) {
	testCases := []struct {
		name    string
		run     func(t *testing.T) error
		wantErr string
	}{
		{name: "it dials SMTP_HOST:SMTP_PORT; nothing listening is a dial error", wantErr: "mailer: dial", run: func(t *testing.T) error {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			port := ln.Addr().(*net.TCPAddr).Port
			require.NoError(t, ln.Close()) // nothing listens there now
			return NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: port, From: from}).Send(context.Background(), Message{To: "jane@example.com"})
		}},
		{name: "it verifies the server's certificate: a self-signed one fails STARTTLS", wantErr: "mailer: STARTTLS", run: func(t *testing.T) error {
			srv := newFakeSMTP(t, false, true, false, "")
			s := NewSMTP(SMTPConfig{Host: "localhost", Port: 587, From: from})
			s.addr = srv.ln.Addr().String()
			return s.Send(context.Background(), Message{To: "jane@example.com"})
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, tc.run(t), tc.wantErr)
		})
	}
}
