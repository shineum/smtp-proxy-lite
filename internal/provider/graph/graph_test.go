package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shineum/smtp-proxy-lite/internal/email"
)

func TestBuildSendMailRequest_BasicEmail(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		From:     "sender@example.com",
		To:       []string{"alice@example.com", "bob@example.com"},
		Subject:  "Test Subject",
		TextBody: "Hello, World!",
	}

	req := buildSendMailRequest(msg)

	if req.Message.Subject != "Test Subject" {
		t.Errorf("Subject: got %q, want %q", req.Message.Subject, "Test Subject")
	}
	if req.Message.Body.ContentType != "text" {
		t.Errorf("Body.ContentType: got %q, want %q", req.Message.Body.ContentType, "text")
	}
	if req.Message.Body.Content != "Hello, World!" {
		t.Errorf("Body.Content: got %q, want %q", req.Message.Body.Content, "Hello, World!")
	}
	if len(req.Message.ToRecipients) != 2 {
		t.Fatalf("ToRecipients count: got %d, want 2", len(req.Message.ToRecipients))
	}
	if req.Message.ToRecipients[0].EmailAddress.Address != "alice@example.com" {
		t.Errorf("ToRecipients[0]: got %q, want %q", req.Message.ToRecipients[0].EmailAddress.Address, "alice@example.com")
	}
	if req.Message.ToRecipients[1].EmailAddress.Address != "bob@example.com" {
		t.Errorf("ToRecipients[1]: got %q, want %q", req.Message.ToRecipients[1].EmailAddress.Address, "bob@example.com")
	}
	if len(req.Message.CcRecipients) != 0 {
		t.Errorf("CcRecipients: got %d, want 0", len(req.Message.CcRecipients))
	}
	if len(req.Message.Attachments) != 0 {
		t.Errorf("Attachments: got %d, want 0", len(req.Message.Attachments))
	}
}

func TestBuildSendMailRequest_HTMLBody(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "HTML Email",
		TextBody: "Plain text",
		HtmlBody: "<p>HTML content</p>",
	}

	req := buildSendMailRequest(msg)

	if req.Message.Body.ContentType != "html" {
		t.Errorf("Body.ContentType: got %q, want %q", req.Message.Body.ContentType, "html")
	}
	if req.Message.Body.Content != "<p>HTML content</p>" {
		t.Errorf("Body.Content: got %q, want %q", req.Message.Body.Content, "<p>HTML content</p>")
	}
}

func TestBuildSendMailRequest_WithAttachments(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "With Attachment",
		TextBody: "See attached",
		Attachments: []email.Attachment{
			{
				Filename:    "report.pdf",
				ContentType: "application/pdf",
				Content:     []byte("pdf-content"),
			},
		},
	}

	req := buildSendMailRequest(msg)

	if len(req.Message.Attachments) != 1 {
		t.Fatalf("Attachments count: got %d, want 1", len(req.Message.Attachments))
	}

	att := req.Message.Attachments[0]
	if att.ODataType != "#microsoft.graph.fileAttachment" {
		t.Errorf("ODataType: got %q, want %q", att.ODataType, "#microsoft.graph.fileAttachment")
	}
	if att.Name != "report.pdf" {
		t.Errorf("Name: got %q, want %q", att.Name, "report.pdf")
	}
	if att.ContentType != "application/pdf" {
		t.Errorf("ContentType: got %q, want %q", att.ContentType, "application/pdf")
	}
	if att.ContentBytes == "" {
		t.Error("ContentBytes should not be empty")
	}
}

func TestBuildSendMailRequest_WithCc(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		To:       []string{"alice@example.com"},
		Cc:       []string{"carol@example.com", "dave@example.com"},
		Subject:  "With CC",
		TextBody: "Hello",
	}

	req := buildSendMailRequest(msg)

	if len(req.Message.CcRecipients) != 2 {
		t.Fatalf("CcRecipients count: got %d, want 2", len(req.Message.CcRecipients))
	}
	if req.Message.CcRecipients[0].EmailAddress.Address != "carol@example.com" {
		t.Errorf("CcRecipients[0]: got %q, want %q", req.Message.CcRecipients[0].EmailAddress.Address, "carol@example.com")
	}
}

