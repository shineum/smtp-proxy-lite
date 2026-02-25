package parser

import (
	"strings"
	"testing"
)

func TestParsePlainTextEmail(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: Test Subject",
		"Message-Id: <test123@example.com>",
		"Content-Type: text/plain",
		"",
		"Hello, this is a plain text email.",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.From != "sender@example.com" {
		t.Errorf("From: got %q, want %q", msg.From, "sender@example.com")
	}
	if len(msg.To) != 1 || msg.To[0] != "recipient@example.com" {
		t.Errorf("To: got %v, want [recipient@example.com]", msg.To)
	}
	if msg.Subject != "Test Subject" {
		t.Errorf("Subject: got %q, want %q", msg.Subject, "Test Subject")
	}
	if msg.MessageID != "<test123@example.com>" {
		t.Errorf("MessageID: got %q, want %q", msg.MessageID, "<test123@example.com>")
	}
	if msg.TextBody != "Hello, this is a plain text email." {
		t.Errorf("TextBody: got %q, want %q", msg.TextBody, "Hello, this is a plain text email.")
	}
	if msg.HtmlBody != "" {
		t.Errorf("HtmlBody: got %q, want empty", msg.HtmlBody)
	}
	if len(msg.Attachments) != 0 {
		t.Errorf("Attachments: got %d, want 0", len(msg.Attachments))
	}
}

func TestParseMultipartTextAndHTML(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"To: alice@example.com, bob@example.com",
		"Cc: carol@example.com",
		"Subject: Multipart Test",
		"Content-Type: multipart/alternative; boundary=boundary123",
		"",
		"--boundary123",
		"Content-Type: text/plain",
		"",
		"Plain text body",
		"--boundary123",
		"Content-Type: text/html",
		"",
		"<html><body><p>HTML body</p></body></html>",
		"--boundary123--",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.From != "sender@example.com" {
		t.Errorf("From: got %q, want %q", msg.From, "sender@example.com")
	}
	if len(msg.To) != 2 {
		t.Fatalf("To: got %d recipients, want 2", len(msg.To))
	}
	if msg.To[0] != "alice@example.com" {
		t.Errorf("To[0]: got %q, want %q", msg.To[0], "alice@example.com")
	}
	if msg.To[1] != "bob@example.com" {
		t.Errorf("To[1]: got %q, want %q", msg.To[1], "bob@example.com")
	}
	if len(msg.Cc) != 1 || msg.Cc[0] != "carol@example.com" {
		t.Errorf("Cc: got %v, want [carol@example.com]", msg.Cc)
	}
	if msg.TextBody != "Plain text body" {
		t.Errorf("TextBody: got %q, want %q", msg.TextBody, "Plain text body")
	}
	if msg.HtmlBody != "<html><body><p>HTML body</p></body></html>" {
		t.Errorf("HtmlBody: got %q, want %q", msg.HtmlBody, "<html><body><p>HTML body</p></body></html>")
	}
}

func TestParseEmailWithAttachments(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: With Attachment",
		"Content-Type: multipart/mixed; boundary=mixedboundary",
		"",
		"--mixedboundary",
		"Content-Type: text/plain",
		"",
		"Email body text",
		"--mixedboundary",
		"Content-Type: application/pdf; name=\"report.pdf\"",
		"Content-Disposition: attachment; filename=\"report.pdf\"",
		"Content-Transfer-Encoding: base64",
		"",
		"SGVsbG8gV29ybGQ=",
		"--mixedboundary--",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.TextBody != "Email body text" {
		t.Errorf("TextBody: got %q, want %q", msg.TextBody, "Email body text")
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("Attachments: got %d, want 1", len(msg.Attachments))
	}

	att := msg.Attachments[0]
	if att.Filename != "report.pdf" {
		t.Errorf("Attachment Filename: got %q, want %q", att.Filename, "report.pdf")
	}
	if att.ContentType != "application/pdf" {
		t.Errorf("Attachment ContentType: got %q, want %q", att.ContentType, "application/pdf")
	}
	if string(att.Content) != "Hello World" {
		t.Errorf("Attachment Content: got %q, want %q", string(att.Content), "Hello World")
	}
}

func TestParseMalformedMIME(t *testing.T) {
	t.Parallel()

	t.Run("completely invalid message", func(t *testing.T) {
		t.Parallel()
		raw := []byte("not a valid email at all\x00\x01\x02")
		_, err := Parse(raw)
		if err == nil {
			t.Error("expected error for completely invalid message, got nil")
		}
	})

	t.Run("missing content type defaults to text/plain", func(t *testing.T) {
		t.Parallel()
		raw := []byte(strings.Join([]string{
			"From: sender@example.com",
			"To: recipient@example.com",
			"Subject: No Content Type",
			"",
			"Body without content type header",
		}, "\r\n"))

		msg, err := Parse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.TextBody != "Body without content type header" {
			t.Errorf("TextBody: got %q, want %q", msg.TextBody, "Body without content type header")
		}
	})

	t.Run("multipart missing boundary", func(t *testing.T) {
		t.Parallel()
		raw := []byte(strings.Join([]string{
			"From: sender@example.com",
			"To: recipient@example.com",
			"Content-Type: multipart/mixed",
			"",
			"some body",
		}, "\r\n"))

		_, err := Parse(raw)
		if err == nil {
			t.Error("expected error for multipart missing boundary, got nil")
		}
	})
}

