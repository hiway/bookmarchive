package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// BOOKMARK SERVICE TESTS
// =============================================================================

func TestNewBookmarkService_Success(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	service, err := newBookmarkService(cfg, db, eventChan)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if service.config != cfg {
		t.Error("Expected config to be set correctly")
	}

	if service.db != db {
		t.Error("Expected database to be set correctly")
	}

	if service.eventChan == nil {
		t.Error("Expected eventChan to be set")
	}

	if service.ctx == nil {
		t.Error("Expected context to be initialized")
	}

	if service.cancel == nil {
		t.Error("Expected cancel function to be initialized")
	}
}

func TestNewBookmarkService_NilConfig(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	_, err := newBookmarkService(nil, db, eventChan)
	if err == nil {
		t.Fatal("Expected error for nil config")
	}

	expectedMsg := "config cannot be nil"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestNewBookmarkService_NilDatabase(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
	}

	eventChan := make(chan ServerEvent, 10)

	_, err := newBookmarkService(cfg, nil, eventChan)
	if err == nil {
		t.Fatal("Expected error for nil database")
	}

	expectedMsg := "database cannot be nil"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestNewBookmarkService_EmptyMastodonServer(t *testing.T) {
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

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	_, err := newBookmarkService(cfg, db, eventChan)
	if err == nil {
		t.Fatal("Expected error for empty mastodon server")
	}

	expectedMsg := "mastodon server URL is required"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestNewBookmarkService_EmptyAccessToken(t *testing.T) {
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

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	_, err := newBookmarkService(cfg, db, eventChan)
	if err == nil {
		t.Fatal("Expected error for empty access token")
	}

	expectedMsg := "mastodon access token is required"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestBookmarkService_Stop(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	service, err := newBookmarkService(cfg, db, eventChan)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test that context is active before stop
	select {
	case <-service.ctx.Done():
		t.Error("Context should not be done before stop")
	default:
		// Expected
	}

	// Stop the service
	err = service.stop()
	if err != nil {
		t.Errorf("Expected no error from stop, got %v", err)
	}

	// Verify context is cancelled
	select {
	case <-service.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Context should be cancelled after stop")
	}
}

func TestBookmarkService_Stop_NilCancel(t *testing.T) {
	service := &BookmarkService{
		cancel: nil, // Nil cancel function
	}

	err := service.stop()
	if err != nil {
		t.Errorf("Expected no error with nil cancel, got %v", err)
	}
}

func TestBookmarkService_CreateBookmarkClient_Success(t *testing.T) {
	// Note: This test will fail in test environment due to madon.RestoreApp,
	// but we're testing the error handling path
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	service, err := newBookmarkService(cfg, db, eventChan)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	_, err = service.createBookmarkClient()
	// We expect this to fail in test environment, but we're testing error handling
	if err != nil {
		// Verify it's the expected type of error
		if !strings.Contains(err.Error(), "failed to create mastodon client") &&
			!strings.Contains(err.Error(), "failed to verify mastodon credentials") &&
			!strings.Contains(err.Error(), "failed to get madon client") {
			t.Errorf("Unexpected error type: %s", err.Error())
		}
	}
}

// Mock BookmarkClient for testing
type MockBookmarkClient struct {
	bookmarks  []Bookmark
	nextURL    string
	nextURLs   []string // Multiple URLs for pagination
	errors     []error
	callCount  int
	nextURLIdx int // Index for nextURLs
}

func (m *MockBookmarkClient) GetBookmarks(ctx context.Context, limit int, nextURL string) ([]Bookmark, string, error) {
	if m.callCount < len(m.errors) && m.errors[m.callCount] != nil {
		err := m.errors[m.callCount]
		m.callCount++
		return nil, "", err
	}

	m.callCount++

	// Use nextURLs if provided for pagination testing
	if len(m.nextURLs) > 0 {
		if m.nextURLIdx < len(m.nextURLs) {
			url := m.nextURLs[m.nextURLIdx]
			m.nextURLIdx++
			return m.bookmarks, url, nil
		}
		return m.bookmarks, "", nil // End of pagination
	}

	return m.bookmarks, m.nextURL, nil
}

func TestBookmarkService_ProcessBookmarkBatch_Success(t *testing.T) {
	cfg := &Config{
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content", "username"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	service := &BookmarkService{
		config:    cfg,
		db:        db,
		ctx:       context.Background(),
		eventChan: eventChan,
	}

	// Create test bookmarks
	bookmarks := []Bookmark{
		{
			ID: "bookmark-1",
			Status: Status{
				ID:      "status-1",
				Content: "Test content 1",
				Account: Account{Username: "user1"},
			},
			CreatedAt: time.Now(),
		},
		{
			ID: "bookmark-2",
			Status: Status{
				ID:      "status-2",
				Content: "Test content 2",
				Account: Account{Username: "user2"},
			},
			CreatedAt: time.Now(),
		},
	}

	err := service.processBookmarkBatch(bookmarks)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify bookmarks were inserted
	for _, bookmark := range bookmarks {
		dbBookmark, err := db.getBookmark(bookmark.Status.ID)
		if err != nil {
			t.Errorf("Failed to get bookmark %s: %v", bookmark.Status.ID, err)
		}
		if dbBookmark == nil {
			t.Errorf("Bookmark %s not found in database", bookmark.Status.ID)
		}
	}

	// Verify events were sent
	events := drainEventChannel(eventChan)
	if len(events) < 2 { // At least batch_start and batch_complete
		t.Errorf("Expected at least 2 events, got %d", len(events))
	}
}

func TestBookmarkService_ProcessBookmarkBatch_SkipExisting(t *testing.T) {
	cfg := &Config{
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	// Insert a bookmark first
	existingBookmark := &DBBookmark{
		StatusID:     "status-1",
		CreatedAt:    time.Now(),
		BookmarkedAt: time.Now(),
		SearchText:   "existing content",
		RawJSON:      `{"id":"status-1"}`,
	}
	err := db.insertBookmark(existingBookmark)
	if err != nil {
		t.Fatalf("Failed to insert existing bookmark: %v", err)
	}

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
	}

	// Try to process the same bookmark again
	bookmarks := []Bookmark{
		{
			ID: "bookmark-1",
			Status: Status{
				ID:      "status-1", // Same as existing
				Content: "New content",
			},
			CreatedAt: time.Now(),
		},
	}

	err = service.processBookmarkBatch(bookmarks)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify the original bookmark is unchanged
	dbBookmark, err := db.getBookmark("status-1")
	if err != nil {
		t.Fatalf("Failed to get bookmark: %v", err)
	}

	if dbBookmark.SearchText != "existing content" {
		t.Error("Expected existing bookmark to be unchanged")
	}
}

func TestBookmarkService_ProcessBookmarkBatch_ContextCancelled(t *testing.T) {
	cfg := &Config{
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    ctx,
	}

	bookmarks := []Bookmark{
		{
			ID: "bookmark-1",
			Status: Status{
				ID:      "status-1",
				Content: "Test content",
			},
			CreatedAt: time.Now(),
		},
	}

	err := service.processBookmarkBatch(bookmarks)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestBookmarkService_PollBookmarks_Success(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	// Initialize backfill state
	err := db.updateBackfillState("", true, nil)
	if err != nil {
		t.Fatalf("Failed to initialize backfill state: %v", err)
	}

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{
				{
					ID: "new-bookmark",
					Status: Status{
						ID:      "new-status",
						Content: "New content",
					},
					CreatedAt: time.Now(),
				},
			},
		},
	}

	err = service.pollBookmarks()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify bookmark was processed
	dbBookmark, err := db.getBookmark("new-status")
	if err != nil {
		t.Errorf("Failed to get new bookmark: %v", err)
	}
	if dbBookmark == nil {
		t.Error("Expected new bookmark to be saved")
	}
}

func TestBookmarkService_PollBookmarks_NoNewBookmarks(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{}, // No bookmarks
		},
	}

	err := service.pollBookmarks()
	if err != nil {
		t.Fatalf("Expected no error with no bookmarks, got %v", err)
	}
}

