package ses

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/shineum/smtp-proxy-lite/internal/email"
)

// mockSESClient implements SendEmailAPI for testing.
type mockSESClient struct {
	sendFn    func(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
	callCount int
	lastInput *sesv2.SendEmailInput
}

func (m *mockSESClient) SendEmail(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	m.callCount++
	m.lastInput = params
	if m.sendFn != nil {
		return m.sendFn(ctx, params, optFns...)
	}
	return &sesv2.SendEmailOutput{MessageId: aws.String("test-message-id")}, nil
}

func TestName(t *testing.T) {
	t.Parallel()
	p := NewWithClient("sender@example.com", &mockSESClient{})
	if got := p.Name(); got != "ses" {
		t.Errorf("Name(): got %q, want %q", got, "ses")
	}
}

func TestSend_SimpleTextEmail(t *testing.T) {
	t.Parallel()

	mock := &mockSESClient{}
	p := NewWithClient("sender@example.com", mock)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"to@example.com"},
		Subject:  "Test Subject",
		TextBody: "Hello, World!",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.callCount != 1 {
		t.Errorf("call count: got %d, want 1", mock.callCount)
	}

	input := mock.lastInput
	if input.Content.Simple == nil {
		t.Fatal("expected simple email content, got nil")
	}
	if got := *input.FromEmailAddress; got != "sender@example.com" {
		t.Errorf("FromEmailAddress: got %q, want %q", got, "sender@example.com")
	}
	if got := *input.Content.Simple.Subject.Data; got != "Test Subject" {
		t.Errorf("Subject: got %q, want %q", got, "Test Subject")
	}
	if got := *input.Content.Simple.Body.Text.Data; got != "Hello, World!" {
		t.Errorf("TextBody: got %q, want %q", got, "Hello, World!")
	}
	if input.Content.Simple.Body.Html != nil {
		t.Error("expected no HTML body")
	}
}

func TestSend_SimpleHtmlEmail(t *testing.T) {
	t.Parallel()

	mock := &mockSESClient{}
	p := NewWithClient("sender@example.com", mock)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"to@example.com"},
		Subject:  "HTML Test",
		TextBody: "Plain text fallback",
		HtmlBody: "<h1>Hello</h1>",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := mock.lastInput
	if got := *input.Content.Simple.Body.Html.Data; got != "<h1>Hello</h1>" {
		t.Errorf("HtmlBody: got %q, want %q", got, "<h1>Hello</h1>")
	}
	if got := *input.Content.Simple.Body.Text.Data; got != "Plain text fallback" {
		t.Errorf("TextBody: got %q, want %q", got, "Plain text fallback")
	}
}

func TestSend_WithRecipients(t *testing.T) {
	t.Parallel()

	mock := &mockSESClient{}
	p := NewWithClient("sender@example.com", mock)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"to1@example.com", "to2@example.com"},
		Cc:       []string{"cc@example.com"},
		Bcc:      []string{"bcc@example.com"},
		Subject:  "Multi-recipient",
		TextBody: "Hello",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := mock.lastInput.Destination
	if len(dest.ToAddresses) != 2 {
		t.Errorf("ToAddresses: got %d, want 2", len(dest.ToAddresses))
	}
	if len(dest.CcAddresses) != 1 {
		t.Errorf("CcAddresses: got %d, want 1", len(dest.CcAddresses))
	}
	if len(dest.BccAddresses) != 1 {
		t.Errorf("BccAddresses: got %d, want 1", len(dest.BccAddresses))
	}
}

func TestSend_WithAttachments(t *testing.T) {
	t.Parallel()

	mock := &mockSESClient{}
	p := NewWithClient("sender@example.com", mock)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"to@example.com"},
		Subject:  "With Attachment",
		TextBody: "See attachment",
		Attachments: []email.Attachment{
			{
				Filename:    "test.txt",
				ContentType: "text/plain",
				Content:     []byte("file content"),
			},
		},
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := mock.lastInput
	if input.Content.Raw == nil {
		t.Fatal("expected raw email content for attachment, got nil")
	}
	if input.Content.Simple != nil {
		t.Error("expected no simple content when using raw message")
	}

	rawStr := string(input.Content.Raw.Data)
	if !strings.Contains(rawStr, "From: sender@example.com") {
		t.Error("raw message missing From header")
	}
	if !strings.Contains(rawStr, "To: to@example.com") {
		t.Error("raw message missing To header")
	}
	if !strings.Contains(rawStr, "Subject: With Attachment") {
		t.Error("raw message missing Subject header")
	}
	if !strings.Contains(rawStr, "multipart/mixed") {
		t.Error("raw message missing multipart/mixed content type")
	}
	if !strings.Contains(rawStr, "text/plain") {
		t.Error("raw message missing text body content type")
	}
	if !strings.Contains(rawStr, "test.txt") {
		t.Error("raw message missing attachment filename")
	}
}

