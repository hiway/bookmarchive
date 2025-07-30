package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// TEST HELPERS AND SETUP
// =============================================================================

func setupTestDatabase(t *testing.T) *Database {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false, // Use normal mode for tests to avoid WAL files
			BusyTimeout: "1s",
		},
	}

	db, err := newDatabase(cfg)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return db
}

func createTestBookmark(statusID string, content string) *DBBookmark {
	now := time.Now()
	return &DBBookmark{
		StatusID:     statusID,
		CreatedAt:    now.Add(-time.Hour),
		BookmarkedAt: now,
		SearchText:   content,
		RawJSON:      `{"id":"` + statusID + `","content":"` + content + `"}`,
	}
}

// =============================================================================
// DATABASE CREATION AND CONFIGURATION TESTS
// =============================================================================

func TestNewDatabase_SuccessfulCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     true,
			BusyTimeout: "5s",
		},
	}

	db, err := newDatabase(cfg)
	if err != nil {
		t.Fatalf("Expected successful database creation, got error: %v", err)
	}
	defer db.close()

	if db.db == nil {
		t.Error("Expected database connection to be non-nil")
	}

	if db.path != dbPath {
		t.Errorf("Expected database path '%s', got '%s'", dbPath, db.path)
	}

	// Verify the database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected database file to be created")
	}
}

func TestNewDatabase_InvalidBusyTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false,
			BusyTimeout: "invalid-duration",
		},
	}

	_, err := newDatabase(cfg)
	if err == nil {
		t.Error("Expected error for invalid busy timeout")
	}

	if !strings.Contains(err.Error(), "invalid busy timeout") {
		t.Errorf("Expected 'invalid busy timeout' error, got: %v", err)
	}
}

func TestNewDatabase_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "nested", "directory")
	dbPath := filepath.Join(subDir, "test.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false,
			BusyTimeout: "1s",
		},
	}

	db, err := newDatabase(cfg)
	if err != nil {
		t.Fatalf("Expected successful database creation with directory creation, got error: %v", err)
	}
	defer db.close()

	// Verify the nested directory was created
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Error("Expected nested directory to be created")
	}

	// Verify the database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected database file to be created in nested directory")
	}
}

func TestNewDatabase_DefaultBusyTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false,
			BusyTimeout: "", // Empty should use default
		},
	}

	db, err := newDatabase(cfg)
	if err != nil {
		t.Fatalf("Expected successful database creation with default timeout, got error: %v", err)
	}
	defer db.close()

	// Test should pass - we can't easily verify the timeout was set to 5s,
	// but if the database was created successfully, the default was used
}

// =============================================================================
// DATABASE CLOSE TESTS
// =============================================================================

func TestDatabase_Close(t *testing.T) {
	db := setupTestDatabase(t)

	err := db.close()
	if err != nil {
		t.Errorf("Expected successful close, got error: %v", err)
	}

	// Verify the database connection is nil after close
	if db.db != nil {
		t.Error("Expected database connection to be nil after close")
	}

	// Second close should be safe (no error)
	err = db.close()
	if err != nil {
		t.Errorf("Expected second close to be safe, got error: %v", err)
	}
}

// =============================================================================
// MIGRATION TESTS
// =============================================================================

