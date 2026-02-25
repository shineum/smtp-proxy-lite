package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear all relevant env vars for this test
	envVars := []string{
		"PROVIDER",
		"SMTP_LISTEN", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_MAX_MESSAGE_SIZE",
		"GRAPH_TENANT_ID", "GRAPH_CLIENT_ID", "GRAPH_CLIENT_SECRET", "GRAPH_SENDER",
		"SES_REGION", "SES_ACCESS_KEY_ID", "SES_SECRET_ACCESS_KEY", "SES_SENDER",
		"TLS_CERT_FILE", "TLS_KEY_FILE", "LOG_LEVEL",
	}
	for _, env := range envVars {
		t.Setenv(env, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SMTP.Listen != ":2525" {
		t.Errorf("SMTP.Listen: got %q, want %q", cfg.SMTP.Listen, ":2525")
	}
	if cfg.SMTP.Username != "" {
		t.Errorf("SMTP.Username: got %q, want empty", cfg.SMTP.Username)
	}
	if cfg.SMTP.Password != "" {
		t.Errorf("SMTP.Password: got %q, want empty", cfg.SMTP.Password)
	}
	if cfg.SMTP.MaxMessageSize != 26214400 {
		t.Errorf("SMTP.MaxMessageSize: got %d, want %d", cfg.SMTP.MaxMessageSize, 26214400)
	}
	if cfg.Graph.TenantID != "" {
		t.Errorf("Graph.TenantID: got %q, want empty", cfg.Graph.TenantID)
	}
	if cfg.Provider != "" {
		t.Errorf("Provider: got %q, want empty", cfg.Provider)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level: got %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.SES.Region != "" {
		t.Errorf("SES.Region: got %q, want empty", cfg.SES.Region)
	}
}

func TestLoad_EnvVarOverrides(t *testing.T) {
	t.Setenv("PROVIDER", "ses")
	t.Setenv("SMTP_LISTEN", ":9025")
	t.Setenv("SMTP_USERNAME", "admin")
	t.Setenv("SMTP_PASSWORD", "secret123")
	t.Setenv("SMTP_MAX_MESSAGE_SIZE", "10485760")
	t.Setenv("GRAPH_TENANT_ID", "tid-123")
	t.Setenv("GRAPH_CLIENT_ID", "cid-456")
	t.Setenv("GRAPH_CLIENT_SECRET", "csecret-789")
	t.Setenv("GRAPH_SENDER", "noreply@example.com")
	t.Setenv("SES_REGION", "us-east-1")
	t.Setenv("SES_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("SES_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("SES_SENDER", "ses@example.com")
	t.Setenv("TLS_CERT_FILE", "/certs/cert.pem")
	t.Setenv("TLS_KEY_FILE", "/certs/key.pem")
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Provider != "ses" {
		t.Errorf("Provider: got %q, want %q", cfg.Provider, "ses")
	}
	if cfg.SMTP.Listen != ":9025" {
		t.Errorf("SMTP.Listen: got %q, want %q", cfg.SMTP.Listen, ":9025")
	}
	if cfg.SMTP.Username != "admin" {
		t.Errorf("SMTP.Username: got %q, want %q", cfg.SMTP.Username, "admin")
	}
	if cfg.SMTP.Password != "secret123" {
		t.Errorf("SMTP.Password: got %q, want %q", cfg.SMTP.Password, "secret123")
	}
	if cfg.SMTP.MaxMessageSize != 10485760 {
		t.Errorf("SMTP.MaxMessageSize: got %d, want %d", cfg.SMTP.MaxMessageSize, 10485760)
	}
	if cfg.Graph.TenantID != "tid-123" {
		t.Errorf("Graph.TenantID: got %q, want %q", cfg.Graph.TenantID, "tid-123")
	}
	if cfg.Graph.ClientID != "cid-456" {
		t.Errorf("Graph.ClientID: got %q, want %q", cfg.Graph.ClientID, "cid-456")
	}
	if cfg.Graph.ClientSecret != "csecret-789" {
		t.Errorf("Graph.ClientSecret: got %q, want %q", cfg.Graph.ClientSecret, "csecret-789")
	}
	if cfg.Graph.Sender != "noreply@example.com" {
		t.Errorf("Graph.Sender: got %q, want %q", cfg.Graph.Sender, "noreply@example.com")
	}
	if cfg.SES.Region != "us-east-1" {
		t.Errorf("SES.Region: got %q, want %q", cfg.SES.Region, "us-east-1")
	}
	if cfg.SES.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("SES.AccessKeyID: got %q, want %q", cfg.SES.AccessKeyID, "AKIAIOSFODNN7EXAMPLE")
	}
	if cfg.SES.SecretAccessKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("SES.SecretAccessKey: got %q, want %q", cfg.SES.SecretAccessKey, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	}
	if cfg.SES.Sender != "ses@example.com" {
		t.Errorf("SES.Sender: got %q, want %q", cfg.SES.Sender, "ses@example.com")
	}
	if cfg.TLS.CertFile != "/certs/cert.pem" {
		t.Errorf("TLS.CertFile: got %q, want %q", cfg.TLS.CertFile, "/certs/cert.pem")
	}
	if cfg.TLS.KeyFile != "/certs/key.pem" {
		t.Errorf("TLS.KeyFile: got %q, want %q", cfg.TLS.KeyFile, "/certs/key.pem")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level: got %q, want %q", cfg.Logging.Level, "debug")
	}
}

func TestGraphConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		graph  GraphConfig
		expect bool
	}{
		{
			name:   "all set",
			graph:  GraphConfig{TenantID: "t", ClientID: "c", ClientSecret: "s", Sender: "sender@example.com"},
			expect: true,
		},
		{
			name:   "missing tenant_id",
			graph:  GraphConfig{ClientID: "c", ClientSecret: "s", Sender: "sender@example.com"},
			expect: false,
		},
		{
			name:   "missing client_id",
			graph:  GraphConfig{TenantID: "t", ClientSecret: "s", Sender: "sender@example.com"},
			expect: false,
		},
		{
			name:   "missing client_secret",
			graph:  GraphConfig{TenantID: "t", ClientID: "c", Sender: "sender@example.com"},
			expect: false,
		},
		{
			name:   "missing sender",
			graph:  GraphConfig{TenantID: "t", ClientID: "c", ClientSecret: "s"},
			expect: false,
		},
		{
			name:   "none set",
			graph:  GraphConfig{},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{Graph: tt.graph}
			if got := cfg.GraphConfigured(); got != tt.expect {
				t.Errorf("GraphConfigured(): got %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestAuthEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		password string
		expect   bool
	}{
		{name: "both set", username: "user", password: "pass", expect: true},
		{name: "username only", username: "user", password: "", expect: false},
		{name: "password only", username: "", password: "pass", expect: false},
		{name: "neither set", username: "", password: "", expect: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{SMTP: SMTPConfig{Username: tt.username, Password: tt.password}}
			if got := cfg.AuthEnabled(); got != tt.expect {
				t.Errorf("AuthEnabled(): got %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	yamlContent := `
smtp:
  listen: ":3025"
  username: "yamluser"
  password: "yamlpass"
  max_message_size: 5242880
graph:
  tenant_id: "yaml-tenant"
  client_id: "yaml-client"
  client_secret: "yaml-secret"
  sender: "yaml@example.com"
tls:
  cert_file: "/yaml/cert.pem"
  key_file: "/yaml/key.pem"
logging:
  level: "warn"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	// Clear env vars to ensure YAML values come through
	envVars := []string{
		"PROVIDER",
		"SMTP_LISTEN", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_MAX_MESSAGE_SIZE",
		"GRAPH_TENANT_ID", "GRAPH_CLIENT_ID", "GRAPH_CLIENT_SECRET", "GRAPH_SENDER",
		"SES_REGION", "SES_ACCESS_KEY_ID", "SES_SECRET_ACCESS_KEY", "SES_SENDER",
		"TLS_CERT_FILE", "TLS_KEY_FILE", "LOG_LEVEL",
	}
	for _, env := range envVars {
		t.Setenv(env, "")
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SMTP.Listen != ":3025" {
		t.Errorf("SMTP.Listen: got %q, want %q", cfg.SMTP.Listen, ":3025")
	}
	if cfg.SMTP.Username != "yamluser" {
		t.Errorf("SMTP.Username: got %q, want %q", cfg.SMTP.Username, "yamluser")
	}
	if cfg.SMTP.MaxMessageSize != 5242880 {
		t.Errorf("SMTP.MaxMessageSize: got %d, want %d", cfg.SMTP.MaxMessageSize, 5242880)
	}
	if cfg.Graph.TenantID != "yaml-tenant" {
		t.Errorf("Graph.TenantID: got %q, want %q", cfg.Graph.TenantID, "yaml-tenant")
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("Logging.Level: got %q, want %q", cfg.Logging.Level, "warn")
	}
}

func TestLoadFromFile_EnvOverridesYAML(t *testing.T) {
	yamlContent := `
smtp:
  listen: ":3025"
  username: "yamluser"
logging:
  level: "warn"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	t.Setenv("SMTP_LISTEN", ":9025")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("LOG_LEVEL", "error")

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Env var should override YAML
	if cfg.SMTP.Listen != ":9025" {
		t.Errorf("SMTP.Listen: got %q, want %q (env should override YAML)", cfg.SMTP.Listen, ":9025")
	}
	// Empty env var should NOT override YAML value
	if cfg.SMTP.Username != "yamluser" {
		t.Errorf("SMTP.Username: got %q, want %q (empty env should not override YAML)", cfg.SMTP.Username, "yamluser")
	}
	// Env var should override YAML
	if cfg.Logging.Level != "error" {
		t.Errorf("Logging.Level: got %q, want %q (env should override YAML)", cfg.Logging.Level, "error")
	}
}

func TestLoadFromFile_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadFromFile("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := LoadFromFile(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestSESConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ses    SESConfig
		expect bool
	}{
		{
			name:   "region and sender set",
			ses:    SESConfig{Region: "us-east-1", Sender: "ses@example.com"},
			expect: true,
		},
		{
			name:   "all fields set",
			ses:    SESConfig{Region: "us-east-1", AccessKeyID: "key", SecretAccessKey: "secret", Sender: "ses@example.com"},
			expect: true,
		},
		{
			name:   "missing region",
			ses:    SESConfig{Sender: "ses@example.com"},
			expect: false,
		},
		{
			name:   "missing sender",
			ses:    SESConfig{Region: "us-east-1"},
			expect: false,
		},
		{
			name:   "none set",
			ses:    SESConfig{},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{SES: tt.ses}
			if got := cfg.SESConfigured(); got != tt.expect {
				t.Errorf("SESConfigured(): got %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestProviderEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{name: "ses", envValue: "ses", want: "ses"},
		{name: "graph", envValue: "graph", want: "graph"},
		{name: "stdout", envValue: "stdout", want: "stdout"},
		{name: "uppercase SES", envValue: "SES", want: "ses"},
		{name: "empty", envValue: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PROVIDER", tt.envValue)
			// Clear other env vars
			for _, env := range []string{
				"SMTP_LISTEN", "SMTP_USERNAME", "SMTP_PASSWORD",
				"GRAPH_TENANT_ID", "GRAPH_CLIENT_ID", "GRAPH_CLIENT_SECRET", "GRAPH_SENDER",
				"SES_REGION", "SES_ACCESS_KEY_ID", "SES_SECRET_ACCESS_KEY", "SES_SENDER",
			} {
				t.Setenv(env, "")
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Provider != tt.want {
				t.Errorf("Provider: got %q, want %q", cfg.Provider, tt.want)
			}
		})
	}
}

func TestLoad_InvalidMaxMessageSize(t *testing.T) {
	t.Setenv("SMTP_MAX_MESSAGE_SIZE", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid value should be ignored, keeping the default
	if cfg.SMTP.MaxMessageSize != 26214400 {
		t.Errorf("SMTP.MaxMessageSize: got %d, want %d (should keep default for invalid input)", cfg.SMTP.MaxMessageSize, 26214400)
	}
}