func TestSend_RetryOnError(t *testing.T) {
	t.Parallel()

	callCount := 0
	mock := &mockSESClient{
		sendFn: func(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			callCount++
			if callCount <= 2 {
				return nil, errors.New("transient error")
			}
			return &sesv2.SendEmailOutput{MessageId: aws.String("ok")}, nil
		},
	}
	p := NewWithClient("sender@example.com", mock)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"to@example.com"},
		Subject:  "Retry Test",
		TextBody: "Hello",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("call count: got %d, want 3", callCount)
	}
}

func TestSend_AllRetriesExhausted(t *testing.T) {
	t.Parallel()

	mock := &mockSESClient{
		sendFn: func(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			return nil, errors.New("persistent error")
		},
	}
	p := NewWithClient("sender@example.com", mock)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"to@example.com"},
		Subject:  "Fail Test",
		TextBody: "Hello",
	}

	err := p.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if !strings.Contains(err.Error(), "after 3 retries") {
		t.Errorf("error message: got %q, want to contain 'after 3 retries'", err.Error())
	}
	// 1 initial + 3 retries = 4 total
	if mock.callCount != 4 {
		t.Errorf("call count: got %d, want 4", mock.callCount)
	}
}

func TestSend_ContextCancelled(t *testing.T) {
	t.Parallel()

	mock := &mockSESClient{
		sendFn: func(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			return nil, errors.New("error")
		},
	}
	p := NewWithClient("sender@example.com", mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"to@example.com"},
		Subject:  "Cancel Test",
		TextBody: "Hello",
	}

	err := p.Send(ctx, msg)
	if err == nil {
		t.Fatal("expected error when context cancelled")
	}
}

func TestBuildSimpleInput(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		To:       []string{"to@example.com"},
		Cc:       []string{"cc@example.com"},
		Bcc:      []string{"bcc@example.com"},
		Subject:  "Test",
		TextBody: "text",
		HtmlBody: "<p>html</p>",
	}

	input := buildSimpleInput("sender@example.com", msg)

	if got := *input.FromEmailAddress; got != "sender@example.com" {
		t.Errorf("FromEmailAddress: got %q, want %q", got, "sender@example.com")
	}
	if input.Content.Simple.Body.Html == nil {
		t.Fatal("expected HTML body")
	}
	if input.Content.Simple.Body.Text == nil {
		t.Fatal("expected text body")
	}
	if got := *input.Content.Simple.Body.Html.Charset; got != "UTF-8" {
		t.Errorf("HTML charset: got %q, want %q", got, "UTF-8")
	}
}

func TestBuildRawMessage(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		To:        []string{"to@example.com"},
		Cc:        []string{"cc@example.com"},
		Subject:   "Raw Test",
		TextBody:  "text body",
		MessageID: "<msg-123@example.com>",
		Attachments: []email.Attachment{
			{
				Filename:    "doc.pdf",
				ContentType: "application/pdf",
				Content:     []byte("pdf content"),
			},
		},
	}

	raw, err := buildRawMessage("sender@example.com", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rawStr := string(raw)
	checks := []struct {
		name     string
		contains string
	}{
		{"From header", "From: sender@example.com"},
		{"To header", "To: to@example.com"},
		{"Cc header", "Cc: cc@example.com"},
		{"Subject header", "Subject: Raw Test"},
		{"Message-ID header", "Message-ID: <msg-123@example.com>"},
		{"MIME-Version", "MIME-Version: 1.0"},
		{"multipart boundary", "multipart/mixed"},
		{"body content type", "text/plain"},
		{"attachment content type", "application/pdf"},
		{"attachment filename", "doc.pdf"},
		{"base64 encoding", "Content-Transfer-Encoding: base64"},
	}

	for _, check := range checks {
		if !strings.Contains(rawStr, check.contains) {
			t.Errorf("raw message missing %s: expected to contain %q", check.name, check.contains)
		}
	}
}

func TestBuildRawMessage_HtmlBody(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		To:       []string{"to@example.com"},
		Subject:  "HTML Raw",
		HtmlBody: "<h1>Hello</h1>",
		Attachments: []email.Attachment{
			{Filename: "a.txt", ContentType: "text/plain", Content: []byte("x")},
		},
	}

	raw, err := buildRawMessage("sender@example.com", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(raw), "text/html") {
		t.Error("expected text/html content type for HTML body")
	}
}

func TestEncodeBase64WithLineBreaks(t *testing.T) {
	t.Parallel()

	// Create data that produces a long base64 string
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}

	encoded := encodeBase64WithLineBreaks(data)
	lines := strings.Split(encoded, "\r\n")
	for i, line := range lines {
		if i < len(lines)-1 && len(line) != 76 {
			t.Errorf("line %d length: got %d, want 76", i, len(line))
		}
		if len(line) > 76 {
			t.Errorf("line %d exceeds 76 chars: got %d", i, len(line))
		}
	}
}

func TestBackoffDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
	}

	for _, tt := range tests {
		if got := backoffDelay(tt.attempt); got != tt.want {
			t.Errorf("backoffDelay(%d): got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

// Verify SESProvider implements provider.Provider interface
func TestProviderInterface(t *testing.T) {
	t.Parallel()

	var _ interface {
		Send(ctx context.Context, msg *email.Email) error
		Name() string
	} = (*SESProvider)(nil)
}
