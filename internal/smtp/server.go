package smtp

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/shineum/smtp-proxy-lite/internal/provider"
)

// shutdownTimeout is the maximum time to wait for in-flight connections
// during graceful shutdown.
const shutdownTimeout = 30 * time.Second

// ServerConfig holds the configuration for an SMTP server.
type ServerConfig struct {
	// ListenAddr is the address to listen on (e.g., ":2525").
	ListenAddr string

	// Hostname is the server hostname used in EHLO responses.
	Hostname string

	// Provider is the email delivery backend.
	Provider provider.Provider

	// TLSConfig is the TLS configuration for STARTTLS support.
	// If nil, STARTTLS is not advertised.
	TLSConfig *tls.Config

	// AuthUsername and AuthPassword configure SMTP AUTH.
	// If both are empty, authentication is not required.
	AuthUsername string
	AuthPassword string
}

// Server is an SMTP server that accepts connections and delegates
// email delivery to a configured Provider.
type Server struct {
	config   ServerConfig
	auth     *Authenticator
	listener net.Listener

	// wg tracks in-flight session goroutines for graceful shutdown.
	wg sync.WaitGroup
}

// New creates a new SMTP Server with the given configuration.
func New(cfg ServerConfig) *Server {
	if cfg.Hostname == "" {
		cfg.Hostname = "localhost"
	}

	return &Server{
		config: cfg,
		auth:   NewAuthenticator(cfg.AuthUsername, cfg.AuthPassword),
	}
}

// ListenAndServe starts the SMTP server and blocks until the context is cancelled.
// On context cancellation, it stops accepting new connections and waits up to
// 30 seconds for in-flight sessions to complete.
// @MX:WARN: [AUTO] Goroutine spawned per connection without explicit limit
// @MX:REASON: Each accepted TCP connection starts a goroutine for session handling
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return err
	}
	s.listener = ln

	slog.Info("SMTP server listening",
		"addr", ln.Addr().String(),
		"provider", s.config.Provider.Name(),
		"auth_enabled", s.auth.Enabled(),
		"tls_enabled", s.config.TLSConfig != nil,
	)

	// Monitor context for shutdown
	go func() {
		<-ctx.Done()
		slog.Info("shutting down SMTP server")
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				// Expected error from listener close during shutdown
				s.waitForSessions()
				return nil
			default:
				slog.Error("accept error", "error", err)
				continue
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			session := NewSession(
				conn,
				s.auth,
				s.config.Provider,
				s.config.Hostname,
				s.config.TLSConfig,
			)
			session.Handle(ctx)
		}()
	}
}

// waitForSessions waits for all in-flight sessions to complete,
// with a maximum timeout to prevent indefinite blocking.
func (s *Server) waitForSessions() {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("all sessions completed")
	case <-time.After(shutdownTimeout):
		slog.Warn("shutdown timeout reached, forcing close")
	}
}

// Addr returns the listener address, or empty string if not listening.
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}
