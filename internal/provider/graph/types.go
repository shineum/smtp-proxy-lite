// Package graph implements a Provider that sends emails via the Microsoft Graph API.
package graph

import (
	"encoding/base64"

	"github.com/sungwon/smtp-proxy-lite/internal/email"
)

// sendMailRequest is the top-level request body for the Graph API sendMail endpoint.
type sendMailRequest struct {
	Message sendMailMessage `json:"message"`
}

// sendMailMessage represents the message portion of a sendMail request.
type sendMailMessage struct {
	Subject      string            `json:"subject"`
	Body         messageBody       `json:"body"`
	ToRecipients []recipient       `json:"toRecipients"`
	CcRecipients []recipient       `json:"ccRecipients,omitempty"`
	Attachments  []graphAttachment `json:"attachments,omitempty"`
}

// messageBody represents the body of an email message.
type messageBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// recipient represents an email recipient.
type recipient struct {
	EmailAddress emailAddress `json:"emailAddress"`
}

// emailAddress represents an email address in a Graph API request.
type emailAddress struct {
	Address string `json:"address"`
}

// graphAttachment represents a file attachment in a Graph API request.
type graphAttachment struct {
	ODataType    string `json:"@odata.type"`
	Name         string `json:"name"`
	ContentType  string `json:"contentType"`
	ContentBytes string `json:"contentBytes"`
}

// tokenResponse represents the OAuth2 token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// graphErrorResponse represents an error response from the Graph API.
type graphErrorResponse struct {
	Error graphError `json:"error"`
}

// graphError represents the error detail in a Graph API error response.
type graphError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// buildSendMailRequest converts an email.Email into a Graph API sendMail request body.
func buildSendMailRequest(msg *email.Email) *sendMailRequest {
	// Determine body content type and content
	body := messageBody{
		ContentType: "text",
		Content:     msg.TextBody,
	}
	if msg.HtmlBody != "" {
		body.ContentType = "html"
		body.Content = msg.HtmlBody
	}

	// Build recipient lists
	toRecipients := make([]recipient, 0, len(msg.To))
	for _, addr := range msg.To {
		toRecipients = append(toRecipients, recipient{
			EmailAddress: emailAddress{Address: addr},
		})
	}

	ccRecipients := make([]recipient, 0, len(msg.Cc))
	for _, addr := range msg.Cc {
		ccRecipients = append(ccRecipients, recipient{
			EmailAddress: emailAddress{Address: addr},
		})
	}

	// Build attachments
	attachments := make([]graphAttachment, 0, len(msg.Attachments))
	for _, att := range msg.Attachments {
		attachments = append(attachments, graphAttachment{
			ODataType:    "#microsoft.graph.fileAttachment",
			Name:         att.Filename,
			ContentType:  att.ContentType,
			ContentBytes: base64.StdEncoding.EncodeToString(att.Content),
		})
	}

	return &sendMailRequest{
		Message: sendMailMessage{
			Subject:      msg.Subject,
			Body:         body,
			ToRecipients: toRecipients,
			CcRecipients: ccRecipients,
			Attachments:  attachments,
		},
	}
}
