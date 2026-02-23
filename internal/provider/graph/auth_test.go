package graph

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenCache_AcquiresToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if r.FormValue("grant_type") != "client_credentials" {
			t.Errorf("grant_type: got %q, want %q", r.FormValue("grant_type"), "client_credentials")
		}
		if r.FormValue("client_id") != "test-client-id" {
			t.Errorf("client_id: got %q, want %q", r.FormValue("client_id"), "test-client-id")
		}
		if r.FormValue("client_secret") != "test-client-secret" {
			t.Errorf("client_secret: got %q, want %q", r.FormValue("client_secret"), "test-client-secret")
		}
		if r.FormValue("scope") != "https://graph.microsoft.com/.default" {
			t.Errorf("scope: got %q, want %q", r.FormValue("scope"), "https://graph.microsoft.com/.default")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "test-access-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	tc := newTokenCache(server.URL, "test-client-id", "test-client-secret", server.Client())

	token, err := tc.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-access-token" {
		t.Errorf("token: got %q, want %q", token, "test-access-token")
	}
}

func TestTokenCache_CachesToken(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "cached-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	tc := newTokenCache(server.URL, "cid", "csecret", server.Client())

	// First call should hit the server
	_, err := tc.Token()
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Second call should use cache
	token, err := tc.Token()
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if token != "cached-token" {
		t.Errorf("token: got %q, want %q", token, "cached-token")
	}

	if callCount.Load() != 1 {
		t.Errorf("server call count: got %d, want 1 (token should be cached)", callCount.Load())
	}
}

func TestTokenCache_RefreshesExpiredToken(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "token-" + string(rune('0'+count)),
			ExpiresIn:   1, // Expires almost immediately (minus 5 min buffer = already expired)
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	tc := newTokenCache(server.URL, "cid", "csecret", server.Client())

	// First call
	_, err := tc.Token()
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Token should be expired (1s - 5min buffer = negative), so next call refreshes
	_, err = tc.Token()
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	if callCount.Load() != 2 {
		t.Errorf("server call count: got %d, want 2 (expired token should trigger refresh)", callCount.Load())
	}
}

func TestTokenCache_ForceRefresh(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "force-token-" + string(rune('0'+count)),
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	tc := newTokenCache(server.URL, "cid", "csecret", server.Client())

	// First call
	_, err := tc.Token()
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Force refresh should bypass cache
	token, err := tc.ForceRefresh()
	if err != nil {
		t.Fatalf("force refresh error: %v", err)
	}

	if callCount.Load() != 2 {
		t.Errorf("server call count: got %d, want 2", callCount.Load())
	}

	// Verify we got a new token (not the cached one)
	if token == "" {
		t.Error("force refresh returned empty token")
	}
}

func TestTokenCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		// Simulate some latency
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "concurrent-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	tc := newTokenCache(server.URL, "cid", "csecret", server.Client())

	// Launch multiple goroutines requesting tokens concurrently
	var wg sync.WaitGroup
	const goroutines = 10
	tokens := make([]string, goroutines)
	errors := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tokens[idx], errors[idx] = tc.Token()
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}

	for i, token := range tokens {
		if token != "concurrent-token" {
			t.Errorf("goroutine %d token: got %q, want %q", i, token, "concurrent-token")
		}
	}
}

func TestTokenCache_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	tc := newTokenCache(server.URL, "cid", "csecret", server.Client())

	_, err := tc.Token()
	if err == nil {
		t.Error("expected error for server error response, got nil")
	}
}

func TestTokenCache_EmptyAccessToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "",
			ExpiresIn:   3600,
		})
	}))
	defer server.Close()

	tc := newTokenCache(server.URL, "cid", "csecret", server.Client())

	_, err := tc.Token()
	if err == nil {
		t.Error("expected error for empty access token, got nil")
	}
}
