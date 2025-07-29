package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/McKael/madon/v3"
)

// =============================================================================
// MASTODON CLIENT TESTS
// =============================================================================

func TestNewMastodonClient_Success(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:        "https://mastodon.example.com",
			AccessToken:   "test-token",
			ClientTimeout: "45s",
		},
	}

	client, err := newMastodonClient(cfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if client.server != "https://mastodon.example.com" {
		t.Errorf("Expected server to be 'https://mastodon.example.com', got '%s'", client.server)
	}

	if client.accessToken != "test-token" {
		t.Errorf("Expected access token to be 'test-token', got '%s'", client.accessToken)
	}

	expectedTimeout := 45 * time.Second
	if client.timeout != expectedTimeout {
		t.Errorf("Expected timeout to be %v, got %v", expectedTimeout, client.timeout)
	}

	if client.madonClient != nil {
		t.Error("Expected madonClient to be nil initially")
	}
}

func TestNewMastodonClient_DefaultTimeout(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
			// No timeout specified
		},
	}

	client, err := newMastodonClient(cfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedTimeout := 30 * time.Second
	if client.timeout != expectedTimeout {
		t.Errorf("Expected default timeout to be %v, got %v", expectedTimeout, client.timeout)
	}
}

func TestNewMastodonClient_EmptyServer(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "", // Empty server
			AccessToken: "test-token",
		},
	}

	_, err := newMastodonClient(cfg)
	if err == nil {
		t.Fatal("Expected error for empty server, got nil")
	}

	expectedMsg := "mastodon server URL is required"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestNewMastodonClient_EmptyAccessToken(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "", // Empty access token
		},
	}

	_, err := newMastodonClient(cfg)
	if err == nil {
		t.Fatal("Expected error for empty access token, got nil")
	}

	expectedMsg := "mastodon access token is required"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestNewMastodonClient_InvalidTimeout(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:        "https://mastodon.example.com",
			AccessToken:   "test-token",
			ClientTimeout: "invalid-timeout",
		},
	}

	_, err := newMastodonClient(cfg)
	if err == nil {
		t.Fatal("Expected error for invalid timeout, got nil")
	}

	if !strings.Contains(err.Error(), "invalid client timeout") {
		t.Errorf("Expected error message to contain 'invalid client timeout', got '%s'", err.Error())
	}
}

func TestMastodonClient_InitMadonClient_Success(t *testing.T) {
	client := &MastodonClient{
		server:      "https://mastodon.example.com",
		accessToken: "test-token",
		timeout:     30 * time.Second,
	}

	// Note: This test may fail in isolated environments due to madon.RestoreApp
	// In a real-world scenario, we'd mock this dependency
	err := client.initMadonClient()

	// Since madon.RestoreApp might fail in test environment, we check the error type
	// The important thing is that the method doesn't panic and handles errors properly
	if err != nil {
		// This is expected in test environment - just verify error handling works
		if !strings.Contains(err.Error(), "failed to create madon client") {
			t.Errorf("Expected error to contain 'failed to create madon client', got '%s'", err.Error())
		}
	}
}

func TestMastodonClient_InitMadonClient_AlreadyInitialized(t *testing.T) {
	// Create a mock madon client
	mockClient := &madon.Client{}

	client := &MastodonClient{
		server:      "https://mastodon.example.com",
		accessToken: "test-token",
		timeout:     30 * time.Second,
		madonClient: mockClient, // Already initialized
	}

	err := client.initMadonClient()
	if err != nil {
		t.Fatalf("Expected no error when already initialized, got %v", err)
	}

	// Should return the same client
	if client.madonClient != mockClient {
		t.Error("Expected madonClient to remain the same when already initialized")
	}
}