func TestParseMultipleRecipients(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"To: alice@example.com, bob@example.com, carol@example.com",
		"Bcc: secret@example.com",
		"Subject: Multiple Recipients",
		"Content-Type: text/plain",
		"",
		"Hello everyone",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg.To) != 3 {
		t.Fatalf("To: got %d recipients, want 3", len(msg.To))
	}
	if len(msg.Bcc) != 1 || msg.Bcc[0] != "secret@example.com" {
		t.Errorf("Bcc: got %v, want [secret@example.com]", msg.Bcc)
	}
}

func TestParseEmptyAddressFields(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"Subject: No To",
		"Content-Type: text/plain",
		"",
		"Body",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.To != nil {
		t.Errorf("To: got %v, want nil", msg.To)
	}
	if msg.Cc != nil {
		t.Errorf("Cc: got %v, want nil", msg.Cc)
	}
	if msg.Bcc != nil {
		t.Errorf("Bcc: got %v, want nil", msg.Bcc)
	}
}

func TestParseRawHeaders(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"X-Custom-Header: custom-value",
		"Subject: Headers Test",
		"Content-Type: text/plain",
		"",
		"Body",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.RawHeaders == nil {
		t.Fatal("RawHeaders is nil")
	}
	if vals, ok := msg.RawHeaders["X-Custom-Header"]; !ok || len(vals) == 0 || vals[0] != "custom-value" {
		t.Errorf("X-Custom-Header: got %v, want [custom-value]", vals)
	}
}

func TestParseBase64AttachmentWithCRLF(t *testing.T) {
	t.Parallel()

	raw := []byte("From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: CRLF Base64\r\n" +
		"Content-Type: multipart/mixed; boundary=bound\r\n" +
		"\r\n" +
		"--bound\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"body\r\n" +
		"--bound\r\n" +
		"Content-Type: application/pdf; name=\"file.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"file.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"SGVs\r\n" +
		"bG8g\r\n" +
		"V29y\r\n" +
		"bGQ=\r\n" +
		"--bound--\r\n")

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg.Attachments) != 1 {
		t.Fatalf("Attachments: got %d, want 1", len(msg.Attachments))
	}

	att := msg.Attachments[0]
	if att.Filename != "file.pdf" {
		t.Errorf("Filename: got %q, want %q", att.Filename, "file.pdf")
	}
	if string(att.Content) != "Hello World" {
		t.Errorf("Content: got %q, want %q", string(att.Content), "Hello World")
	}
}

func TestParseAttachmentWithoutFilename(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: No Filename",
		"Content-Type: multipart/mixed; boundary=bound",
		"",
		"--bound",
		"Content-Type: text/plain",
		"",
		"body",
		"--bound",
		"Content-Type: application/pdf",
		"Content-Disposition: attachment",
		"Content-Transfer-Encoding: base64",
		"",
		"SGVsbG8gV29ybGQ=",
		"--bound--",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg.Attachments) != 1 {
		t.Fatalf("Attachments: got %d, want 1", len(msg.Attachments))
	}

	att := msg.Attachments[0]
	if att.Filename == "" {
		t.Error("Filename should not be empty for attachments without explicit filename")
	}
	if att.Filename != "attachment.pdf" {
		t.Errorf("Filename: got %q, want %q", att.Filename, "attachment.pdf")
	}
	if string(att.Content) != "Hello World" {
		t.Errorf("Content: got %q, want %q", string(att.Content), "Hello World")
	}
}

func TestParseNestedMultipart(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: Nested Multipart",
		"Content-Type: multipart/mixed; boundary=outer",
		"",
		"--outer",
		"Content-Type: multipart/alternative; boundary=inner",
		"",
		"--inner",
		"Content-Type: text/plain",
		"",
		"Plain text part",
		"--inner",
		"Content-Type: text/html",
		"",
		"<p>HTML part</p>",
		"--inner--",
		"--outer",
		"Content-Type: application/octet-stream; name=\"data.bin\"",
		"Content-Disposition: attachment; filename=\"data.bin\"",
		"",
		"binarydata",
		"--outer--",
	}, "\r\n"))

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.TextBody != "Plain text part" {
		t.Errorf("TextBody: got %q, want %q", msg.TextBody, "Plain text part")
	}
	if msg.HtmlBody != "<p>HTML part</p>" {
		t.Errorf("HtmlBody: got %q, want %q", msg.HtmlBody, "<p>HTML part</p>")
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("Attachments: got %d, want 1", len(msg.Attachments))
	}
	if msg.Attachments[0].Filename != "data.bin" {
		t.Errorf("Attachment Filename: got %q, want %q", msg.Attachments[0].Filename, "data.bin")
	}
}