func TestDatabase_RunMigrations(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Verify that tables were created by migrations
	tables := []string{"bookmarks", "bookmarks_fts", "backfill_state"}

	for _, table := range tables {
		var count int
		query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?"
		err := db.db.QueryRow(query, table).Scan(&count)
		if err != nil {
			t.Errorf("Failed to check for table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("Expected table %s to exist, found %d tables with that name", table, count)
		}
	}

	// Verify indexes were created
	indexes := []string{"idx_created_at", "idx_bookmarked_at"}

	for _, index := range indexes {
		var count int
		query := "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?"
		err := db.db.QueryRow(query, index).Scan(&count)
		if err != nil {
			t.Errorf("Failed to check for index %s: %v", index, err)
		}
		if count != 1 {
			t.Errorf("Expected index %s to exist, found %d indexes with that name", index, count)
		}
	}

	// Verify backfill_state has initial record
	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM backfill_state").Scan(&count)
	if err != nil {
		t.Errorf("Failed to check backfill_state records: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 initial backfill_state record, found %d", count)
	}
}

func TestDatabase_RunMigrations_NilDatabase(t *testing.T) {
	db := &Database{db: nil}

	err := db.runMigrations()
	if err == nil {
		t.Error("Expected error when running migrations on nil database")
	}

	if !strings.Contains(err.Error(), "database connection is nil") {
		t.Errorf("Expected 'database connection is nil' error, got: %v", err)
	}
}

// =============================================================================
// BOOKMARK INSERTION TESTS
// =============================================================================

func TestDatabase_InsertBookmark_Success(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	bookmark := createTestBookmark("test-status-1", "This is a test bookmark")

	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Expected successful bookmark insertion, got error: %v", err)
	}

	// Verify bookmark was inserted into main table
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM bookmarks WHERE status_id = ?", bookmark.StatusID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to verify bookmark insertion: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 bookmark with status_id %s, found %d", bookmark.StatusID, count)
	}

	// Verify bookmark was indexed in FTS table
	err = db.db.QueryRow("SELECT COUNT(*) FROM bookmarks_fts WHERE status_id = ?", bookmark.StatusID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to verify FTS indexing: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 FTS entry for status_id %s, found %d", bookmark.StatusID, count)
	}
}

func TestDatabase_InsertBookmark_Replacement(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	bookmark := createTestBookmark("test-status-1", "Original content")

	// Insert initial bookmark
	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Failed to insert initial bookmark: %v", err)
	}

	// Update bookmark with new content
	bookmark.SearchText = "Updated content"
	bookmark.RawJSON = `{"id":"test-status-1","content":"Updated content"}`

	err = db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Expected successful bookmark replacement, got error: %v", err)
	}

	// Verify only one bookmark exists (was replaced, not duplicated)
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM bookmarks WHERE status_id = ?", bookmark.StatusID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to verify bookmark count: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 bookmark after replacement, found %d", count)
	}

	// Verify content was updated
	var searchText string
	err = db.db.QueryRow("SELECT search_text FROM bookmarks WHERE status_id = ?", bookmark.StatusID).Scan(&searchText)
	if err != nil {
		t.Errorf("Failed to verify updated content: %v", err)
	}
	if searchText != "Updated content" {
		t.Errorf("Expected updated content 'Updated content', got '%s'", searchText)
	}
}

func TestDatabase_InsertBookmark_NilDatabase(t *testing.T) {
	db := &Database{db: nil}
	bookmark := createTestBookmark("test-status-1", "Test content")

	err := db.insertBookmark(bookmark)
	if err == nil {
		t.Error("Expected error when inserting bookmark into nil database")
	}

	if !strings.Contains(err.Error(), "database connection is nil") {
		t.Errorf("Expected 'database connection is nil' error, got: %v", err)
	}
}

// =============================================================================
// BOOKMARK RETRIEVAL TESTS
// =============================================================================

func TestDatabase_GetBookmark_Found(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	originalBookmark := createTestBookmark("test-status-1", "Test content")

	// Insert bookmark
	err := db.insertBookmark(originalBookmark)
	if err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	// Retrieve bookmark
	retrievedBookmark, err := db.getBookmark("test-status-1")
	if err != nil {
		t.Fatalf("Expected successful bookmark retrieval, got error: %v", err)
	}

	if retrievedBookmark == nil {
		t.Fatal("Expected non-nil bookmark")
	}

	if retrievedBookmark.StatusID != originalBookmark.StatusID {
		t.Errorf("Expected status_id '%s', got '%s'", originalBookmark.StatusID, retrievedBookmark.StatusID)
	}

	if retrievedBookmark.SearchText != originalBookmark.SearchText {
		t.Errorf("Expected search_text '%s', got '%s'", originalBookmark.SearchText, retrievedBookmark.SearchText)
	}

	if retrievedBookmark.RawJSON != originalBookmark.RawJSON {
		t.Errorf("Expected raw_json '%s', got '%s'", originalBookmark.RawJSON, retrievedBookmark.RawJSON)
	}
}

