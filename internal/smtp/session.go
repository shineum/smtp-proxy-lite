package smtp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/sungwon/smtp-proxy-lite/internal/parser"
	"github.com/sungwon/smtp-proxy-lite/internal/provider"
)

// Session states for the SMTP state machine.
const (
	stateConnected = iota
	stateGreeted
	stateAuthOK
	stateMailFrom
	stateRcptTo
	stateData
	stateDone
)

// idleTimeout is the maximum time a session can remain idle before being closed.
const idleTimeout = 60 * time.Second

// maxMessageSize is the default maximum message size (10 MB).
const maxMessageSize = 10 * 1024 * 1024

// Session represents a single SMTP client connection and manages the
// SMTP protocol state machine.
type Session struct {
	conn     net.Conn
	reader   *bufio.Reader
	writer   *bufio.Writer
	state    int
	auth     *Authenticator
	provider provider.Provider
	hostname string

	// TLS support
	tlsConfig *tls.Config
	tlsActive bool

	// Current transaction
	mailFrom   string
	rcptTo     []string
	dataBuffer strings.Builder
}

// NewSession creates a new SMTP session for the given connection.
func NewSession(conn net.Conn, auth *Authenticator, prov provider.Provider, hostname string, tlsConfig *tls.Config) *Session {
	return &Session{
		conn:      conn,
		reader:    bufio.NewReader(conn),
		writer:    bufio.NewWriter(conn),
		state:     stateConnected,
		auth:      auth,
		provider:  prov,
		hostname:  hostname,
		tlsConfig: tlsConfig,
	}
}

// Handle runs the SMTP session, processing commands until the client
// disconnects or an error occurs.
func (s *Session) Handle(ctx context.Context) {
	defer s.conn.Close()

	s.writeLine("220 %s ESMTP smtp-proxy-lite", s.hostname)

	for {
		select {
		case <-ctx.Done():
			s.writeLine("421 Service shutting down")
			return
		default:
		}

		if err := s.conn.SetDeadline(time.Now().Add(idleTimeout)); err != nil {
			slog.Error("failed to set connection deadline", "error", err)
			return
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				slog.Debug("connection read error", "error", err)
			}
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		cmd, arg := parseCommand(line)
		done := s.handleCommand(ctx, cmd, arg)
		if done {
			return
		}
	}
}

// handleCommand processes a single SMTP command and returns true if the session should end.
func (s *Session) handleCommand(ctx context.Context, cmd, arg string) bool {
	switch cmd {
	case "EHLO", "HELO":
		s.handleEHLO(cmd, arg)
	case "STARTTLS":
		s.handleSTARTTLS()
	case "AUTH":
		s.handleAUTH(arg)
	case "MAIL":
		s.handleMAIL(arg)
	case "RCPT":
		s.handleRCPT(arg)
	case "DATA":
		s.handleDATA(ctx)
	case "RSET":
		s.handleRSET()
	case "NOOP":
		s.writeLine("250 OK")
	case "QUIT":
		s.writeLine("221 Bye")
		return true
	default:
		s.writeLine("500 Unrecognized command")
	}
	return false
}

// handleEHLO processes EHLO/HELO commands.
func (s *Session) handleEHLO(cmd, arg string) {
	if arg == "" {
		s.writeLine("501 Syntax: %s hostname", cmd)
		return
	}

	if cmd == "HELO" {
		s.state = stateGreeted
		s.writeLine("250 %s Hello %s", s.hostname, arg)
		return
	}

	// EHLO response with capabilities
	s.state = stateGreeted
	s.writeLine("250-%s Hello %s", s.hostname, arg)

	if s.tlsConfig != nil && !s.tlsActive {
		s.writeLine("250-STARTTLS")
	}
	if s.auth.Enabled() {
		s.writeLine("250-AUTH PLAIN LOGIN")
	}
	s.writeLine("250-SIZE %d", maxMessageSize)
	s.writeLine("250 OK")
}