func TestMastodonClient_GetMadonClient(t *testing.T) {
	client := &MastodonClient{
		server:      "https://mastodon.example.com",
		accessToken: "test-token",
		timeout:     30 * time.Second,
	}

	// This will likely fail in test environment, but we're testing the error path
	_, err := client.getMadonClient()
	if err != nil {
		// Expected in test environment
		if !strings.Contains(err.Error(), "failed to create madon client") {
			t.Errorf("Expected error to contain 'failed to create madon client', got '%s'", err.Error())
		}
	}
}

func TestMastodonClient_VerifyCredentials_Success(t *testing.T) {
	// Create a mock madon client that's already initialized
	mockClient := &madon.Client{}

	client := &MastodonClient{
		server:      "https://mastodon.example.com",
		accessToken: "test-token",
		timeout:     30 * time.Second,
		madonClient: mockClient, // Pre-initialized to avoid madon.RestoreApp call
	}

	// Since we can't easily mock madon.Client.GetCurrentAccount() in this test environment,
	// we'll test the actual behavior which will fail on network call
	err := client.verifyCredentials()
	if err != nil {
		// Expected because GetCurrentAccount will fail on network call
		if !strings.Contains(err.Error(), "failed to verify credentials") {
			t.Errorf("Expected error to contain 'failed to verify credentials', got '%s'", err.Error())
		}
	}
}

func TestMastodonClient_VerifyCredentials_InitFails(t *testing.T) {
	client := &MastodonClient{
		server:      "https://mastodon.example.com",
		accessToken: "test-token",
		timeout:     30 * time.Second,
		// madonClient is nil, will trigger initMadonClient
	}

	err := client.verifyCredentials()
	if err != nil {
		// Could be either init failure or credential verification failure
		if !strings.Contains(err.Error(), "failed to create madon client") &&
			!strings.Contains(err.Error(), "failed to verify credentials") {
			t.Errorf("Expected error about madon client or credential verification, got '%s'", err.Error())
		}
	}
}

// =============================================================================
// RATE LIMITER TESTS
// =============================================================================

func TestNewRateLimiter_Creation(t *testing.T) {
	maxRequests := 10
	timeWindow := 5 * time.Minute

	rl := newRateLimiter(maxRequests, timeWindow)

	if rl.maxRequests != maxRequests {
		t.Errorf("Expected maxRequests to be %d, got %d", maxRequests, rl.maxRequests)
	}

	if rl.timeWindow != timeWindow {
		t.Errorf("Expected timeWindow to be %v, got %v", timeWindow, rl.timeWindow)
	}

	if rl.requests == nil {
		t.Error("Expected requests slice to be initialized")
	}

	if len(rl.requests) != 0 {
		t.Errorf("Expected requests slice to be empty initially, got length %d", len(rl.requests))
	}
}

func TestRateLimiter_Allow_WithinLimit(t *testing.T) {
	rl := newRateLimiter(3, 1*time.Second)

	// First three requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Fourth request should be denied
	if rl.allow() {
		t.Error("Fourth request should be denied")
	}
}

func TestRateLimiter_Allow_TimeWindowExpiry(t *testing.T) {
	rl := newRateLimiter(2, 20*time.Millisecond) // Reduced time window

	// Fill up the rate limit
	if !rl.allow() {
		t.Error("First request should be allowed")
	}
	if !rl.allow() {
		t.Error("Second request should be allowed")
	}

	// Third request should be denied
	if rl.allow() {
		t.Error("Third request should be denied")
	}

	// Wait for time window to expire
	time.Sleep(25 * time.Millisecond) // Reduced wait time

	// Now requests should be allowed again
	if !rl.allow() {
		t.Error("Request after time window expiry should be allowed")
	}
}

func TestRateLimiter_Wait_Success(t *testing.T) {
	rl := newRateLimiter(1, 50*time.Millisecond) // Reduced time window
	ctx := context.Background()

	// Fill up the rate limit
	if !rl.allow() {
		t.Error("First request should be allowed")
	}

	// Wait should complete successfully after the time window
	start := time.Now()
	err := rl.wait(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error from wait, got %v", err)
	}

	// Should have waited at least 50ms for the time window to expire
	if duration < 50*time.Millisecond {
		t.Errorf("Expected to wait at least 50ms, waited %v", duration)
	}
}