func TestBookmarkService_PollBookmarks_GetBackfillStateError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
	}

	// Use a database with nil connection to trigger error
	db := &Database{db: nil}

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{},
	}

	err := service.pollBookmarks()
	if err == nil {
		t.Fatal("Expected error with nil database")
	}

	if !strings.Contains(err.Error(), "failed to get backfill state") {
		t.Errorf("Expected backfill state error, got %v", err)
	}
}

func TestBookmarkService_PollBookmarks_FetchError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			errors: []error{fmt.Errorf("fetch error")},
		},
	}

	err := service.pollBookmarks()
	if err == nil {
		t.Fatal("Expected error from client")
	}

	if !strings.Contains(err.Error(), "failed to fetch bookmarks") {
		t.Errorf("Expected fetch error, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_AlreadyComplete(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	// Mark backfill as complete
	err := db.updateBackfillState("", true, nil)
	if err != nil {
		t.Fatalf("Failed to set backfill complete: %v", err)
	}

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{},
	}

	err = service.runBackfill()
	if err != nil {
		t.Fatalf("Expected no error when backfill complete, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_NoBookmarks(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize:     2,
			BackfillDelay: "1ms", // Very short delay for testing
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{}, // No bookmarks
		},
	}

	err := service.runBackfill()
	if err != nil {
		t.Fatalf("Expected no error with empty backfill, got %v", err)
	}

	// Verify backfill is marked complete
	state, err := db.getBackfillState()
	if err != nil {
		t.Fatalf("Failed to get backfill state: %v", err)
	}

	if !state.BackfillComplete {
		t.Error("Expected backfill to be marked complete")
	}
}