func TestDatabase_GetBookmark_NotFound(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	retrievedBookmark, err := db.getBookmark("nonexistent-status")
	if err != nil {
		t.Errorf("Expected no error for nonexistent bookmark, got: %v", err)
	}

	if retrievedBookmark != nil {
		t.Error("Expected nil bookmark for nonexistent status")
	}
}

func TestDatabase_GetBookmark_NilDatabase(t *testing.T) {
	db := &Database{db: nil}

	_, err := db.getBookmark("test-status-1")
	if err == nil {
		t.Error("Expected error when getting bookmark from nil database")
	}

	if !strings.Contains(err.Error(), "database connection is nil") {
		t.Errorf("Expected 'database connection is nil' error, got: %v", err)
	}
}

// =============================================================================
// BACKFILL STATE TESTS
// =============================================================================

func TestDatabase_GetBackfillState_Initial(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	state, err := db.getBackfillState()
	if err != nil {
		t.Fatalf("Expected successful backfill state retrieval, got error: %v", err)
	}

	if state == nil {
		t.Fatal("Expected non-nil backfill state")
	}

	if state.LastProcessedID != "" {
		t.Errorf("Expected empty last_processed_id initially, got '%s'", state.LastProcessedID)
	}

	if state.BackfillComplete {
		t.Error("Expected backfill_complete to be false initially")
	}

	if state.LastPollTime != nil {
		t.Error("Expected last_poll_time to be nil initially")
	}

	// CreatedAt and UpdatedAt should be set
	if state.CreatedAt.IsZero() {
		t.Error("Expected created_at to be set")
	}

	if state.UpdatedAt.IsZero() {
		t.Error("Expected updated_at to be set")
	}
}

func TestDatabase_UpdateBackfillState_Success(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	lastProcessedID := "test-processed-id"
	backfillComplete := true
	now := time.Now()

	err := db.updateBackfillState(lastProcessedID, backfillComplete, &now)
	if err != nil {
		t.Fatalf("Expected successful backfill state update, got error: %v", err)
	}

	// Retrieve and verify updated state
	state, err := db.getBackfillState()
	if err != nil {
		t.Fatalf("Failed to retrieve updated backfill state: %v", err)
	}

	if state.LastProcessedID != lastProcessedID {
		t.Errorf("Expected last_processed_id '%s', got '%s'", lastProcessedID, state.LastProcessedID)
	}

	if !state.BackfillComplete {
		t.Error("Expected backfill_complete to be true")
	}

	if state.LastPollTime == nil {
		t.Error("Expected last_poll_time to be set")
	} else {
		// Allow for small time differences due to storage precision
		diff := state.LastPollTime.Sub(now)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected last_poll_time close to %v, got %v (diff: %v)", now, *state.LastPollTime, diff)
		}
	}
}

func TestDatabase_UpdateBackfillState_NilPollTime(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	err := db.updateBackfillState("test-id", false, nil)
	if err != nil {
		t.Fatalf("Expected successful update with nil poll time, got error: %v", err)
	}

	state, err := db.getBackfillState()
	if err != nil {
		t.Fatalf("Failed to retrieve backfill state: %v", err)
	}

	if state.LastPollTime != nil {
		t.Error("Expected last_poll_time to remain nil")
	}
}

func TestDatabase_GetBackfillState_NilDatabase(t *testing.T) {
	db := &Database{db: nil}

	_, err := db.getBackfillState()
	if err == nil {
		t.Error("Expected error when getting backfill state from nil database")
	}

	if !strings.Contains(err.Error(), "database connection is nil") {
		t.Errorf("Expected 'database connection is nil' error, got: %v", err)
	}
}