// handleSTARTTLS upgrades the connection to TLS.
func (s *Session) handleSTARTTLS() {
	if s.tlsConfig == nil {
		s.writeLine("454 TLS not available")
		return
	}
	if s.tlsActive {
		s.writeLine("454 TLS already active")
		return
	}

	s.writeLine("220 Ready to start TLS")

	tlsConn := tls.Server(s.conn, s.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		slog.Error("TLS handshake failed", "error", err)
		return
	}

	s.conn = tlsConn
	s.reader = bufio.NewReader(tlsConn)
	s.writer = bufio.NewWriter(tlsConn)
	s.tlsActive = true
	s.state = stateConnected
}

// handleAUTH processes AUTH commands (PLAIN and LOGIN mechanisms).
func (s *Session) handleAUTH(arg string) {
	if s.state < stateGreeted {
		s.writeLine("503 Send EHLO/HELO first")
		return
	}
	if !s.auth.Enabled() {
		s.writeLine("503 AUTH not available")
		return
	}

	parts := strings.SplitN(arg, " ", 2)
	mechanism := strings.ToUpper(parts[0])

	switch mechanism {
	case "PLAIN":
		s.handleAuthPlain(parts)
	case "LOGIN":
		s.handleAuthLogin()
	default:
		s.writeLine("504 Unrecognized authentication type")
	}
}

// handleAuthPlain processes AUTH PLAIN authentication.
func (s *Session) handleAuthPlain(parts []string) {
	var encoded string

	if len(parts) > 1 && parts[1] != "" {
		// Credentials provided inline: AUTH PLAIN <base64>
		encoded = parts[1]
	} else {
		// Challenge-response: send 334 and wait for credentials
		s.writeLine("334")
		line, err := s.reader.ReadString('\n')
		if err != nil {
			slog.Error("failed to read AUTH PLAIN response", "error", err)
			return
		}
		encoded = strings.TrimRight(line, "\r\n")
	}

	if encoded == "*" {
		s.writeLine("501 Authentication cancelled")
		return
	}

	if err := s.auth.VerifyPlain(encoded); err != nil {
		s.writeLine("535 Authentication failed")
		return
	}

	s.state = stateAuthOK
	s.writeLine("235 Authentication successful")
}

// handleAuthLogin processes AUTH LOGIN authentication via challenge-response.
func (s *Session) handleAuthLogin() {
	// Challenge for username (base64 encoded "Username:")
	s.writeLine("334 VXNlcm5hbWU6")
	userLine, err := s.reader.ReadString('\n')
	if err != nil {
		slog.Error("failed to read AUTH LOGIN username", "error", err)
		return
	}
	encodedUser := strings.TrimRight(userLine, "\r\n")

	if encodedUser == "*" {
		s.writeLine("501 Authentication cancelled")
		return
	}

	// Challenge for password (base64 encoded "Password:")
	s.writeLine("334 UGFzc3dvcmQ6")
	passLine, err := s.reader.ReadString('\n')
	if err != nil {
		slog.Error("failed to read AUTH LOGIN password", "error", err)
		return
	}
	encodedPass := strings.TrimRight(passLine, "\r\n")

	if encodedPass == "*" {
		s.writeLine("501 Authentication cancelled")
		return
	}

	if err := s.auth.VerifyLogin(encodedUser, encodedPass); err != nil {
		s.writeLine("535 Authentication failed")
		return
	}

	s.state = stateAuthOK
	s.writeLine("235 Authentication successful")
}

// handleMAIL processes the MAIL FROM command.
func (s *Session) handleMAIL(arg string) {
	if s.auth.Enabled() && s.state < stateAuthOK {
		s.writeLine("530 Authentication required")
		return
	}
	if s.state < stateGreeted {
		s.writeLine("503 Send EHLO/HELO first")
		return
	}

	upper := strings.ToUpper(arg)
	if !strings.HasPrefix(upper, "FROM:") {
		s.writeLine("501 Syntax: MAIL FROM:<address>")
		return
	}

	addr := extractAddress(arg[5:])
	if addr == "" {
		s.writeLine("501 Syntax: MAIL FROM:<address>")
		return
	}

	s.mailFrom = addr
	s.rcptTo = nil
	s.dataBuffer.Reset()
	s.state = stateMailFrom
	s.writeLine("250 OK")
}