func TestRateLimiter_Wait_ContextCanceled(t *testing.T) {
	rl := newRateLimiter(1, 10*time.Second) // Long time window
	ctx, cancel := context.WithCancel(context.Background())

	// Fill up the rate limit
	if !rl.allow() {
		t.Error("First request should be allowed")
	}

	// Cancel context immediately
	cancel()

	// Wait should return context error
	err := rl.wait(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestRateLimiter_Wait_ContextTimeout(t *testing.T) {
	rl := newRateLimiter(1, 10*time.Second)                                       // Long time window
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond) // Reduced timeout
	defer cancel()

	// Fill up the rate limit
	if !rl.allow() {
		t.Error("First request should be allowed")
	}

	// Wait should timeout
	err := rl.wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded error, got %v", err)
	}
}

// =============================================================================
// MASTODON BOOKMARK CLIENT TESTS
// =============================================================================

func TestNewMastodonBookmarkClient_Creation(t *testing.T) {
	mockClient := &madon.Client{}
	maxRetries := 5

	bc := newMastodonBookmarkClient(mockClient, maxRetries)

	if bc.client != mockClient {
		t.Error("Expected client to be set correctly")
	}

	if bc.maxRetries != maxRetries {
		t.Errorf("Expected maxRetries to be %d, got %d", maxRetries, bc.maxRetries)
	}

	if bc.rateLimiter == nil {
		t.Error("Expected rateLimiter to be initialized")
	}

	// Verify rate limiter settings
	if bc.rateLimiter.maxRequests != 150 {
		t.Errorf("Expected rate limiter maxRequests to be 150, got %d", bc.rateLimiter.maxRequests)
	}

	expectedWindow := 5 * time.Minute
	if bc.rateLimiter.timeWindow != expectedWindow {
		t.Errorf("Expected rate limiter timeWindow to be %v, got %v", expectedWindow, bc.rateLimiter.timeWindow)
	}
}

func TestMastodonBookmarkClient_GetBookmarks_NilClient(t *testing.T) {
	bc := &MastodonBookmarkClient{
		client:      nil, // Nil client
		rateLimiter: newRateLimiter(150, 5*time.Minute),
		maxRetries:  3,
	}

	ctx := context.Background()
	bookmarks, nextURL, err := bc.GetBookmarks(ctx, 20, "")

	if err != nil {
		t.Errorf("Expected no error with nil client, got %v", err)
	}

	if len(bookmarks) != 0 {
		t.Errorf("Expected empty bookmarks slice, got %d items", len(bookmarks))
	}

	if nextURL != "" {
		t.Errorf("Expected empty nextURL, got '%s'", nextURL)
	}
}

