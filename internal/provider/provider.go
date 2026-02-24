// Package provider defines the interface for email delivery backends.
package provider

import (
	"context"

	"github.com/shineum/smtp-proxy-lite/internal/email"
)

// Provider is the interface that email delivery backends must implement.
// Each provider handles the actual sending of parsed email messages
// to the target service (e.g., stdout, Gmail API, SendGrid, etc.).
type Provider interface {
	// Send delivers an email message through this provider.
	// It returns an error if the delivery fails.
	Send(ctx context.Context, msg *email.Email) error

	// Name returns the human-readable name of this provider.
	Name() string
}
