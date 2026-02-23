package smtp

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sungwon/smtp-proxy-lite/internal/email"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	lastMsg *email.Email
	sendErr error
}

func (m *mockProvider) Send(_ context.Context, msg *email.Email) error {
	m.lastMsg = msg
	return m.sendErr
}

func (m *mockProvider) Name() string {
	return "mock"
}

// connPair creates a connected pair of net.Conn for testing SMTP sessions.
func connPair(t *testing.T) (client net.Conn, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	done := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		done <- conn
	}()

	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	server = <-done
	return client, server
}

// readLine reads a line from a buffered reader with a timeout.
func readLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read line: %v", err)
	}
	return strings.TrimRight(line, "\r\n")
}

// sendCmd sends a command to the SMTP session.
func sendCmd(t *testing.T, conn net.Conn, cmd string) {
	t.Helper()
	_, err := conn.Write([]byte(cmd + "\r\n"))
	if err != nil {
		t.Fatalf("failed to write command: %v", err)
	}
}

func TestSession_Greeting(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	greeting := readLine(t, reader)

	if !strings.HasPrefix(greeting, "220 ") {
		t.Errorf("greeting: got %q, want prefix '220 '", greeting)
	}
	if !strings.Contains(greeting, "mail.test.com") {
		t.Errorf("greeting should contain hostname, got %q", greeting)
	}
}

func TestSession_EHLO(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("user", "pass")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	sendCmd(t, client, "EHLO client.test.com")

	// Read all EHLO responses
	var ehloLines []string
	for {
		line := readLine(t, reader)
		ehloLines = append(ehloLines, line)
		if !strings.HasPrefix(line, "250-") {
			break
		}
	}

	// Verify capabilities
	foundAuth := false
	foundSize := false
	for _, line := range ehloLines {
		if strings.Contains(line, "AUTH PLAIN LOGIN") {
			foundAuth = true
		}
		if strings.Contains(line, "SIZE") {
			foundSize = true
		}
	}

	if !foundAuth {
		t.Error("EHLO response missing AUTH capability")
	}
	if !foundSize {
		t.Error("EHLO response missing SIZE capability")
	}
}

func TestSession_HELO(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	sendCmd(t, client, "HELO client.test.com")
	response := readLine(t, reader)

	if !strings.HasPrefix(response, "250 ") {
		t.Errorf("HELO response: got %q, want prefix '250 '", response)
	}
}

func TestSession_QUIT(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	sendCmd(t, client, "QUIT")
	response := readLine(t, reader)

	if !strings.HasPrefix(response, "221 ") {
		t.Errorf("QUIT response: got %q, want prefix '221 '", response)
	}
}

func TestSession_NOOP(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	sendCmd(t, client, "NOOP")
	response := readLine(t, reader)

	if !strings.HasPrefix(response, "250 ") {
		t.Errorf("NOOP response: got %q, want prefix '250 '", response)
	}
}

func TestSession_MailTransaction_NoAuth(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	// EHLO
	sendCmd(t, client, "EHLO client.test.com")
	for {
		line := readLine(t, reader)
		if !strings.HasPrefix(line, "250-") {
			break
		}
	}

	// MAIL FROM
	sendCmd(t, client, "MAIL FROM:<sender@example.com>")
	resp := readLine(t, reader)
	if !strings.HasPrefix(resp, "250 ") {
		t.Errorf("MAIL FROM response: got %q, want prefix '250 '", resp)
	}

	// RCPT TO
	sendCmd(t, client, "RCPT TO:<recipient@example.com>")
	resp = readLine(t, reader)
	if !strings.HasPrefix(resp, "250 ") {
		t.Errorf("RCPT TO response: got %q, want prefix '250 '", resp)
	}

	// DATA
	sendCmd(t, client, "DATA")
	resp = readLine(t, reader)
	if !strings.HasPrefix(resp, "354 ") {
		t.Errorf("DATA response: got %q, want prefix '354 '", resp)
	}

	// Send message content
	message := strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: Test Email",
		"Content-Type: text/plain",
		"",
		"Hello, this is a test email.",
		".",
	}, "\r\n")
	_, err := client.Write([]byte(message + "\r\n"))
	if err != nil {
		t.Fatalf("failed to write DATA: %v", err)
	}

	resp = readLine(t, reader)
	if !strings.HasPrefix(resp, "250 ") {
		t.Errorf("DATA completion response: got %q, want prefix '250 '", resp)
	}

	// Verify provider received the message
	if prov.lastMsg == nil {
		t.Fatal("provider did not receive message")
	}
	if prov.lastMsg.Subject != "Test Email" {
		t.Errorf("Subject: got %q, want %q", prov.lastMsg.Subject, "Test Email")
	}
}

