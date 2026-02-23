// Package smtp implements an SMTP server with TLS, authentication, and provider-based delivery.
package smtp

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Authenticator handles SMTP AUTH verification against configured credentials.
type Authenticator struct {
	username string
	password string
}

// NewAuthenticator creates an Authenticator with the given credentials.
// If both username and password are empty, authentication is disabled.
func NewAuthenticator(username, password string) *Authenticator {
	return &Authenticator{
		username: username,
		password: password,
	}
}

// Enabled returns true if authentication credentials are configured.
func (a *Authenticator) Enabled() bool {
	return a.username != "" && a.password != ""
}

// VerifyPlain decodes and verifies an AUTH PLAIN response.
// AUTH PLAIN format: base64(\0username\0password)
// Returns nil on success or an error describing the failure.
func (a *Authenticator) VerifyPlain(encoded string) error {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("invalid base64 encoding")
	}

	// AUTH PLAIN format: \0username\0password
	// or: authzid\0authcid\0password
	parts := strings.SplitN(string(decoded), "\x00", 3)
	if len(parts) != 3 {
		return fmt.Errorf("invalid AUTH PLAIN format")
	}

	// parts[0] is authorization identity (ignored)
	// parts[1] is authentication identity (username)
	// parts[2] is password
	user := parts[1]
	pass := parts[2]

	if user != a.username || pass != a.password {
		return fmt.Errorf("authentication failed")
	}

	return nil
}

// VerifyLogin verifies AUTH LOGIN credentials after the challenge-response flow.
// Both username and password should be base64-encoded.
func (a *Authenticator) VerifyLogin(encodedUser, encodedPass string) error {
	user, err := base64.StdEncoding.DecodeString(encodedUser)
	if err != nil {
		return fmt.Errorf("invalid base64 username")
	}

	pass, err := base64.StdEncoding.DecodeString(encodedPass)
	if err != nil {
		return fmt.Errorf("invalid base64 password")
	}

	if string(user) != a.username || string(pass) != a.password {
		return fmt.Errorf("authentication failed")
	}

	return nil
}