// handleRCPT processes the RCPT TO command.
func (s *Session) handleRCPT(arg string) {
	if s.state < stateMailFrom {
		s.writeLine("503 Send MAIL FROM first")
		return
	}

	upper := strings.ToUpper(arg)
	if !strings.HasPrefix(upper, "TO:") {
		s.writeLine("501 Syntax: RCPT TO:<address>")
		return
	}

	addr := extractAddress(arg[3:])
	if addr == "" {
		s.writeLine("501 Syntax: RCPT TO:<address>")
		return
	}

	s.rcptTo = append(s.rcptTo, addr)
	s.state = stateRcptTo
	s.writeLine("250 OK")
}

// handleDATA processes the DATA command.
// @MX:WARN: [AUTO] DATA handler reads until dot-stuffed terminator; large messages may consume memory
// @MX:REASON: Unbounded read from network until \r\n.\r\n terminator
func (s *Session) handleDATA(ctx context.Context) {
	if s.state < stateRcptTo {
		s.writeLine("503 Send RCPT TO first")
		return
	}

	s.writeLine("354 Start mail input; end with <CRLF>.<CRLF>")

	var dataBuilder strings.Builder
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			slog.Error("error reading DATA", "error", err)
			return
		}

		// Check for end of data marker
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "." {
			break
		}

		// Dot-stuffing: lines starting with ".." have the leading dot removed
		if strings.HasPrefix(trimmed, "..") {
			line = line[1:]
		}

		dataBuilder.WriteString(line)
	}

	rawData := dataBuilder.String()

	// Parse the message
	msg, err := parser.Parse([]byte(rawData))
	if err != nil {
		slog.Error("failed to parse message", "error", err)
		s.writeLine("550 Failed to process message")
		s.resetTransaction()
		return
	}

	// Set envelope information if not present in parsed message
	if msg.From == "" {
		msg.From = s.mailFrom
	}
	if len(msg.To) == 0 {
		msg.To = s.rcptTo
	}

	// Send via provider
	if err := s.provider.Send(ctx, msg); err != nil {
		slog.Error("provider send failed",
			"provider", s.provider.Name(),
			"error", err,
		)
		// Map provider errors to SMTP response codes
		s.writeLine("451 Temporary failure, please try again later")
		s.resetTransaction()
		return
	}

	s.writeLine("250 OK message queued")
	s.resetTransaction()
}

// handleRSET resets the current transaction state.
func (s *Session) handleRSET() {
	s.resetTransaction()
	s.writeLine("250 OK")
}

// resetTransaction clears the current mail transaction state without
// affecting the session state (greeting, auth).
func (s *Session) resetTransaction() {
	s.mailFrom = ""
	s.rcptTo = nil
	s.dataBuffer.Reset()

	// Reset state to post-auth or post-greet
	if s.auth.Enabled() && s.state >= stateAuthOK {
		s.state = stateAuthOK
	} else if s.state >= stateGreeted {
		s.state = stateGreeted
	}
}

// writeLine writes a formatted line to the client, followed by \r\n.
func (s *Session) writeLine(format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	_, err := s.writer.WriteString(line + "\r\n")
	if err != nil {
		slog.Error("failed to write to client", "error", err)
		return
	}
	if err := s.writer.Flush(); err != nil {
		slog.Error("failed to flush to client", "error", err)
	}
}

// parseCommand splits an SMTP command line into the command verb and its argument.
func parseCommand(line string) (string, string) {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToUpper(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	return cmd, arg
}

// extractAddress extracts an email address from an SMTP parameter,
// handling both angle-bracket and bare formats.
func extractAddress(s string) string {
	s = strings.TrimSpace(s)

	// Handle angle-bracket format: <user@example.com>
	if strings.HasPrefix(s, "<") {
		end := strings.Index(s, ">")
		if end < 0 {
			return ""
		}
		return s[1:end]
	}

	// Bare address format
	return s
}