func TestSession_RSET(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	// EHLO
	sendCmd(t, client, "EHLO client.test.com")
	for {
		line := readLine(t, reader)
		if !strings.HasPrefix(line, "250-") {
			break
		}
	}

	// MAIL FROM
	sendCmd(t, client, "MAIL FROM:<sender@example.com>")
	readLine(t, reader) // 250 OK

	// RSET
	sendCmd(t, client, "RSET")
	resp := readLine(t, reader)
	if !strings.HasPrefix(resp, "250 ") {
		t.Errorf("RSET response: got %q, want prefix '250 '", resp)
	}

	// Verify state is reset -- RCPT TO should fail without MAIL FROM
	sendCmd(t, client, "RCPT TO:<recipient@example.com>")
	resp = readLine(t, reader)
	if !strings.HasPrefix(resp, "503 ") {
		t.Errorf("RCPT TO after RSET: got %q, want prefix '503 '", resp)
	}
}

func TestSession_StateOrderEnforcement(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("user", "pass")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	// MAIL FROM before EHLO should fail
	sendCmd(t, client, "MAIL FROM:<sender@example.com>")
	resp := readLine(t, reader)
	if !strings.HasPrefix(resp, "503 ") {
		t.Errorf("MAIL FROM before EHLO: got %q, want prefix '503 '", resp)
	}

	// EHLO first
	sendCmd(t, client, "EHLO client.test.com")
	for {
		line := readLine(t, reader)
		if !strings.HasPrefix(line, "250-") {
			break
		}
	}

	// MAIL FROM without AUTH should fail when auth is enabled
	sendCmd(t, client, "MAIL FROM:<sender@example.com>")
	resp = readLine(t, reader)
	if !strings.HasPrefix(resp, "530 ") {
		t.Errorf("MAIL FROM without AUTH: got %q, want prefix '530 '", resp)
	}

	// RCPT TO before MAIL FROM should fail
	sendCmd(t, client, "RCPT TO:<recipient@example.com>")
	resp = readLine(t, reader)
	if !strings.HasPrefix(resp, "503 ") {
		t.Errorf("RCPT TO before MAIL FROM: got %q, want prefix '503 '", resp)
	}

	// DATA before RCPT TO should fail
	sendCmd(t, client, "DATA")
	resp = readLine(t, reader)
	if !strings.HasPrefix(resp, "503 ") {
		t.Errorf("DATA before RCPT TO: got %q, want prefix '503 '", resp)
	}
}

func TestSession_UnknownCommand(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	sendCmd(t, client, "INVALID")
	resp := readLine(t, reader)
	if !strings.HasPrefix(resp, "500 ") {
		t.Errorf("unknown command response: got %q, want prefix '500 '", resp)
	}
}

func TestSession_EHLO_MissingHostname(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("", "")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	sendCmd(t, client, "EHLO")
	resp := readLine(t, reader)
	if !strings.HasPrefix(resp, "501 ") {
		t.Errorf("EHLO without hostname: got %q, want prefix '501 '", resp)
	}
}

func TestParseCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		wantCmd string
		wantArg string
	}{
		{"EHLO client.test.com", "EHLO", "client.test.com"},
		{"MAIL FROM:<user@example.com>", "MAIL", "FROM:<user@example.com>"},
		{"RCPT TO:<user@example.com>", "RCPT", "TO:<user@example.com>"},
		{"DATA", "DATA", ""},
		{"QUIT", "QUIT", ""},
		{"ehlo client.test.com", "EHLO", "client.test.com"},
		{"AUTH PLAIN dGVzdA==", "AUTH", "PLAIN dGVzdA=="},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			cmd, arg := parseCommand(tt.input)
			if cmd != tt.wantCmd {
				t.Errorf("command: got %q, want %q", cmd, tt.wantCmd)
			}
			if arg != tt.wantArg {
				t.Errorf("arg: got %q, want %q", arg, tt.wantArg)
			}
		})
	}
}

func TestExtractAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"<user@example.com>", "user@example.com"},
		{"  <user@example.com>  ", "user@example.com"},
		{"user@example.com", "user@example.com"},
		{"<>", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := extractAddress(tt.input)
			if got != tt.want {
				t.Errorf("extractAddress(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSession_AuthBeforeMailFrom(t *testing.T) {
	t.Parallel()

	client, server := connPair(t)
	defer client.Close()

	prov := &mockProvider{}
	auth := NewAuthenticator("user", "pass")
	sess := NewSession(server, auth, prov, "mail.test.com", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go sess.Handle(ctx)

	reader := bufio.NewReader(client)
	readLine(t, reader) // Skip greeting

	// AUTH before EHLO should fail
	sendCmd(t, client, "AUTH PLAIN dGVzdA==")
	resp := readLine(t, reader)
	if !strings.HasPrefix(resp, "503 ") {
		t.Errorf("AUTH before EHLO: got %q, want prefix '503 '", resp)
	}
}