func TestBuildSendMailRequest_JSONMarshaling(t *testing.T) {
	t.Parallel()

	msg := &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "JSON Test",
		TextBody: "Body",
	}

	req := buildSendMailRequest(msg)
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("JSON marshal error: %v", err)
	}

	// Verify it round-trips through JSON
	var decoded sendMailRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if decoded.Message.Subject != "JSON Test" {
		t.Errorf("round-trip Subject: got %q, want %q", decoded.Message.Subject, "JSON Test")
	}
}

func TestGraphProvider_Name(t *testing.T) {
	t.Parallel()

	p := &GraphProvider{}
	if p.Name() != "msgraph" {
		t.Errorf("Name: got %q, want %q", p.Name(), "msgraph")
	}
}

func TestGraphProvider_SendSuccess(t *testing.T) {
	t.Parallel()

	// Token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "test-token",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	// Graph API server
	graphServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization header: got %q, want %q", r.Header.Get("Authorization"), "Bearer test-token")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type header: got %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}

		// Verify request body is valid JSON
		var body sendMailRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body.Message.Subject != "Test" {
			t.Errorf("Subject in body: got %q, want %q", body.Message.Subject, "Test")
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer graphServer.Close()

	p := newWithOverrides(
		GraphProviderConfig{
			TenantID:     "test-tenant",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Sender:       "sender@example.com",
		},
		graphServer.URL,
		tokenServer.URL,
		graphServer.Client(),
	)

	msg := &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "Test",
		TextBody: "Body",
	}

	err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGraphProvider_PermanentError(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()

	graphServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(graphErrorResponse{
			Error: graphError{Code: "BadRequest", Message: "Invalid recipient"},
		})
	}))
	defer graphServer.Close()

	p := newWithOverrides(
		GraphProviderConfig{Sender: "s@example.com", TenantID: "t", ClientID: "c", ClientSecret: "s"},
		graphServer.URL, tokenServer.URL, graphServer.Client(),
	)

	err := p.Send(context.Background(), &email.Email{
		To:       []string{"bad@example.com"},
		Subject:  "Test",
		TextBody: "Body",
	})

	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
}

func TestGraphProvider_ForbiddenError(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()

	graphServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(graphErrorResponse{
			Error: graphError{Code: "Forbidden", Message: "Insufficient permissions"},
		})
	}))
	defer graphServer.Close()

	p := newWithOverrides(
		GraphProviderConfig{Sender: "s@example.com", TenantID: "t", ClientID: "c", ClientSecret: "s"},
		graphServer.URL, tokenServer.URL, graphServer.Client(),
	)

	err := p.Send(context.Background(), &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "Test",
		TextBody: "Body",
	})

	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}

	sendErr, ok := err.(*sendError)
	if !ok {
		t.Fatalf("expected *sendError, got %T", err)
	}
	if !sendErr.permanent {
		t.Error("403 error should be classified as permanent")
	}
}

func TestGraphProvider_RetryOn5xx(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()

	var graphCallCount atomic.Int32

	graphServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := graphCallCount.Add(1)
		if count <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(graphErrorResponse{
				Error: graphError{Code: "ServiceUnavailable", Message: "Try again"},
			})
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer graphServer.Close()

	p := newWithOverrides(
		GraphProviderConfig{Sender: "s@example.com", TenantID: "t", ClientID: "c", ClientSecret: "s"},
		graphServer.URL, tokenServer.URL, graphServer.Client(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := p.Send(ctx, &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "Test",
		TextBody: "Body",
	})

	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}

	if graphCallCount.Load() != 3 {
		t.Errorf("graph call count: got %d, want 3 (2 failures + 1 success)", graphCallCount.Load())
	}
}