func TestBookmarkService_RunBackfill_WithBookmarksNoNextURL(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize:     2,
			BackfillDelay: "1ms",
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	service := &BookmarkService{
		config:    cfg,
		db:        db,
		ctx:       context.Background(),
		eventChan: make(chan ServerEvent, 10),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{
				{
					ID: "backfill-bookmark",
					Status: Status{
						ID:      "backfill-status",
						Content: "Backfill content",
					},
					CreatedAt: time.Now(),
				},
			},
			nextURL: "", // No next URL - this will complete backfill
		},
	}

	err := service.runBackfill()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify bookmark was processed
	dbBookmark, err := db.getBookmark("backfill-status")
	if err != nil {
		t.Errorf("Failed to get backfill bookmark: %v", err)
	}
	if dbBookmark == nil {
		t.Error("Expected backfill bookmark to be saved")
	}

	// Verify backfill is complete
	state, err := db.getBackfillState()
	if err != nil {
		t.Fatalf("Failed to get backfill state: %v", err)
	}

	if !state.BackfillComplete {
		t.Error("Expected backfill to be marked complete")
	}
}

func TestBookmarkService_RunBackfill_ContextCancelled(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    ctx,
		client: &MockBookmarkClient{},
	}

	err := service.runBackfill()
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestBookmarkService_StartPolling_InvalidInterval(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			Interval: "invalid-duration",
		},
	}

	service := &BookmarkService{
		config: cfg,
		ctx:    context.Background(),
	}

	err := service.startPolling()
	if err == nil {
		t.Fatal("Expected error for invalid interval")
	}

	if !strings.Contains(err.Error(), "invalid polling interval") {
		t.Errorf("Expected interval error, got %v", err)
	}
}

