// Package stdout implements a Provider that prints emails to standard output.
package stdout

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sungwon/smtp-proxy-lite/internal/email"
)

// Provider prints email messages to stdout in a human-readable format.
type Provider struct {
	// writer is the output destination, defaulting to os.Stdout.
	writer io.Writer
}

// New creates a new stdout Provider that writes to os.Stdout.
func New() *Provider {
	return &Provider{writer: os.Stdout}
}

// NewWithWriter creates a new stdout Provider that writes to the given writer.
// This is useful for testing.
func NewWithWriter(w io.Writer) *Provider {
	return &Provider{writer: w}
}

// Send prints the email message to stdout in a readable format.
// It always returns nil (success).
func (p *Provider) Send(_ context.Context, msg *email.Email) error {
	var b strings.Builder

	b.WriteString("========================================\n")
	b.WriteString(fmt.Sprintf("From: %s\n", msg.From))
	b.WriteString(fmt.Sprintf("To: %s\n", strings.Join(msg.To, ", ")))

	if len(msg.Cc) > 0 {
		b.WriteString(fmt.Sprintf("Cc: %s\n", strings.Join(msg.Cc, ", ")))
	}

	b.WriteString(fmt.Sprintf("Subject: %s\n", msg.Subject))
	b.WriteString("Body:\n")

	body := msg.TextBody
	if body == "" {
		body = msg.HtmlBody
	}
	b.WriteString(body + "\n")

	if len(msg.Attachments) > 0 {
		attachments := make([]string, 0, len(msg.Attachments))
		for _, att := range msg.Attachments {
			attachments = append(attachments, fmt.Sprintf("%s (%s)", att.Filename, formatSize(len(att.Content))))
		}
		b.WriteString(fmt.Sprintf("Attachments: %s\n", strings.Join(attachments, ", ")))
	}

	b.WriteString("========================================\n")

	_, err := fmt.Fprint(p.writer, b.String())
	if err != nil {
		// Log the write error but still return nil since the provider
		// contract says stdout always succeeds conceptually
		return nil
	}

	return nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "stdout"
}

// formatSize formats a byte count into a human-readable string.
func formatSize(bytes int) string {
	const (
		kb = 1024
		mb = kb * 1024
	)

	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