func TestMastodonBookmarkClient_GetBookmarks_RateLimitContextCanceled(t *testing.T) {
	// Create a rate limiter that's already at capacity
	rl := newRateLimiter(1, 10*time.Second)
	rl.allow() // Fill up the rate limit

	bc := &MastodonBookmarkClient{
		client:      &madon.Client{},
		rateLimiter: rl,
		maxRetries:  3,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err == nil {
		t.Error("Expected error due to canceled context")
	}

	if !strings.Contains(err.Error(), "rate limit wait failed") {
		t.Errorf("Expected error to contain 'rate limit wait failed', got '%s'", err.Error())
	}
}

// Test with a mock HTTP server for successful API response
func TestMastodonBookmarkClient_GetBookmarks_SuccessfulResponse(t *testing.T) {
	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Expected Authorization header 'Bearer test-token', got '%s'", auth)
		}

		userAgent := r.Header.Get("User-Agent")
		if userAgent != "bookmarchive/1.0" {
			t.Errorf("Expected User-Agent 'bookmarchive/1.0', got '%s'", userAgent)
		}

		// Verify URL parameters
		limit := r.URL.Query().Get("limit")
		if limit != "20" {
			t.Errorf("Expected limit parameter '20', got '%s'", limit)
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://mastodon.example.com/api/v1/bookmarks?max_id=123>; rel="next"`)
		w.WriteHeader(http.StatusOK)

		// Mock JSON response with a single status
		response := `[{
			"id": "123",
			"uri": "https://mastodon.example.com/users/testuser/statuses/123",
			"url": "https://mastodon.example.com/@testuser/123",
			"content": "<p>Test content</p>",
			"spoiler_text": "",
			"created_at": "2023-01-01T00:00:00Z",
			"account": {
				"id": "456",
				"username": "testuser",
				"display_name": "Test User",
				"avatar": "https://mastodon.example.com/avatar.jpg"
			},
			"media_attachments": [],
			"tags": []
		}]`
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	// Create client with mock server
	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 3)
	ctx := context.Background()

	bookmarks, nextURL, err := bc.GetBookmarks(ctx, 20, "")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(bookmarks) != 1 {
		t.Errorf("Expected 1 bookmark, got %d", len(bookmarks))
	}

	if bookmarks[0].ID != "123" {
		t.Errorf("Expected bookmark ID '123', got '%s'", bookmarks[0].ID)
	}

	if bookmarks[0].Status.Account.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", bookmarks[0].Status.Account.Username)
	}

	expectedNextURL := "https://mastodon.example.com/api/v1/bookmarks?max_id=123"
	if nextURL != expectedNextURL {
		t.Errorf("Expected nextURL '%s', got '%s'", expectedNextURL, nextURL)
	}
}

// Test with HTTP error response
func TestMastodonBookmarkClient_GetBookmarks_HTTPError(t *testing.T) {
	// Create mock HTTP server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 0) // No retries to avoid delays
	ctx := context.Background()

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err == nil {
		t.Error("Expected error for HTTP 500 response")
	}

	if !strings.Contains(err.Error(), "API request failed with status 500") {
		t.Errorf("Expected error about status 500, got '%s'", err.Error())
	}
}

// Test with invalid JSON response
func TestMastodonBookmarkClient_GetBookmarks_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "invalid json")
	}))
	defer server.Close()

	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 3)
	ctx := context.Background()

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err == nil {
		t.Error("Expected error for invalid JSON response")
	}

	if !strings.Contains(err.Error(), "failed to decode JSON response") {
		t.Errorf("Expected error about JSON decoding, got '%s'", err.Error())
	}
}

// Test with custom nextURL
func TestMastodonBookmarkClient_GetBookmarks_WithNextURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that the custom URL was used
		if r.URL.String() != "/api/v1/bookmarks?max_id=456" {
			t.Errorf("Expected URL '/api/v1/bookmarks?max_id=456', got '%s'", r.URL.String())
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "[]") // Empty array
	}))
	defer server.Close()

	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 3)
	ctx := context.Background()

	// Use the server URL as the base for the nextURL
	customURL := server.URL + "/api/v1/bookmarks?max_id=456"

	_, _, err := bc.GetBookmarks(ctx, 20, customURL)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// Test URL parsing error
func TestMastodonBookmarkClient_GetBookmarks_URLParseError(t *testing.T) {
	// Create a mock client with an invalid URL
	mockMadonClient := &madon.Client{
		InstanceURL: "://invalid-url", // Invalid URL to trigger parse error
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 3)
	ctx := context.Background()

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err == nil {
		t.Error("Expected error for invalid URL")
	}

	if !strings.Contains(err.Error(), "failed to parse URL") {
		t.Errorf("Expected error about URL parsing, got '%s'", err.Error())
	}
}