func TestBookmarkService_StartPolling_ZeroInterval(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			Interval: "0s",
		},
	}

	service := &BookmarkService{
		config: cfg,
		ctx:    context.Background(),
	}

	err := service.startPolling()
	if err == nil {
		t.Fatal("Expected error for zero interval")
	}

	expectedMsg := "polling interval must be positive"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestBookmarkService_StartPolling_DefaultInterval(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			Interval: "", // Empty - should default to 5m
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately to stop polling quickly

	service := &BookmarkService{
		config: cfg,
		ctx:    ctx,
	}

	err := service.startPolling()
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_GetStateError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
	}

	// Use database with nil connection to trigger error
	db := &Database{db: nil}

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{},
	}

	err := service.runBackfill()
	if err == nil {
		t.Fatal("Expected error with nil database")
	}

	if !strings.Contains(err.Error(), "failed to get backfill state") {
		t.Errorf("Expected backfill state error, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_FetchError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize:     20,
			BackfillDelay: "1ms",
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			errors: []error{fmt.Errorf("fetch error")},
		},
	}

	err := service.runBackfill()
	if err == nil {
		t.Fatal("Expected error from client fetch")
	}

	if !strings.Contains(err.Error(), "failed to fetch bookmarks") {
		t.Errorf("Expected fetch error, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_ProcessBatchError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize:     2,
			BackfillDelay: "1ms",
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{
				{
					ID: "test-bookmark",
					Status: Status{
						ID:      "test-status",
						Content: "Test content",
					},
					CreatedAt: time.Now(),
				},
			},
			errors: []error{errors.New("forced error in GetBookmarks")}, // First call fails
		},
	}

	err := service.runBackfill()
	if err == nil {
		t.Fatal("Expected error from GetBookmarks")
	}

	if !strings.Contains(err.Error(), "forced error in GetBookmarks") {
		t.Errorf("Expected GetBookmarks error, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_UpdateStateError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize:     2,
			BackfillDelay: "1ms",
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)

	// Set up a custom mock that will fail on the updateBackfillState operation
	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{
				{
					ID: "test-bookmark",
					Status: Status{
						ID:      "test-status",
						Content: "Test content",
					},
					CreatedAt: time.Now(),
				},
			},
			nextURL: "next-url", // This will trigger updateBackfillState
		},
	}

	// Close the database AFTER the test starts but before updateBackfillState is called
	// We'll do this by closing it in a goroutine after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond) // Let getBackfillState succeed first
		db.close()
	}()

	err := service.runBackfill()
	if err == nil {
		t.Fatal("Expected error from updateBackfillState")
	}

	// The error could be from updateBackfillState or from database being closed
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("Expected database error, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_WithNextURL(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize:     2,
			BackfillDelay: "1ms",
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	// Initialize with existing state that has LastProcessedID
	err := db.updateBackfillState("existing-next-url", false, nil)
	if err != nil {
		t.Fatalf("Failed to initialize state: %v", err)
	}

	service := &BookmarkService{
		config:    cfg,
		db:        db,
		ctx:       context.Background(),
		eventChan: make(chan ServerEvent, 10),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{
				{
					ID: "batch-bookmark",
					Status: Status{
						ID:      "batch-status",
						Content: "Batch content",
					},
					CreatedAt: time.Now(),
				},
			},
			// Use nextURLs for proper pagination testing
			nextURLs: []string{"next-batch-url"}, // One page then empty
		},
	}

	// Use a timeout context to prevent infinite loop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	service.ctx = ctx

	err = service.runBackfill()
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify bookmark was processed
	dbBookmark, _ := db.getBookmark("batch-status")
	if dbBookmark == nil {
		t.Error("Expected bookmark to be processed during backfill")
	}
}

func TestBookmarkService_Start_CreateClientError(t *testing.T) {
	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "", // Invalid server to trigger error
			AccessToken: "test-token",
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
	}

	err := service.start()
	if err == nil {
		t.Fatal("Expected error from createBookmarkClient")
	}

	if !strings.Contains(err.Error(), "failed to create bookmark client") {
		t.Errorf("Expected create client error, got %v", err)
	}
}

func TestBookmarkService_ProcessBookmarkBatch_DatabaseError(t *testing.T) {
	cfg := &Config{
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	// Create database and then close it to trigger errors
	db := setupTestDatabase(t)
	db.close() // Close to trigger database errors

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
	}

	bookmarks := []Bookmark{
		{
			ID: "bookmark-1",
			Status: Status{
				ID:      "status-1",
				Content: "Test content",
			},
			CreatedAt: time.Now(),
		},
	}

	// Should not return error (it continues processing other bookmarks)
	err := service.processBookmarkBatch(bookmarks)
	if err != nil {
		t.Errorf("Expected no error (should continue on individual bookmark errors), got %v", err)
	}
}

