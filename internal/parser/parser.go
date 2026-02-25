// Package parser provides RFC 5322 email message parsing with MIME multipart support.
package parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"

	"github.com/shineum/smtp-proxy-lite/internal/email"
)

// Parse parses a raw RFC 5322 email message into an Email struct.
// It handles plain text messages, multipart messages with text/html bodies,
// and attachments. Unrecognized MIME parts are logged as warnings.
func Parse(raw []byte) (*email.Email, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	result := &email.Email{
		RawHeaders: make(map[string][]string),
	}

	// Copy all headers
	for key, values := range msg.Header {
		result.RawHeaders[key] = values
	}

	// Extract standard header fields
	result.From = msg.Header.Get("From")
	result.Subject = msg.Header.Get("Subject")
	result.MessageID = msg.Header.Get("Message-Id")
	result.To = parseAddressList(msg.Header.Get("To"))
	result.Cc = parseAddressList(msg.Header.Get("Cc"))
	result.Bcc = parseAddressList(msg.Header.Get("Bcc"))

	contentType := msg.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// If content type is unparseable, treat as plain text
		slog.Warn("failed to parse content type, treating as plain text",
			"content_type", contentType,
			"error", err,
		)
		body, readErr := io.ReadAll(msg.Body)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read message body: %w", readErr)
		}
		result.TextBody = string(body)
		return result, nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return nil, fmt.Errorf("multipart message missing boundary")
		}
		if err := parseMultipart(msg.Body, boundary, result); err != nil {
			return nil, fmt.Errorf("failed to parse multipart message: %w", err)
		}
	} else {
		body, err := io.ReadAll(msg.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read message body: %w", err)
		}
		switch mediaType {
		case "text/plain":
			result.TextBody = string(body)
		case "text/html":
			result.HtmlBody = string(body)
		default:
			slog.Warn("unrecognized top-level content type",
				"content_type", mediaType,
			)
			result.TextBody = string(body)
		}
	}

	return result, nil
}

// parseMultipart processes a multipart MIME message body, extracting text/plain,
// text/html parts and attachments.
func parseMultipart(body io.Reader, boundary string, result *email.Email) error {
	reader := multipart.NewReader(body, boundary)

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read next part: %w", err)
		}

		partContentType := part.Header.Get("Content-Type")
		if partContentType == "" {
			partContentType = "text/plain"
		}

		mediaType, params, err := mime.ParseMediaType(partContentType)
		if err != nil {
			slog.Warn("failed to parse part content type, skipping",
				"content_type", partContentType,
				"error", err,
			)
			continue
		}

		contentDisposition := part.Header.Get("Content-Disposition")
		isAttachment := strings.HasPrefix(contentDisposition, "attachment")

		// Check for nested multipart
		if strings.HasPrefix(mediaType, "multipart/") {
			nestedBoundary := params["boundary"]
			if nestedBoundary == "" {
				slog.Warn("nested multipart missing boundary, skipping")
				continue
			}
			if err := parseMultipart(part, nestedBoundary, result); err != nil {
				slog.Warn("failed to parse nested multipart",
					"error", err,
				)
			}
			continue
		}

		content, err := readPartContent(part)
		if err != nil {
			slog.Warn("failed to read part content",
				"content_type", mediaType,
				"error", err,
			)
			continue
		}

		if isAttachment {
			filename := extractFilename(part, params)
			result.Attachments = append(result.Attachments, email.Attachment{
				Filename:    filename,
				ContentType: mediaType,
				Content:     content,
			})
			continue
		}

		switch mediaType {
		case "text/plain":
			if result.TextBody == "" {
				result.TextBody = string(content)
			}
		case "text/html":
			if result.HtmlBody == "" {
				result.HtmlBody = string(content)
			}
		default:
			// Check if it has a filename even without attachment disposition
			filename := extractFilename(part, params)
			if filename != "" {
				result.Attachments = append(result.Attachments, email.Attachment{
					Filename:    filename,
					ContentType: mediaType,
					Content:     content,
				})
			} else {
				slog.Warn("unrecognized MIME part, skipping",
					"content_type", mediaType,
					"disposition", contentDisposition,
				)
			}
		}
	}

	return nil
}

// readPartContent reads the full content of a MIME part, handling
// Content-Transfer-Encoding (base64, quoted-printable).
func readPartContent(part *multipart.Part) ([]byte, error) {
	encoding := part.Header.Get("Content-Transfer-Encoding")
	encoding = strings.ToLower(strings.TrimSpace(encoding))

	raw, err := io.ReadAll(part)
	if err != nil {
		return nil, err
	}

	switch encoding {
	case "base64":
		cleaned := strings.NewReplacer("\r", "", "\n", "").Replace(string(raw))
		decoded, err := base64.StdEncoding.DecodeString(cleaned)
		if err != nil {
			// Try with RawStdEncoding for unpadded base64
			decoded, err = base64.RawStdEncoding.DecodeString(cleaned)
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64 content: %w", err)
			}
		}
		return decoded, nil
	default:
		// For "7bit", "8bit", "binary", "quoted-printable", or empty,
		// return raw content. Go's multipart reader handles QP internally.
		return raw, nil
	}
}

// extractFilename extracts the filename from a MIME part, checking both
// Content-Disposition and Content-Type parameters.
func extractFilename(part *multipart.Part, params map[string]string) string {
	// Try Content-Disposition filename first (via multipart.Part)
	if fn := part.FileName(); fn != "" {
		return fn
	}
	// Fall back to Content-Type "name" parameter
	if name, ok := params["name"]; ok && name != "" {
		return name
	}
	// Generate fallback name from media type to satisfy Graph API's required "name" property
	if mediaType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type")); err == nil {
		parts := strings.SplitN(mediaType, "/", 2)
		if len(parts) == 2 {
			return "attachment." + parts[1]
		}
	}
	return "attachment"
}

// parseAddressList splits a comma-separated address list into individual addresses.
func parseAddressList(raw string) []string {
	if raw == "" {
		return nil
	}

	addresses, err := mail.ParseAddressList(raw)
	if err != nil {
		// Fall back to simple comma split if RFC 5322 parsing fails
		parts := strings.Split(raw, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}

	result := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		result = append(result, addr.Address)
	}
	return result
}
