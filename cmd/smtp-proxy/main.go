// Package main is the entry point for the SMTP proxy server.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shineum/smtp-proxy-lite/internal/config"
	"github.com/shineum/smtp-proxy-lite/internal/provider"
	"github.com/shineum/smtp-proxy-lite/internal/provider/graph"
	"github.com/shineum/smtp-proxy-lite/internal/provider/ses"
	"github.com/shineum/smtp-proxy-lite/internal/provider/stdout"
	"github.com/shineum/smtp-proxy-lite/internal/smtp"
	smtptls "github.com/shineum/smtp-proxy-lite/internal/tls"
)

func main() {
	configPath := flag.String("config", "", "path to YAML configuration file (optional)")
	flag.Parse()

	// Load configuration
	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Setup structured logging
	setupLogger(cfg.Logging.Level)

	// Load or generate TLS certificates
	tlsConfig, err := smtptls.LoadOrGenerateTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	if err != nil {
		slog.Error("failed to setup TLS", "error", err)
		os.Exit(1)
	}

	tlsMode := "self-signed"
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		tlsMode = "file"
	}

	// Select email delivery provider
	prov := selectProvider(cfg)

	// Create SMTP server
	server := smtp.New(smtp.ServerConfig{
		ListenAddr:   cfg.SMTP.Listen,
		Hostname:     "localhost",
		Provider:     prov,
		TLSConfig:    tlsConfig,
		AuthUsername: cfg.SMTP.Username,
		AuthPassword: cfg.SMTP.Password,
	})

	slog.Info("starting smtp-proxy-lite",
		"listen", cfg.SMTP.Listen,
		"provider", prov.Name(),
		"auth_enabled", cfg.AuthEnabled(),
		"tls_mode", tlsMode,
	)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		slog.Info("received signal, initiating shutdown", "signal", sig)
		cancel()
	}()

	// Start the server (blocks until context is cancelled)
	if err := server.ListenAndServe(ctx); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	slog.Info("smtp-proxy-lite stopped")
}

// loadConfig loads configuration from the specified path (YAML + env override)
// or from environment variables only if no path is given.
func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.LoadFromFile(path)
	}
	return config.Load()
}

// setupLogger configures the global slog logger with JSON output and the
// specified log level.
func setupLogger(level string) {
	var logLevel slog.Level

	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}

// selectProvider chooses the email delivery backend based on configuration.
// If the PROVIDER env var is set, it takes precedence.
// Otherwise, it falls back to auto-detection (Graph if configured, else stdout).
func selectProvider(cfg *config.Config) provider.Provider {
	switch cfg.Provider {
	case "ses":
		if !cfg.SESConfigured() {
			slog.Error("SES provider selected but SES_REGION and SES_SENDER are required")
			os.Exit(1)
		}
		slog.Info("using AWS SES provider",
			"region", cfg.SES.Region,
			"sender", cfg.SES.Sender,
		)
		p, err := ses.New(context.Background(), ses.SESProviderConfig{
			Region:          cfg.SES.Region,
			AccessKeyID:     cfg.SES.AccessKeyID,
			SecretAccessKey: cfg.SES.SecretAccessKey,
			Sender:          cfg.SES.Sender,
		})
		if err != nil {
			slog.Error("failed to create SES provider", "error", err)
			os.Exit(1)
		}
		return p

	case "graph":
		if !cfg.GraphConfigured() {
			slog.Error("Graph provider selected but GRAPH_TENANT_ID, GRAPH_CLIENT_ID, GRAPH_CLIENT_SECRET, and GRAPH_SENDER are required")
			os.Exit(1)
		}
		slog.Info("using Microsoft Graph provider",
			"sender", cfg.Graph.Sender,
		)
		return graph.New(graph.GraphProviderConfig{
			TenantID:     cfg.Graph.TenantID,
			ClientID:     cfg.Graph.ClientID,
			ClientSecret: cfg.Graph.ClientSecret,
			Sender:       cfg.Graph.Sender,
		})

	case "stdout":
		slog.Info("using stdout provider")
		return stdout.New()

	case "":
		// Auto-detection fallback for backward compatibility
		if cfg.GraphConfigured() {
			slog.Info("using Microsoft Graph provider (auto-detected)",
				"sender", cfg.Graph.Sender,
			)
			return graph.New(graph.GraphProviderConfig{
				TenantID:     cfg.Graph.TenantID,
				ClientID:     cfg.Graph.ClientID,
				ClientSecret: cfg.Graph.ClientSecret,
				Sender:       cfg.Graph.Sender,
			})
		}
		if cfg.SESConfigured() {
			slog.Info("using AWS SES provider (auto-detected)",
				"region", cfg.SES.Region,
				"sender", cfg.SES.Sender,
			)
			p, err := ses.New(context.Background(), ses.SESProviderConfig{
				Region:          cfg.SES.Region,
				AccessKeyID:     cfg.SES.AccessKeyID,
				SecretAccessKey: cfg.SES.SecretAccessKey,
				Sender:          cfg.SES.Sender,
			})
			if err != nil {
				slog.Error("failed to create SES provider", "error", err)
				os.Exit(1)
			}
			return p
		}
		slog.Info("no provider configured, using stdout provider")
		return stdout.New()

	default:
		slog.Error("unknown provider", "provider", cfg.Provider)
		os.Exit(1)
		return nil
	}
}