func TestDatabase_UpdateBackfillState_NilDatabase(t *testing.T) {
	db := &Database{db: nil}

	err := db.updateBackfillState("test-id", false, nil)
	if err == nil {
		t.Error("Expected error when updating backfill state on nil database")
	}

	if !strings.Contains(err.Error(), "database connection is nil") {
		t.Errorf("Expected 'database connection is nil' error, got: %v", err)
	}
}

// =============================================================================
// SEARCH TESTS
// =============================================================================

func TestDatabase_SearchBookmarksWithFTS5_EmptyQuery(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	request := &SearchRequest{
		Query: "",
		Limit: 10,
	}

	results, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Errorf("Expected no error for empty query, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results for empty query, got %d results", len(results))
	}
}

func TestDatabase_SearchBookmarksWithFTS5_WithResults(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert test bookmarks
	bookmarks := []*DBBookmark{
		createTestBookmark("status-1", "golang programming tutorial"),
		createTestBookmark("status-2", "python web development"),
		createTestBookmark("status-3", "golang database operations"),
	}

	for _, bookmark := range bookmarks {
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %s: %v", bookmark.StatusID, err)
		}
	}

	request := &SearchRequest{
		Query: "golang",
		Limit: 10,
	}

	results, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Fatalf("Expected successful search, got error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'golang' search, got %d", len(results))
	}

	// Verify results contain the expected bookmarks
	foundStatusIDs := make(map[string]bool)
	for _, result := range results {
		foundStatusIDs[result.Bookmark.StatusID] = true

		// Verify rank is set (should be negative for BM25)
		if result.Rank >= 0 {
			t.Errorf("Expected negative BM25 rank, got %f", result.Rank)
		}
	}

	if !foundStatusIDs["status-1"] || !foundStatusIDs["status-3"] {
		t.Error("Expected to find status-1 and status-3 in golang search results")
	}
}

func TestDatabase_SearchBookmarksWithFTS5_WithHighlighting(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	bookmark := createTestBookmark("status-1", "This is a golang programming tutorial")
	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	request := &SearchRequest{
		Query:              "golang",
		Limit:              10,
		EnableHighlighting: true,
		SnippetLength:      50,
	}

	results, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Fatalf("Expected successful search with highlighting, got error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Snippet == "" {
		t.Error("Expected snippet to be set when highlighting is enabled")
	}

	if !strings.Contains(result.Snippet, "<mark>") {
		t.Errorf("Expected snippet to contain highlighting marks, got: %s", result.Snippet)
	}
}

func TestDatabase_SearchBookmarksWithFTS5_LimitAndOffset(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert multiple test bookmarks
	for i := 1; i <= 5; i++ {
		bookmark := createTestBookmark(
			fmt.Sprintf("status-%d", i),
			fmt.Sprintf("test content %d", i),
		)
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %d: %v", i, err)
		}
	}

	// Test with limit
	request := &SearchRequest{
		Query: "test",
		Limit: 2,
	}

	results, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Fatalf("Expected successful search, got error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results with limit=2, got %d", len(results))
	}

	// Test with offset
	request.Offset = 2

	results, err = db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Fatalf("Expected successful search with offset, got error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results with limit=2 and offset=2, got %d", len(results))
	}
}

func TestDatabase_SearchBookmarksWithFTS5_DefaultLimits(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	request := &SearchRequest{
		Query:  "test",
		Limit:  0,  // Should default to 100
		Offset: -1, // Should default to 0
	}

	// This test just verifies that defaults are applied without error
	// We can't easily test the actual default values without inserting 100+ records
	_, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Errorf("Expected successful search with default limits, got error: %v", err)
	}
}

func TestDatabase_SearchBookmarksWithFTS5_NilDatabase(t *testing.T) {
	db := &Database{db: nil}

	request := &SearchRequest{Query: "test"}

	_, err := db.searchBookmarksWithFTS5(request)
	if err == nil {
		t.Error("Expected error when searching with nil database")
	}

	if !strings.Contains(err.Error(), "database connection is nil") {
		t.Errorf("Expected 'database connection is nil' error, got: %v", err)
	}
}