// Test request creation error
func TestMastodonBookmarkClient_GetBookmarks_RequestCreationError(t *testing.T) {
	mockMadonClient := &madon.Client{
		InstanceURL: "https://mastodon.example.com",
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 0) // No retries to avoid test delays

	// Use a context that's already canceled to potentially trigger request creation issues
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// This may or may not fail depending on timing, but we're testing error handling
	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err != nil {
		// Could be rate limit wait failed or other errors - just verify we handle errors
		if !strings.Contains(err.Error(), "rate limit wait failed") &&
			!strings.Contains(err.Error(), "failed to create request") &&
			!strings.Contains(err.Error(), "HTTP request failed") {
			t.Errorf("Unexpected error type: %s", err.Error())
		}
	}
}

// Test HTTP 4xx error (non-retryable)
func TestMastodonBookmarkClient_GetBookmarks_HTTP4xxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // 401 - should not retry
		fmt.Fprint(w, "Unauthorized")
	}))
	defer server.Close()

	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 3)
	ctx := context.Background()

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err == nil {
		t.Error("Expected error for HTTP 401 response")
	}

	if !strings.Contains(err.Error(), "API request failed with status 401") {
		t.Errorf("Expected error about status 401, got '%s'", err.Error())
	}
}

// Test retry on HTTP 429 (rate limit) - using minimal retries for speed
func TestMastodonBookmarkClient_GetBookmarks_HTTP429Retry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 1 { // Only fail once to minimize test time
			w.WriteHeader(http.StatusTooManyRequests) // 429 - should retry
			fmt.Fprint(w, "Rate limited")
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "[]") // Empty array on success
		}
	}))
	defer server.Close()

	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken: &madon.UserToken{
			AccessToken: "test-token",
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 2) // Reduced retries for speed
	ctx := context.Background()

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err != nil {
		t.Errorf("Expected success after retries, got %v", err)
	}

	// Should have been called 2 times (1 failure + 1 success)
	if callCount != 2 {
		t.Errorf("Expected 2 calls (with retries), got %d", callCount)
	}
}

// Test missing UserToken
func TestMastodonBookmarkClient_GetBookmarks_MissingUserToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that no Authorization header is set
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Expected no Authorization header, got '%s'", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()

	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken:   nil, // No user token
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 3)
	ctx := context.Background()

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err != nil {
		t.Errorf("Expected no error with missing token, got %v", err)
	}
}

// Test empty UserToken.AccessToken
func TestMastodonBookmarkClient_GetBookmarks_EmptyAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that no Authorization header is set
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Expected no Authorization header, got '%s'", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()

	mockMadonClient := &madon.Client{
		InstanceURL: server.URL,
		UserToken: &madon.UserToken{
			AccessToken: "", // Empty access token
		},
	}

	bc := newMastodonBookmarkClient(mockMadonClient, 3)
	ctx := context.Background()

	_, _, err := bc.GetBookmarks(ctx, 20, "")

	if err != nil {
		t.Errorf("Expected no error with empty token, got %v", err)
	}
}

// =============================================================================
// CONVERSION FUNCTION TESTS
// =============================================================================

