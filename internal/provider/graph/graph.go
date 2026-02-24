package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/shineum/smtp-proxy-lite/internal/email"
)

// GraphProviderConfig holds the configuration for creating a GraphProvider.
type GraphProviderConfig struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	Sender       string
}

// maxRetries is the maximum number of retry attempts for transient failures.
const maxRetries = 3

// baseRetryDelay is the initial delay for exponential backoff.
const baseRetryDelay = 1 * time.Second

// GraphProvider sends emails via the Microsoft Graph API using OAuth2
// client credentials authentication.
// @MX:ANCHOR: [AUTO] External system integration point for Microsoft Graph API
// @MX:REASON: All email delivery flows through this provider when Graph is configured
type GraphProvider struct {
	sender     string
	graphURL   string
	httpClient *http.Client
	token      *tokenCache
}

// New creates a new GraphProvider with the given configuration.
func New(cfg GraphProviderConfig) *GraphProvider {
	tokenURL := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/token",
		cfg.TenantID,
	)

	client := &http.Client{Timeout: 30 * time.Second}

	return &GraphProvider{
		sender:     cfg.Sender,
		graphURL:   fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", cfg.Sender),
		httpClient: client,
		token:      newTokenCache(tokenURL, cfg.ClientID, cfg.ClientSecret, client),
	}
}

// newWithOverrides creates a GraphProvider with custom URLs and HTTP client,
// used for testing.
func newWithOverrides(cfg GraphProviderConfig, graphURL, tokenURL string, client *http.Client) *GraphProvider {
	return &GraphProvider{
		sender:     cfg.Sender,
		graphURL:   graphURL,
		httpClient: client,
		token:      newTokenCache(tokenURL, cfg.ClientID, cfg.ClientSecret, client),
	}
}

// Send delivers an email message via the Microsoft Graph API.
// It includes retry logic with exponential backoff for transient failures,
// Retry-After header respect for HTTP 429, and automatic token refresh for HTTP 401.
func (g *GraphProvider) Send(ctx context.Context, msg *email.Email) error {
	reqBody := buildSendMailRequest(msg)
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	var lastErr error
	tokenRefreshed := false

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			slog.Debug("retrying Graph API request",
				"attempt", attempt,
				"max_retries", maxRetries,
			)
		}

		err := g.doSendRequest(ctx, bodyJSON)
		if err == nil {
			return nil
		}

		lastErr = err

		graphErr, ok := err.(*sendError)
		if !ok {
			return err
		}

		switch {
		case graphErr.permanent:
			return graphErr
		case graphErr.statusCode == http.StatusUnauthorized && !tokenRefreshed:
			// Refresh token once and retry immediately
			slog.Info("refreshing Graph API token after 401")
			if _, refreshErr := g.token.ForceRefresh(); refreshErr != nil {
				return fmt.Errorf("token refresh failed: %w", refreshErr)
			}
			tokenRefreshed = true
			continue
		case graphErr.statusCode == http.StatusTooManyRequests:
			delay := g.retryAfterDelay(graphErr.retryAfter, attempt)
			slog.Info("rate limited by Graph API",
				"retry_after", delay,
			)
			if err := sleepWithContext(ctx, delay); err != nil {
				return fmt.Errorf("context cancelled during retry wait: %w", err)
			}
			continue
		case graphErr.transient:
			delay := backoffDelay(attempt)
			slog.Info("transient Graph API error, retrying",
				"status", graphErr.statusCode,
				"delay", delay,
			)
			if err := sleepWithContext(ctx, delay); err != nil {
				return fmt.Errorf("context cancelled during retry wait: %w", err)
			}
			continue
		default:
			return graphErr
		}
	}

	return fmt.Errorf("Graph API request failed after %d retries: %w", maxRetries, lastErr)
}

// Name returns the provider name.
func (g *GraphProvider) Name() string {
	return "msgraph"
}

// doSendRequest performs a single HTTP request to the Graph API sendMail endpoint.
func (g *GraphProvider) doSendRequest(ctx context.Context, bodyJSON []byte) error {
	token, err := g.token.Token()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.graphURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return &sendError{
			message:   fmt.Sprintf("HTTP request failed: %v", err),
			transient: true,
		}
	}
	defer resp.Body.Close()

	// HTTP 202 Accepted is success for sendMail
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)

	var graphErrResp graphErrorResponse
	if jsonErr := json.Unmarshal(body, &graphErrResp); jsonErr == nil && graphErrResp.Error.Message != "" {
		return classifyError(resp.StatusCode, graphErrResp.Error.Message, resp.Header.Get("Retry-After"))
	}

	return classifyError(resp.StatusCode, string(body), resp.Header.Get("Retry-After"))
}

// sendError represents an error from the Graph API send operation with
// classification for retry logic.
type sendError struct {
	message    string
	statusCode int
	permanent  bool
	transient  bool
	retryAfter string
}

func (e *sendError) Error() string {
	return fmt.Sprintf("Graph API error (HTTP %d): %s", e.statusCode, e.message)
}

// classifyError categorizes an HTTP error response for retry decisions.
func classifyError(statusCode int, message, retryAfter string) *sendError {
	err := &sendError{
		message:    message,
		statusCode: statusCode,
		retryAfter: retryAfter,
	}

	switch {
	case statusCode == http.StatusBadRequest || statusCode == http.StatusForbidden:
		err.permanent = true
	case statusCode == http.StatusUnauthorized:
		err.transient = true
	case statusCode == http.StatusTooManyRequests:
		err.transient = true
	case statusCode >= 500:
		err.transient = true
	default:
		err.permanent = true
	}

	return err
}

// retryAfterDelay parses the Retry-After header value and returns the appropriate delay.
// Falls back to exponential backoff if the header is missing or unparseable.
func (g *GraphProvider) retryAfterDelay(retryAfter string, attempt int) time.Duration {
	if retryAfter == "" {
		return backoffDelay(attempt)
	}

	seconds, err := strconv.Atoi(retryAfter)
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	return backoffDelay(attempt)
}

// backoffDelay returns the exponential backoff delay for the given attempt number.
// Delays are: 1s, 2s, 4s
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
