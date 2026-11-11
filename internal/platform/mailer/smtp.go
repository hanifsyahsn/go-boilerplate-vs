package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"time"
)

// sendTimeout bounds one email, from the dial to QUIT.
const sendTimeout = 15 * time.Second

// SMTPConfig is the outgoing mail server (SMTP_*).
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// SMTP sends through a mail server: port 465 is implicit TLS, any other port
// must offer STARTTLS (it never sends in plain text). The password never
// reaches a log or an error string.
type SMTP struct {
	cfg SMTPConfig
	// tlsConfig and addr are seams for tests (a self-signed server on a
	// random port); nil / "" in prod.
	tlsConfig *tls.Config
	addr      string
	now       func() time.Time
}

func NewSMTP(cfg SMTPConfig) *SMTP {
	return &SMTP{cfg: cfg, now: time.Now}
}

// errNoSTARTTLS refuses a server that would take the message in plain text.
var errNoSTARTTLS = errors.New("mailer: the SMTP server doesn't offer STARTTLS; refusing to send in plain text")

func (s *SMTP) Send(ctx context.Context, m Message) error {
	msg, err := buildMessage(s.cfg.From, m, s.now())
	if err != nil {
		return err
	}
	sender, _ := mail.ParseAddress(s.cfg.From) // buildMessage parsed it
	recipient, _ := mail.ParseAddress(m.To)    // buildMessage parsed it

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	addr := s.addr
	if addr == "" {
		addr = net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mailer: dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline) // every read / write below stops at the send timeout
	}

	tlsConfig := s.tlsConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	}
	if s.cfg.Port == 465 {
		conn = tls.Client(conn, tlsConfig)
	}

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("mailer: greeting: %w", err)
	}
	defer func() { _ = c.Close() }()

	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errNoSTARTTLS
		}
		if err = c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("mailer: STARTTLS: %w", err)
		}
	}
	if s.cfg.Username != "" {
		// The error never carries the password: net/smtp reports the
		// server's reply, not the credentials sent.
		if err = c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mailer: auth as %s: %w", s.cfg.Username, err)
		}
	}
	if err = c.Mail(sender.Address); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	if err = c.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("mailer: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("mailer: write message: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("mailer: end of message: %w", err)
	}
	if err = c.Quit(); err != nil {
		return fmt.Errorf("mailer: QUIT: %w", err)
	}
	return nil
}