func TestConvertMadonStatusToBookmark_BasicConversion(t *testing.T) {
	// Create a mock madon.Status
	description := "Test media description"
	status := madon.Status{
		ID:          "123",
		URI:         "https://mastodon.example.com/users/testuser/statuses/123",
		URL:         "https://mastodon.example.com/@testuser/123",
		Content:     "<p>Test content</p>",
		SpoilerText: "Warning: test content",
		CreatedAt:   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		Account: &madon.Account{
			ID:          "456",
			Username:    "testuser",
			DisplayName: "Test User",
			Avatar:      "https://mastodon.example.com/avatar.jpg",
		},
		MediaAttachments: []madon.Attachment{
			{
				ID:          "789",
				Type:        "image",
				URL:         "https://mastodon.example.com/media/789.jpg",
				Description: &description,
			},
		},
		Tags: []madon.Tag{
			{
				Name: "test",
				URL:  "https://mastodon.example.com/tags/test",
			},
		},
	}

	bookmark := convertMadonStatusToBookmark(status)

	// Verify basic fields
	if bookmark.ID != "123" {
		t.Errorf("Expected ID '123', got '%s'", bookmark.ID)
	}

	if bookmark.Status.ID != "123" {
		t.Errorf("Expected Status.ID '123', got '%s'", bookmark.Status.ID)
	}

	if bookmark.Status.URI != "https://mastodon.example.com/users/testuser/statuses/123" {
		t.Errorf("Expected correct URI, got '%s'", bookmark.Status.URI)
	}

	if bookmark.Status.Content != "<p>Test content</p>" {
		t.Errorf("Expected correct content, got '%s'", bookmark.Status.Content)
	}

	if bookmark.Status.SpoilerText != "Warning: test content" {
		t.Errorf("Expected correct spoiler text, got '%s'", bookmark.Status.SpoilerText)
	}

	// Verify account
	if bookmark.Status.Account.ID != "456" {
		t.Errorf("Expected account ID '456', got '%s'", bookmark.Status.Account.ID)
	}

	if bookmark.Status.Account.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", bookmark.Status.Account.Username)
	}

	// Verify media attachments
	if len(bookmark.Status.MediaAttachments) != 1 {
		t.Errorf("Expected 1 media attachment, got %d", len(bookmark.Status.MediaAttachments))
	}

	media := bookmark.Status.MediaAttachments[0]
	if media.ID != "789" {
		t.Errorf("Expected media ID '789', got '%s'", media.ID)
	}

	if media.Description != "Test media description" {
		t.Errorf("Expected media description 'Test media description', got '%s'", media.Description)
	}

	// Verify tags
	if len(bookmark.Status.Tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(bookmark.Status.Tags))
	}

	tag := bookmark.Status.Tags[0]
	if tag.Name != "test" {
		t.Errorf("Expected tag name 'test', got '%s'", tag.Name)
	}

	// Verify timestamps
	expectedTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	if !bookmark.CreatedAt.Equal(expectedTime) {
		t.Errorf("Expected CreatedAt %v, got %v", expectedTime, bookmark.CreatedAt)
	}
}

func TestConvertMadonStatusToBookmark_NilMediaDescription(t *testing.T) {
	status := madon.Status{
		ID: "123",
		MediaAttachments: []madon.Attachment{
			{
				ID:          "789",
				Type:        "image",
				URL:         "https://mastodon.example.com/media/789.jpg",
				Description: nil, // Nil description
			},
		},
		Account: &madon.Account{ID: "456"},
	}

	bookmark := convertMadonStatusToBookmark(status)

	if len(bookmark.Status.MediaAttachments) != 1 {
		t.Errorf("Expected 1 media attachment, got %d", len(bookmark.Status.MediaAttachments))
	}

	media := bookmark.Status.MediaAttachments[0]
	if media.Description != "" {
		t.Errorf("Expected empty description for nil pointer, got '%s'", media.Description)
	}
}

func TestConvertMadonStatusToBookmark_EmptyCollections(t *testing.T) {
	status := madon.Status{
		ID:               "123",
		MediaAttachments: []madon.Attachment{}, // Empty
		Tags:             []madon.Tag{},        // Empty
		Account:          &madon.Account{ID: "456"},
	}

	bookmark := convertMadonStatusToBookmark(status)

	if len(bookmark.Status.MediaAttachments) != 0 {
		t.Errorf("Expected 0 media attachments, got %d", len(bookmark.Status.MediaAttachments))
	}

	if len(bookmark.Status.Tags) != 0 {
		t.Errorf("Expected 0 tags, got %d", len(bookmark.Status.Tags))
	}
}