// =============================================================================
// RECENT BOOKMARKS TESTS
// =============================================================================

func TestDatabase_GetRecentBookmarks_Success(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert bookmarks with different timestamps
	now := time.Now()
	bookmarks := []*DBBookmark{
		{
			StatusID:     "status-1",
			CreatedAt:    now.Add(-3 * time.Hour),
			BookmarkedAt: now.Add(-1 * time.Hour), // Most recent
			SearchText:   "recent bookmark 1",
			RawJSON:      `{"id":"status-1"}`,
		},
		{
			StatusID:     "status-2",
			CreatedAt:    now.Add(-2 * time.Hour),
			BookmarkedAt: now.Add(-2 * time.Hour), // Oldest
			SearchText:   "recent bookmark 2",
			RawJSON:      `{"id":"status-2"}`,
		},
		{
			StatusID:     "status-3",
			CreatedAt:    now.Add(-1 * time.Hour),
			BookmarkedAt: now.Add(-30 * time.Minute), // Middle
			SearchText:   "recent bookmark 3",
			RawJSON:      `{"id":"status-3"}`,
		},
	}

	for _, bookmark := range bookmarks {
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %s: %v", bookmark.StatusID, err)
		}
	}

	results, err := db.getRecentBookmarks(10, 0)
	if err != nil {
		t.Fatalf("Expected successful recent bookmarks retrieval, got error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 recent bookmarks, got %d", len(results))
	}

	// Verify order (should be ordered by bookmarked_at DESC)
	expectedOrder := []string{"status-3", "status-1", "status-2"}
	for i, result := range results {
		if result.Bookmark.StatusID != expectedOrder[i] {
			t.Errorf("Expected bookmark %d to have status_id %s, got %s",
				i, expectedOrder[i], result.Bookmark.StatusID)
		}

		// Rank should be 0.0 for recent bookmarks
		if result.Rank != 0.0 {
			t.Errorf("Expected rank 0.0 for recent bookmark, got %f", result.Rank)
		}
	}
}

func TestDatabase_GetRecentBookmarks_LimitAndOffset(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert 5 bookmarks
	now := time.Now()
	for i := 1; i <= 5; i++ {
		bookmark := &DBBookmark{
			StatusID:     fmt.Sprintf("status-%d", i),
			CreatedAt:    now.Add(-time.Duration(i) * time.Hour),
			BookmarkedAt: now.Add(-time.Duration(i) * time.Minute),
			SearchText:   fmt.Sprintf("bookmark %d", i),
			RawJSON:      fmt.Sprintf(`{"id":"status-%d"}`, i),
		}
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %d: %v", i, err)
		}
	}

	// Test limit
	results, err := db.getRecentBookmarks(2, 0)
	if err != nil {
		t.Fatalf("Expected successful retrieval with limit, got error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results with limit=2, got %d", len(results))
	}

	// Test offset
	results, err = db.getRecentBookmarks(2, 2)
	if err != nil {
		t.Fatalf("Expected successful retrieval with offset, got error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results with limit=2 and offset=2, got %d", len(results))
	}
}

func TestDatabase_GetRecentBookmarks_DefaultLimits(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Test default limit (should default to 100)
	_, err := db.getRecentBookmarks(0, 0)
	if err != nil {
		t.Errorf("Expected successful retrieval with default limit, got error: %v", err)
	}

	// Test negative offset (should default to 0)
	_, err = db.getRecentBookmarks(10, -5)
	if err != nil {
		t.Errorf("Expected successful retrieval with negative offset, got error: %v", err)
	}
}

func TestDatabase_GetRecentBookmarks_EmptyDatabase(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	results, err := db.getRecentBookmarks(10, 0)
	if err != nil {
		t.Errorf("Expected no error for empty database, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty database, got %d", len(results))
	}
}

func TestDatabase_GetRecentBookmarks_NilDatabase(t *testing.T) {
	db := &Database{db: nil}

	_, err := db.getRecentBookmarks(10, 0)
	if err == nil {
		t.Error("Expected error when getting recent bookmarks from nil database")
	}

	if !strings.Contains(err.Error(), "database connection is nil") {
		t.Errorf("Expected 'database connection is nil' error, got: %v", err)
	}
}

