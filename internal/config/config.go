// Package config provides environment-variable-first configuration loading
// with optional YAML file fallback for the SMTP proxy.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultMaxMessageSize is 25 MB in bytes.
const defaultMaxMessageSize = 26214400

// Config holds the complete application configuration.
type Config struct {
	SMTP    SMTPConfig    `yaml:"smtp"`
	Graph   GraphConfig   `yaml:"graph"`
	TLS     TLSConfig     `yaml:"tls"`
	Logging LoggingConfig `yaml:"logging"`
}

// SMTPConfig holds SMTP server configuration.
type SMTPConfig struct {
	Listen         string `yaml:"listen"`
	Username       string `yaml:"username"`
	Password       string `yaml:"password"`
	MaxMessageSize int64  `yaml:"max_message_size"`
}

// GraphConfig holds Microsoft Graph API configuration.
type GraphConfig struct {
	TenantID     string `yaml:"tenant_id"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Sender       string `yaml:"sender"`
}

// TLSConfig holds TLS certificate file paths.
type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// Load loads configuration from environment variables with sensible defaults.
// Environment variables always take precedence.
func Load() (*Config, error) {
	cfg := &Config{}
	cfg.applyDefaults()
	cfg.applyEnvVars()
	return cfg, nil
}

// LoadFromFile loads configuration from a YAML file as the base layer,
// then overrides with environment variables. Returns an error if the
// specified file path does not exist.
func LoadFromFile(path string) (*Config, error) {
	cfg := &Config{}
	cfg.applyDefaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Environment variables always override YAML values
	cfg.applyEnvVars()

	return cfg, nil
}

// GraphConfigured returns true if all four Graph API credentials are set.
func (c *Config) GraphConfigured() bool {
	return c.Graph.TenantID != "" &&
		c.Graph.ClientID != "" &&
		c.Graph.ClientSecret != "" &&
		c.Graph.Sender != ""
}

// AuthEnabled returns true if both SMTP username and password are set.
func (c *Config) AuthEnabled() bool {
	return c.SMTP.Username != "" && c.SMTP.Password != ""
}

// applyDefaults sets sensible default values for all configuration fields.
func (c *Config) applyDefaults() {
	c.SMTP.Listen = ":2525"
	c.SMTP.MaxMessageSize = defaultMaxMessageSize
	c.Logging.Level = "info"
}

// applyEnvVars overrides configuration with environment variable values.
// Only non-empty environment variables override existing values.
func (c *Config) applyEnvVars() {
	if v := os.Getenv("SMTP_LISTEN"); v != "" {
		c.SMTP.Listen = v
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		c.SMTP.Username = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		c.SMTP.Password = v
	}
	if v := os.Getenv("SMTP_MAX_MESSAGE_SIZE"); v != "" {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.SMTP.MaxMessageSize = size
		}
	}

	if v := os.Getenv("GRAPH_TENANT_ID"); v != "" {
		c.Graph.TenantID = v
	}
	if v := os.Getenv("GRAPH_CLIENT_ID"); v != "" {
		c.Graph.ClientID = v
	}
	if v := os.Getenv("GRAPH_CLIENT_SECRET"); v != "" {
		c.Graph.ClientSecret = v
	}
	if v := os.Getenv("GRAPH_SENDER"); v != "" {
		c.Graph.Sender = v
	}

	if v := os.Getenv("TLS_CERT_FILE"); v != "" {
		c.TLS.CertFile = v
	}
	if v := os.Getenv("TLS_KEY_FILE"); v != "" {
		c.TLS.KeyFile = v
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Logging.Level = strings.ToLower(v)
	}
}
