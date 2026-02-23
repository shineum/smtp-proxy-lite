package stdout

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sungwon/smtp-proxy-lite/internal/email"
)

func TestSend_BasicEmail(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewWithWriter(&buf)

	msg := &email.Email{
		From:    "sender@example.com",
		To:      []string{"alice@example.com", "bob@example.com"},
		Subject: "Monthly Report",
		TextBody: "Please find the report attached.",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "From: sender@example.com") {
		t.Error("output missing From header")
	}
	if !strings.Contains(output, "To: alice@example.com, bob@example.com") {
		t.Error("output missing To header")
	}
	if !strings.Contains(output, "Subject: Monthly Report") {
		t.Error("output missing Subject header")
	}
	if !strings.Contains(output, "Please find the report attached.") {
		t.Error("output missing body text")
	}
	if strings.Contains(output, "Attachments:") {
		t.Error("output should not contain Attachments line when there are none")
	}
	if !strings.HasPrefix(output, "========================================\n") {
		t.Error("output should start with separator line")
	}
	if !strings.HasSuffix(output, "========================================\n") {
		t.Error("output should end with separator line")
	}
}

func TestSend_WithCc(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewWithWriter(&buf)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"alice@example.com"},
		Cc:       []string{"carol@example.com"},
		Subject:  "With CC",
		TextBody: "Hello",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Cc: carol@example.com") {
		t.Error("output missing Cc header")
	}
}

func TestSend_NoCc(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewWithWriter(&buf)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"recipient@example.com"},
		Subject:  "No CC",
		TextBody: "Body",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "Cc:") {
		t.Error("output should not contain Cc line when there are no Cc recipients")
	}
}

func TestSend_WithAttachments(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewWithWriter(&buf)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"alice@example.com", "bob@example.com"},
		Cc:       []string{"carol@example.com"},
		Subject:  "Monthly Report",
		TextBody: "Please find the report attached.",
		Attachments: []email.Attachment{
			{
				Filename:    "report.pdf",
				ContentType: "application/pdf",
				Content:     make([]byte, 1258291), // ~1.2 MB
			},
			{
				Filename:    "summary.xlsx",
				ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				Content:     make([]byte, 46080), // ~45 KB
			},
		},
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Attachments:") {
		t.Error("output missing Attachments line")
	}
	if !strings.Contains(output, "report.pdf") {
		t.Error("output missing report.pdf attachment")
	}
	if !strings.Contains(output, "summary.xlsx") {
		t.Error("output missing summary.xlsx attachment")
	}
	if !strings.Contains(output, "MB") {
		t.Error("output should contain MB size for large attachment")
	}
	if !strings.Contains(output, "KB") {
		t.Error("output should contain KB size for medium attachment")
	}
}

func TestSend_HTMLBodyFallback(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewWithWriter(&buf)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"recipient@example.com"},
		Subject:  "HTML Only",
		HtmlBody: "<p>HTML content</p>",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "<p>HTML content</p>") {
		t.Error("output should display HTML body when text body is empty")
	}
}

func TestSend_MultipleRecipients(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewWithWriter(&buf)

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"a@example.com", "b@example.com", "c@example.com"},
		Subject:  "To Many",
		TextBody: "Hello all",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "To: a@example.com, b@example.com, c@example.com") {
		t.Error("output should list all recipients comma-separated")
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	p := New()
	if p.Name() != "stdout" {
		t.Errorf("Name: got %q, want %q", p.Name(), "stdout")
	}
}

func TestFormatSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int
		want  string
	}{
		{name: "zero bytes", bytes: 0, want: "0 B"},
		{name: "small bytes", bytes: 512, want: "512 B"},
		{name: "kilobytes", bytes: 46080, want: "45.0 KB"},
		{name: "megabytes", bytes: 1258291, want: "1.2 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d): got %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