// =============================================================================
// UNIFIED SEARCH TESTS
// =============================================================================

func TestDatabase_SearchOrRecentBookmarks_WithQuery(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert test bookmark
	bookmark := createTestBookmark("status-1", "golang programming")
	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	request := &SearchRequest{
		Query: "golang",
		Limit: 10,
	}

	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Fatalf("Expected successful search, got error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 search result, got %d", len(results))
	}

	// Should have a rank from FTS search
	if results[0].Rank >= 0 {
		t.Errorf("Expected negative BM25 rank from FTS search, got %f", results[0].Rank)
	}
}

func TestDatabase_SearchOrRecentBookmarks_EmptyQuery(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert test bookmark
	bookmark := createTestBookmark("status-1", "test content")
	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	request := &SearchRequest{
		Query: "",
		Limit: 10,
	}

	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Fatalf("Expected successful recent bookmarks retrieval, got error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 recent bookmark, got %d", len(results))
	}

	// Should have rank 0.0 from recent bookmarks
	if results[0].Rank != 0.0 {
		t.Errorf("Expected rank 0.0 from recent bookmarks, got %f", results[0].Rank)
	}
}

func TestDatabase_SearchOrRecentBookmarks_WhitespaceQuery(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert test bookmark
	bookmark := createTestBookmark("status-1", "test content")
	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	request := &SearchRequest{
		Query: "   \t\n   ", // Only whitespace
		Limit: 10,
	}

	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Fatalf("Expected successful recent bookmarks retrieval, got error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 recent bookmark, got %d", len(results))
	}

	// Should use recent bookmarks for whitespace-only query
	if results[0].Rank != 0.0 {
		t.Errorf("Expected rank 0.0 from recent bookmarks, got %f", results[0].Rank)
	}
}

// =============================================================================
// QUERY PREPARATION TESTS
// =============================================================================

func TestPrepareFTS5Query_SimpleWord(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello*"},
		{"test", "test*"},
		{"golang", "golang*"},
	}

	for _, test := range tests {
		result := prepareFTS5Query(test.input)
		if result != test.expected {
			t.Errorf("prepareFTS5Query(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestPrepareFTS5Query_MultipleWords(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello* world*"},
		{"go programming", "go* programming*"},
		{"test data base", "test* data* base*"},
	}

	for _, test := range tests {
		result := prepareFTS5Query(test.input)
		if result != test.expected {
			t.Errorf("prepareFTS5Query(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestPrepareFTS5Query_QuotedString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello world"`, `"hello world"`},
		{`"exact phrase"`, `"exact phrase"`},
		{`"test query"`, `"test query"`},
	}

	for _, test := range tests {
		result := prepareFTS5Query(test.input)
		if result != test.expected {
			t.Errorf("prepareFTS5Query(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestPrepareFTS5Query_BooleanOperators(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello AND world", "hello AND world"},
		{"test OR example", "test OR example"},
		{"golang NOT python", "golang NOT python"},
		{"first AND second OR third", "first AND second OR third"},
	}

	for _, test := range tests {
		result := prepareFTS5Query(test.input)
		if result != test.expected {
			t.Errorf("prepareFTS5Query(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestPrepareFTS5Query_ExistingWildcards(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello*", "hello*"},
		{"test* world", "test* world*"},
		{"prefix* suffix", "prefix* suffix*"},
	}

	for _, test := range tests {
		result := prepareFTS5Query(test.input)
		if result != test.expected {
			t.Errorf("prepareFTS5Query(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestPrepareFTS5Query_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello  world  ", "hello* world*"},
		{"\t\ntest\t\n", "test*"},
		{"   ", ""},
	}

	for _, test := range tests {
		result := prepareFTS5Query(test.input)
		if result != test.expected {
			t.Errorf("prepareFTS5Query(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}