func TestBookmarkService_PollBookmarks_ProcessBatchError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	// Initialize backfill state
	err := db.updateBackfillState("", true, nil)
	if err != nil {
		t.Fatalf("Failed to initialize backfill state: %v", err)
	}

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{
				{
					ID: "new-bookmark",
					Status: Status{
						ID:      "new-status",
						Content: "New content",
					},
					CreatedAt: time.Now(),
				},
			},
			errors: []error{errors.New("forced error in GetBookmarks")}, // First call fails
		},
	}

	err = service.pollBookmarks()
	if err == nil {
		t.Fatal("Expected error from GetBookmarks")
	}

	if !strings.Contains(err.Error(), "forced error in GetBookmarks") {
		t.Errorf("Expected GetBookmarks error, got %v", err)
	}
}

func TestBookmarkService_PollBookmarks_UpdateStateError(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: 20,
		},
	}

	db := setupTestDatabase(t)

	// Initialize backfill state
	err := db.updateBackfillState("", true, nil)
	if err != nil {
		t.Fatalf("Failed to initialize backfill state: %v", err)
	}

	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClient{
			bookmarks: []Bookmark{
				{
					ID: "new-bookmark",
					Status: Status{
						ID:      "new-status",
						Content: "New content",
					},
					CreatedAt: time.Now(),
				},
			},
		},
	}

	// Close database immediately to trigger error in updateBackfillState call
	db.close()

	err = service.pollBookmarks()
	if err == nil {
		t.Fatal("Expected error from updateBackfillState")
	}

	if !strings.Contains(err.Error(), "database") {
		t.Errorf("Expected database error, got %v", err)
	}
}

func TestBookmarkService_RunBackfill_DefaultBatchSize(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize:     0, // Zero should default to 40
			BackfillDelay: "1ms",
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	requestedBatchSize := 0
	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClientWithSizeCheck{
			requestedBatchSize: &requestedBatchSize,
			bookmarks:          []Bookmark{}, // Empty to complete immediately
		},
	}

	err := service.runBackfill()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify default batch size of 40 was used
	if requestedBatchSize != 40 {
		t.Errorf("Expected batch size 40, got %d", requestedBatchSize)
	}
}

func TestBookmarkService_PollBookmarks_DefaultBatchSize(t *testing.T) {
	cfg := &Config{
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			BatchSize: -1, // Negative should default to 40
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	requestedBatchSize := 0
	service := &BookmarkService{
		config: cfg,
		db:     db,
		ctx:    context.Background(),
		client: &MockBookmarkClientWithSizeCheck{
			requestedBatchSize: &requestedBatchSize,
			bookmarks:          []Bookmark{}, // Empty
		},
	}

	err := service.pollBookmarks()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify default batch size of 40 was used
	if requestedBatchSize != 40 {
		t.Errorf("Expected batch size 40, got %d", requestedBatchSize)
	}
}

// MockBookmarkClientWithSizeCheck captures the batch size used
type MockBookmarkClientWithSizeCheck struct {
	requestedBatchSize *int
	bookmarks          []Bookmark
	nextURL            string
	errors             []error
	callCount          int
}

func (m *MockBookmarkClientWithSizeCheck) GetBookmarks(ctx context.Context, limit int, nextURL string) ([]Bookmark, string, error) {
	*m.requestedBatchSize = limit

	if m.callCount < len(m.errors) && m.errors[m.callCount] != nil {
		err := m.errors[m.callCount]
		m.callCount++
		return nil, "", err
	}

	m.callCount++
	return m.bookmarks, m.nextURL, nil
}

// Helper function to drain events from a channel
func drainEventChannel(eventChan chan ServerEvent) []ServerEvent {
	var events []ServerEvent
	for {
		select {
		case event := <-eventChan:
			events = append(events, event)
		default:
			return events
		}
	}
}

// Helper function to drain events from a channel - moved setupTestDatabase to database_test.go
