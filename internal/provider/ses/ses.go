// Package ses implements a Provider that sends emails via AWS SES v2.
package ses

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	sesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/shineum/smtp-proxy-lite/internal/email"
)

// maxRetries is the maximum number of retry attempts for transient failures.
const maxRetries = 3

// baseRetryDelay is the initial delay for exponential backoff.
const baseRetryDelay = 1 * time.Second

// SESProviderConfig holds the configuration for creating a SESProvider.
type SESProviderConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Sender          string
}

// SESProvider sends emails via the AWS SES v2 API.
// @MX:ANCHOR: [AUTO] External system integration point for AWS SES
// @MX:REASON: All email delivery flows through this provider when SES is configured
type SESProvider struct {
	sender string
	client SendEmailAPI
}

// SendEmailAPI is the interface for the SES v2 SendEmail operation.
// Used for testing with mock implementations.
type SendEmailAPI interface {
	SendEmail(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

// New creates a new SESProvider with the given configuration.
func New(ctx context.Context, cfg SESProviderConfig) (*SESProvider, error) {
	var opts []func(*awsconfig.LoadOptions) error

	opts = append(opts, awsconfig.WithRegion(cfg.Region))

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := sesv2.NewFromConfig(awsCfg)

	return &SESProvider{
		sender: cfg.Sender,
		client: client,
	}, nil
}

// NewWithClient creates a SESProvider with a custom client, used for testing.
func NewWithClient(sender string, client SendEmailAPI) *SESProvider {
	return &SESProvider{
		sender: sender,
		client: client,
	}
}

// Send delivers an email message via AWS SES v2.
// For emails with attachments, it builds a raw MIME message.
// For simple emails, it uses the SES simple email format.
func (s *SESProvider) Send(ctx context.Context, msg *email.Email) error {
	var input *sesv2.SendEmailInput

	if len(msg.Attachments) > 0 {
		raw, err := buildRawMessage(s.sender, msg)
		if err != nil {
			return fmt.Errorf("failed to build raw message: %w", err)
		}
		input = &sesv2.SendEmailInput{
			Content: &types.EmailContent{
				Raw: &types.RawMessage{
					Data: raw,
				},
			},
		}
	} else {
		input = buildSimpleInput(s.sender, msg)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			slog.Debug("retrying SES API request",
				"attempt", attempt,
				"max_retries", maxRetries,
			)
			delay := backoffDelay(attempt)
			if err := sleepWithContext(ctx, delay); err != nil {
				return fmt.Errorf("context cancelled during retry wait: %w", err)
			}
		}

		_, err := s.client.SendEmail(ctx, input)
		if err == nil {
			return nil
		}

		lastErr = err
		slog.Warn("SES API error",
			"attempt", attempt,
			"error", err,
		)
	}

	return fmt.Errorf("SES API request failed after %d retries: %w", maxRetries, lastErr)
}

// Name returns the provider name.
func (s *SESProvider) Name() string {
	return "ses"
}

// buildSimpleInput creates a SES SendEmailInput for emails without attachments.
func buildSimpleInput(sender string, msg *email.Email) *sesv2.SendEmailInput {
	body := &types.Body{}

	if msg.HtmlBody != "" {
		body.Html = &types.Content{
			Data:    aws.String(msg.HtmlBody),
			Charset: aws.String("UTF-8"),
		}
	}
	if msg.TextBody != "" {
		body.Text = &types.Content{
			Data:    aws.String(msg.TextBody),
			Charset: aws.String("UTF-8"),
		}
	}

	dest := &types.Destination{
		ToAddresses:  msg.To,
		CcAddresses:  msg.Cc,
		BccAddresses: msg.Bcc,
	}

	return &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(sender),
		Destination:      dest,
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(msg.Subject),
					Charset: aws.String("UTF-8"),
				},
				Body: body,
			},
		},
	}
}

// buildRawMessage constructs a raw MIME message for emails with attachments.
func buildRawMessage(sender string, msg *email.Email) ([]byte, error) {
	var buf bytes.Buffer

	// Write headers
	fmt.Fprintf(&buf, "From: %s\r\n", sender)
	if len(msg.To) > 0 {
		fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(msg.To, ", "))
	}
	if len(msg.Cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(msg.Cc, ", "))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", msg.Subject)
	if msg.MessageID != "" {
		fmt.Fprintf(&buf, "Message-ID: %s\r\n", msg.MessageID)
	}
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")

	writer := multipart.NewWriter(&buf)
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", writer.Boundary())

	// Write body part
	bodyHeader := make(textproto.MIMEHeader)
	if msg.HtmlBody != "" {
		bodyHeader.Set("Content-Type", "text/html; charset=UTF-8")
		part, err := writer.CreatePart(bodyHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to create body part: %w", err)
		}
		part.Write([]byte(msg.HtmlBody))
	} else if msg.TextBody != "" {
		bodyHeader.Set("Content-Type", "text/plain; charset=UTF-8")
		part, err := writer.CreatePart(bodyHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to create body part: %w", err)
		}
		part.Write([]byte(msg.TextBody))
	}

	// Write attachments
	for _, att := range msg.Attachments {
		attHeader := make(textproto.MIMEHeader)
		attHeader.Set("Content-Type", att.ContentType)
		attHeader.Set("Content-Transfer-Encoding", "base64")
		attHeader.Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%s", mime.QEncoding.Encode("UTF-8", att.Filename)))

		part, err := writer.CreatePart(attHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to create attachment part: %w", err)
		}

		encoded := encodeBase64WithLineBreaks(att.Content)
		part.Write([]byte(encoded))
	}

	writer.Close()
	return buf.Bytes(), nil
}

// encodeBase64WithLineBreaks encodes bytes to base64 with 76-character line breaks per RFC 2045.
func encodeBase64WithLineBreaks(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var lines []string
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		lines = append(lines, encoded[i:end])
	}
	return strings.Join(lines, "\r\n")
}

// backoffDelay returns the exponential backoff delay for the given attempt number.
func backoffDelay(attempt int) time.Duration {
	delay := baseRetryDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	return delay
}

// sleepWithContext waits for the specified duration or until the context is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
