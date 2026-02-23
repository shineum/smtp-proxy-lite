package smtp

import (
	"encoding/base64"
	"testing"
)

func TestAuthenticator_Enabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		password string
		want     bool
	}{
		{name: "both set", username: "user", password: "pass", want: true},
		{name: "empty username", username: "", password: "pass", want: false},
		{name: "empty password", username: "user", password: "", want: false},
		{name: "both empty", username: "", password: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			auth := NewAuthenticator(tt.username, tt.password)
			if got := auth.Enabled(); got != tt.want {
				t.Errorf("Enabled(): got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthenticator_VerifyPlain_Success(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	// AUTH PLAIN format: \0username\0password
	plaintext := "\x00testuser\x00testpass"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))

	err := auth.VerifyPlain(encoded)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuthenticator_VerifyPlain_WithAuthzID(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	// AUTH PLAIN with authorization identity: authzid\0authcid\0password
	plaintext := "admin\x00testuser\x00testpass"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))

	err := auth.VerifyPlain(encoded)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuthenticator_VerifyPlain_WrongPassword(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	plaintext := "\x00testuser\x00wrongpass"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))

	err := auth.VerifyPlain(encoded)
	if err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestAuthenticator_VerifyPlain_WrongUsername(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	plaintext := "\x00wronguser\x00testpass"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))

	err := auth.VerifyPlain(encoded)
	if err == nil {
		t.Error("expected error for wrong username, got nil")
	}
}

func TestAuthenticator_VerifyPlain_InvalidBase64(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	err := auth.VerifyPlain("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}

func TestAuthenticator_VerifyPlain_InvalidFormat(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	// Only one null separator instead of two
	plaintext := "testuser\x00testpass"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))

	err := auth.VerifyPlain(encoded)
	if err == nil {
		t.Error("expected error for invalid AUTH PLAIN format, got nil")
	}
}

func TestAuthenticator_VerifyLogin_Success(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	encodedUser := base64.StdEncoding.EncodeToString([]byte("testuser"))
	encodedPass := base64.StdEncoding.EncodeToString([]byte("testpass"))

	err := auth.VerifyLogin(encodedUser, encodedPass)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuthenticator_VerifyLogin_WrongPassword(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	encodedUser := base64.StdEncoding.EncodeToString([]byte("testuser"))
	encodedPass := base64.StdEncoding.EncodeToString([]byte("wrongpass"))

	err := auth.VerifyLogin(encodedUser, encodedPass)
	if err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestAuthenticator_VerifyLogin_InvalidBase64User(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	err := auth.VerifyLogin("invalid!!!", base64.StdEncoding.EncodeToString([]byte("testpass")))
	if err == nil {
		t.Error("expected error for invalid base64 username, got nil")
	}
}

func TestAuthenticator_VerifyLogin_InvalidBase64Pass(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator("testuser", "testpass")

	err := auth.VerifyLogin(base64.StdEncoding.EncodeToString([]byte("testuser")), "invalid!!!")
	if err == nil {
		t.Error("expected error for invalid base64 password, got nil")
	}
}
