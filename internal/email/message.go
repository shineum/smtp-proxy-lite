// Package email defines the core email data model used throughout the SMTP proxy.
package email

// Email represents a parsed email message with all its components.
type Email struct {
	From        string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	TextBody    string
	HtmlBody    string
	Attachments []Attachment
	RawHeaders  map[string][]string
	MessageID   string
}

// Attachment represents a file attached to an email message.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}