func TestGraphProvider_RetryOn401WithTokenRefresh(t *testing.T) {
	t.Parallel()

	var tokenCallCount atomic.Int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := tokenCallCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "token-" + string(rune('0'+count)),
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	var graphCallCount atomic.Int32

	graphServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := graphCallCount.Add(1)
		if count == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(graphErrorResponse{
				Error: graphError{Code: "Unauthorized", Message: "Token expired"},
			})
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer graphServer.Close()

	p := newWithOverrides(
		GraphProviderConfig{Sender: "s@example.com", TenantID: "t", ClientID: "c", ClientSecret: "s"},
		graphServer.URL, tokenServer.URL, graphServer.Client(),
	)

	err := p.Send(context.Background(), &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "Test",
		TextBody: "Body",
	})

	if err != nil {
		t.Fatalf("expected success after token refresh, got: %v", err)
	}

	if graphCallCount.Load() != 2 {
		t.Errorf("graph call count: got %d, want 2", graphCallCount.Load())
	}

	// Token should have been refreshed (initial + force refresh)
	if tokenCallCount.Load() < 2 {
		t.Errorf("token call count: got %d, want >= 2", tokenCallCount.Load())
	}
}

func TestGraphProvider_RateLimitWithRetryAfter(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()

	var graphCallCount atomic.Int32

	graphServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := graphCallCount.Add(1)
		if count == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(graphErrorResponse{
				Error: graphError{Code: "TooManyRequests", Message: "Rate limited"},
			})
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer graphServer.Close()

	p := newWithOverrides(
		GraphProviderConfig{Sender: "s@example.com", TenantID: "t", ClientID: "c", ClientSecret: "s"},
		graphServer.URL, tokenServer.URL, graphServer.Client(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := p.Send(ctx, &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "Test",
		TextBody: "Body",
	})

	if err != nil {
		t.Fatalf("expected success after rate limit retry, got: %v", err)
	}

	if graphCallCount.Load() != 2 {
		t.Errorf("graph call count: got %d, want 2", graphCallCount.Load())
	}
}

func TestGraphProvider_ContextCancellation(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()

	graphServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(graphErrorResponse{
			Error: graphError{Code: "ServiceUnavailable", Message: "Down"},
		})
	}))
	defer graphServer.Close()

	p := newWithOverrides(
		GraphProviderConfig{Sender: "s@example.com", TenantID: "t", ClientID: "c", ClientSecret: "s"},
		graphServer.URL, tokenServer.URL, graphServer.Client(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately to test context cancellation during retry
	cancel()

	err := p.Send(ctx, &email.Email{
		To:       []string{"user@example.com"},
		Subject:  "Test",
		TextBody: "Body",
	})

	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		permanent  bool
		transient  bool
	}{
		{name: "400 Bad Request", statusCode: 400, permanent: true, transient: false},
		{name: "401 Unauthorized", statusCode: 401, permanent: false, transient: true},
		{name: "403 Forbidden", statusCode: 403, permanent: true, transient: false},
		{name: "429 Too Many Requests", statusCode: 429, permanent: false, transient: true},
		{name: "500 Internal Server Error", statusCode: 500, permanent: false, transient: true},
		{name: "502 Bad Gateway", statusCode: 502, permanent: false, transient: true},
		{name: "503 Service Unavailable", statusCode: 503, permanent: false, transient: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := classifyError(tt.statusCode, "test message", "")
			if err.permanent != tt.permanent {
				t.Errorf("permanent: got %v, want %v", err.permanent, tt.permanent)
			}
			if err.transient != tt.transient {
				t.Errorf("transient: got %v, want %v", err.transient, tt.transient)
			}
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 1 * time.Second},
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
	}

	for _, tt := range tests {
		got := backoffDelay(tt.attempt)
		if got != tt.want {
			t.Errorf("backoffDelay(%d): got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestSendError_Error(t *testing.T) {
	t.Parallel()

	err := &sendError{
		message:    "test error",
		statusCode: 500,
	}

	expected := "Graph API error (HTTP 500): test error"
	if err.Error() != expected {
		t.Errorf("Error(): got %q, want %q", err.Error(), expected)
	}
}
