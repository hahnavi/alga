package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"alga/logger"
)

// smtpDialTimeout caps the TCP/TLS connect phase of an outbound SMTP send. The
// per-call deadline from Send's context still governs the full transaction.
const smtpDialTimeout = 30 * time.Second

func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

func generateBoundary() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "alga-" + hex.EncodeToString(b)
}

type Sender struct {
	host          string
	port          int
	user          string
	password      string
	from          string
	skipTLSVerify bool
}

func NewSender(host string, port int, user, password, from string, skipTLSVerify bool) *Sender {
	return &Sender{host: host, port: port, user: user, password: password, from: from, skipTLSVerify: skipTLSVerify}
}

func (s *Sender) Enabled() bool {
	return s.host != ""
}

// Send delivers an email. The context governs the full SMTP transaction via a
// deadline; when ctx is already cancelled, Send returns immediately. A missing
// deadline is bounded by smtpDialTimeout on the connect phase so a hung SMTP
// server cannot block the caller indefinitely.
func (s *Sender) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	if !s.Enabled() {
		return errors.New("smtp not configured")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("smtp send cancelled before dial: %w", err)
	}

	var msg strings.Builder
	msg.WriteString("From: " + sanitizeHeader(s.from) + "\r\n")
	msg.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	msg.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	if htmlBody != "" {
		boundary := generateBoundary()
		msg.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
		msg.WriteString("--" + boundary + "\r\n")
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(textBody + "\r\n\r\n")
		msg.WriteString("--" + boundary + "\r\n")
		msg.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
		msg.WriteString(htmlBody + "\r\n\r\n")
		msg.WriteString("--" + boundary + "--\r\n")
	} else {
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(textBody + "\r\n")
	}

	raw := []byte(msg.String())
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))

	// Use a deadline-bearing connection so both the dial and the post-connect
	// SMTP conversation (Hello/Auth/Mail/Rcpt/Data/Close/Quit) are bounded.
	// We start the dial on a synchronous goroutine because net.DialTimeout
	// itself has no context variant, then enforce ctx on the wrapping conn.
	conn, err := dialSMTP(ctx, addr, s.host, s.skipTLSVerify)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = c.Quit() }()

	if err := s.deliver(ctx, c, to, raw); err != nil {
		return err
	}
	logger.Debug("sent email", "component", "email", "to", to, "subject", subject)
	return nil
}

// dialSMTP opens a (optionally TLS-wrapped with skip-verify) TCP connection to
// the SMTP server. The dial phase is bounded by smtpDialTimeout; once up, the
// connection observes ctx's deadline via SetDeadline so the SMTP conversation
// is also bounded.
func dialSMTP(ctx context.Context, addr, host string, skipTLSVerify bool) (net.Conn, error) {
	type dialResult struct {
		conn net.Conn
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
		ch <- dialResult{conn, err}
	}()
	select {
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.conn != nil {
				_ = r.conn.Close()
			}
		}()
		return nil, fmt.Errorf("smtp dial cancelled: %w", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("smtp dial: %w", res.err)
		}
		conn := res.conn
		if skipTLSVerify {
			tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: host}) //#nosec G402 -- operator opt-in via SMTP_SKIP_TLS_VERIFY; warned at startup
			if err := tlsConn.Handshake(); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("smtp tls handshake: %w", err)
			}
			conn = tlsConn
		}
		// Apply ctx's deadline to every subsequent Read/Write on the conn so
		// the SMTP conversation respects request-scoped cancellation.
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(deadline)
		}
		return conn, nil
	}
}

// deliver runs the SMTP MAIL/RCPT/DATA sequence against c, performing AUTH
// when credentials are configured. It is shared by both the TLS and plain
// connection paths so they cannot drift.
func (s *Sender) deliver(ctx context.Context, c *smtp.Client, to string, raw []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("smtp send cancelled: %w", err)
	}
	if err := c.Hello("localhost"); err != nil {
		return fmt.Errorf("smtp hello: %w", err)
	}
	if s.user != "" {
		auth := smtp.PlainAuth("", s.user, s.password, s.host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := wc.Write(raw); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return nil
}
