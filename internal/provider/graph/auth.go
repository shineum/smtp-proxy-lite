package graph

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenExpiryBuffer is the time before actual expiry when we consider a token expired.
// This prevents using a token that is about to expire during a request.
const tokenExpiryBuffer = 5 * time.Minute

// tokenCache manages OAuth2 access tokens with thread-safe caching and
// automatic refresh before expiration.
type tokenCache struct {
	mu           sync.Mutex
	accessToken  string
	expiresAt    time.Time
	tokenURL     string
	clientID     string
	clientSecret string
	scope        string
	httpClient   *http.Client
}

// newTokenCache creates a new token cache for the given OAuth2 client credentials.
func newTokenCache(tokenURL, clientID, clientSecret string, httpClient *http.Client) *tokenCache {
	return &tokenCache{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		scope:        "https://graph.microsoft.com/.default",
		httpClient:   httpClient,
	}
}

// Token returns a valid access token, refreshing it if necessary.
// This method is safe for concurrent use.
func (tc *tokenCache) Token() (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.accessToken != "" && time.Now().Before(tc.expiresAt) {
		return tc.accessToken, nil
	}

	return tc.refresh()
}

// ForceRefresh discards the current token and acquires a new one.
// This is used when a 401 response indicates the token is invalid.
func (tc *tokenCache) ForceRefresh() (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.accessToken = ""
	tc.expiresAt = time.Time{}

	return tc.refresh()
}

// refresh acquires a new token from the OAuth2 token endpoint.
// The caller must hold tc.mu.
func (tc *tokenCache) refresh() (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {tc.clientID},
		"client_secret": {tc.clientSecret},
		"scope":         {tc.scope},
	}

	req, err := http.NewRequest(http.MethodPost, tc.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}

	tc.accessToken = tokenResp.AccessToken
	tc.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - tokenExpiryBuffer)

	return tc.accessToken, nil
}
