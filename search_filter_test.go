package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// TEST HELPERS AND SETUP
// =============================================================================

func setupFilterTestDatabase(t *testing.T) *Database {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_filter.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false, // Use normal mode for tests
			BusyTimeout: "1s",
		},
	}

	db, err := newDatabase(cfg)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return db
}

func createTestAccount() UserAccount {
	return UserAccount{
		AccountID:   "test-user-123",
		Username:    "testuser",
		DisplayName: "Test User",
		Acct:        "testuser@mastodon.social",
		Avatar:      "https://mastodon.social/avatar.jpg",
	}
}

func createTestBookmarkWithAccount(statusID, content, accountID, username string) *DBBookmark {
	now := time.Now()

	// Create realistic raw JSON with account information
	accountData := map[string]interface{}{
		"id":         statusID,
		"created_at": now.Add(-time.Hour).Format(time.RFC3339),
		"status": map[string]interface{}{
			"id":         statusID,
			"content":    content,
			"created_at": now.Add(-time.Hour).Format(time.RFC3339),
			"account": map[string]interface{}{
				"id":           accountID,
				"username":     username,
				"display_name": username + " Display",
				"avatar":       "https://mastodon.social/avatar.jpg",
			},
		},
	}

	rawJSON, err := json.Marshal(accountData)
	if err != nil {
		rawJSON = []byte(fmt.Sprintf(`{"id":"%s","status":{"account":{"id":"%s","username":"%s"}}}`,
			statusID, accountID, username))
	}

	return &DBBookmark{
		StatusID:     statusID,
		CreatedAt:    now.Add(-time.Hour),
		BookmarkedAt: now,
		SearchText:   content,
		RawJSON:      string(rawJSON),
		AccountID:    accountID, // Direct account ID storage
	}
}

// =============================================================================
// USER ACCOUNT STORAGE TESTS
// =============================================================================

func TestDatabase_InsertUserAccount_Success(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	account := createTestAccount()

	err := db.insertUserAccount(&account)
	if err != nil {
		t.Fatalf("Expected successful user account insertion, got error: %v", err)
	}

	// Verify account was inserted
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM user_account WHERE account_id = ?", account.AccountID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to verify account insertion: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 user account, found %d", count)
	}
}

func TestDatabase_InsertUserAccount_Replacement(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	account := createTestAccount()

	// Insert initial account
	err := db.insertUserAccount(&account)
	if err != nil {
		t.Fatalf("Failed to insert initial account: %v", err)
	}

	// Update account with new data
	account.DisplayName = "Updated Display Name"
	account.Avatar = "https://mastodon.social/new-avatar.jpg"

	err = db.insertUserAccount(&account)
	if err != nil {
		t.Fatalf("Expected successful account replacement, got error: %v", err)
	}

	// Verify only one account exists and data was updated
	var count int
	var displayName string
	err = db.db.QueryRow("SELECT COUNT(*), display_name FROM user_account WHERE account_id = ?",
		account.AccountID).Scan(&count, &displayName)
	if err != nil {
		t.Errorf("Failed to verify account update: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 account after replacement, found %d", count)
	}
	if displayName != "Updated Display Name" {
		t.Errorf("Expected updated display name, got '%s'", displayName)
	}
}

func TestDatabase_GetUserAccount_Found(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	originalAccount := createTestAccount()

	// Insert account
	err := db.insertUserAccount(&originalAccount)
	if err != nil {
		t.Fatalf("Failed to insert test account: %v", err)
	}

	// Retrieve account
	retrievedAccount, err := db.getUserAccount()
	if err != nil {
		t.Fatalf("Expected successful account retrieval, got error: %v", err)
	}

	if retrievedAccount == nil {
		t.Fatal("Expected non-nil account")
	}

	if retrievedAccount.AccountID != originalAccount.AccountID {
		t.Errorf("Expected account_id '%s', got '%s'", originalAccount.AccountID, retrievedAccount.AccountID)
	}

	if retrievedAccount.Username != originalAccount.Username {
		t.Errorf("Expected username '%s', got '%s'", originalAccount.Username, retrievedAccount.Username)
	}
}

func TestDatabase_GetUserAccount_NotFound(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	retrievedAccount, err := db.getUserAccount()
	if err != nil {
		t.Errorf("Expected no error for missing account, got: %v", err)
	}

	if retrievedAccount != nil {
		t.Error("Expected nil account when no account exists")
	}
}

// =============================================================================
// SEARCH FILTERING TESTS
// =============================================================================

func TestDatabase_SearchBookmarksWithAccountFilter_AllPosts(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	// Insert user account
	userAccount := createTestAccount()
	err := db.insertUserAccount(&userAccount)
	if err != nil {
		t.Fatalf("Failed to insert user account: %v", err)
	}

	// Insert bookmarks from different users
	bookmarks := []*DBBookmark{
		createTestBookmarkWithAccount("status-1", "golang programming tutorial", "test-user-123", "testuser"),
		createTestBookmarkWithAccount("status-2", "python web development", "other-user-456", "otheruser"),
		createTestBookmarkWithAccount("status-3", "golang database operations", "test-user-123", "testuser"),
		createTestBookmarkWithAccount("status-4", "golang best practices", "another-user-789", "anotheruser"),
	}

	for _, bookmark := range bookmarks {
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %s: %v", bookmark.StatusID, err)
		}
	}

	request := &SearchRequest{
		Query:           "golang",
		Limit:           10,
		FilterByAccount: "all",
	}

	results, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Fatalf("Expected successful search with all filter, got error: %v", err)
	}

	// Should find all 3 golang-related bookmarks regardless of author
	if len(results) != 3 {
		t.Errorf("Expected 3 results for 'golang' search with all filter, got %d", len(results))
	}

	// Verify all expected status IDs are present
	foundStatusIDs := make(map[string]bool)
	for _, result := range results {
		foundStatusIDs[result.Bookmark.StatusID] = true
	}

	expectedIDs := []string{"status-1", "status-3", "status-4"}
	for _, expectedID := range expectedIDs {
		if !foundStatusIDs[expectedID] {
			t.Errorf("Expected to find status_id %s in all posts results", expectedID)
		}
	}
}

func TestDatabase_SearchBookmarksWithAccountFilter_MyPosts(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	// Insert user account
	userAccount := createTestAccount()
	err := db.insertUserAccount(&userAccount)
	if err != nil {
		t.Fatalf("Failed to insert user account: %v", err)
	}

	// Insert bookmarks from different users
	bookmarks := []*DBBookmark{
		createTestBookmarkWithAccount("status-1", "golang programming tutorial", "test-user-123", "testuser"),
		createTestBookmarkWithAccount("status-2", "python web development", "other-user-456", "otheruser"),
		createTestBookmarkWithAccount("status-3", "golang database operations", "test-user-123", "testuser"),
		createTestBookmarkWithAccount("status-4", "golang best practices", "another-user-789", "anotheruser"),
	}

	for _, bookmark := range bookmarks {
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %s: %v", bookmark.StatusID, err)
		}
	}

	request := &SearchRequest{
		Query:           "golang",
		Limit:           10,
		FilterByAccount: "my_posts",
	}

	results, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Fatalf("Expected successful search with my_posts filter, got error: %v", err)
	}

	// Should find only 2 golang-related bookmarks from the current user
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'golang' search with my_posts filter, got %d", len(results))
	}

	// Verify only current user's posts are returned
	for _, result := range results {
		var accountData map[string]interface{}
		err := json.Unmarshal([]byte(result.Bookmark.RawJSON), &accountData)
		if err != nil {
			t.Errorf("Failed to parse raw JSON: %v", err)
			continue
		}

		if status, ok := accountData["status"].(map[string]interface{}); ok {
			if account, ok := status["account"].(map[string]interface{}); ok {
				if accountID, ok := account["id"].(string); ok {
					if accountID != userAccount.AccountID {
						t.Errorf("Expected only current user's posts, found post from account %s", accountID)
					}
				}
			}
		}
	}

	// Verify expected status IDs are present
	foundStatusIDs := make(map[string]bool)
	for _, result := range results {
		foundStatusIDs[result.Bookmark.StatusID] = true
	}

	expectedIDs := []string{"status-1", "status-3"}
	for _, expectedID := range expectedIDs {
		if !foundStatusIDs[expectedID] {
			t.Errorf("Expected to find status_id %s in my_posts results", expectedID)
		}
	}
}

func TestDatabase_SearchBookmarksWithAccountFilter_EmptyQuery(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	// Insert user account
	userAccount := createTestAccount()
	err := db.insertUserAccount(&userAccount)
	if err != nil {
		t.Fatalf("Failed to insert user account: %v", err)
	}

	request := &SearchRequest{
		Query:           "",
		Limit:           10,
		FilterByAccount: "my_posts",
	}

	results, err := db.searchBookmarksWithFTS5(request)
	if err != nil {
		t.Errorf("Expected no error for empty query with filter, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results for empty query, got %d results", len(results))
	}
}

func TestDatabase_GetRecentBookmarksWithAccountFilter_AllPosts(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	// Insert user account
	userAccount := createTestAccount()
	err := db.insertUserAccount(&userAccount)
	if err != nil {
		t.Fatalf("Failed to insert user account: %v", err)
	}

	// Insert bookmarks with different timestamps
	now := time.Now()
	bookmarks := []*DBBookmark{
		{
			StatusID:     "status-1",
			CreatedAt:    now.Add(-3 * time.Hour),
			BookmarkedAt: now.Add(-1 * time.Hour),
			SearchText:   "recent bookmark 1",
			RawJSON:      fmt.Sprintf(`{"status":{"account":{"id":"%s","username":"%s"}}}`, userAccount.AccountID, userAccount.Username),
			AccountID:    userAccount.AccountID,
		},
		{
			StatusID:     "status-2",
			CreatedAt:    now.Add(-2 * time.Hour),
			BookmarkedAt: now.Add(-2 * time.Hour),
			SearchText:   "recent bookmark 2",
			RawJSON:      `{"status":{"account":{"id":"other-user-456","username":"otheruser"}}}`,
			AccountID:    "other-user-456",
		},
		{
			StatusID:     "status-3",
			CreatedAt:    now.Add(-1 * time.Hour),
			BookmarkedAt: now.Add(-30 * time.Minute),
			SearchText:   "recent bookmark 3",
			RawJSON:      `{"status":{"account":{"id":"another-user-789","username":"anotheruser"}}}`,
			AccountID:    "another-user-789",
		},
	}

	for _, bookmark := range bookmarks {
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %s: %v", bookmark.StatusID, err)
		}
	}

	results, err := db.getRecentBookmarks(10, 0, "all")
	if err != nil {
		t.Fatalf("Expected successful recent bookmarks retrieval with all filter, got error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 recent bookmarks with all filter, got %d", len(results))
	}

	// Verify order is by bookmarked_at DESC
	expectedOrder := []string{"status-3", "status-1", "status-2"}
	for i, result := range results {
		if result.Bookmark.StatusID != expectedOrder[i] {
			t.Errorf("Expected bookmark %d to have status_id %s, got %s",
				i, expectedOrder[i], result.Bookmark.StatusID)
		}
	}
}

func TestDatabase_GetRecentBookmarksWithAccountFilter_MyPosts(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	// Insert user account
	userAccount := createTestAccount()
	err := db.insertUserAccount(&userAccount)
	if err != nil {
		t.Fatalf("Failed to insert user account: %v", err)
	}

	// Insert bookmarks with different timestamps and authors
	now := time.Now()
	bookmarks := []*DBBookmark{
		{
			StatusID:     "status-1",
			CreatedAt:    now.Add(-3 * time.Hour),
			BookmarkedAt: now.Add(-1 * time.Hour),
			SearchText:   "my recent bookmark 1",
			RawJSON:      fmt.Sprintf(`{"status":{"account":{"id":"%s","username":"%s"}}}`, userAccount.AccountID, userAccount.Username),
			AccountID:    userAccount.AccountID,
		},
		{
			StatusID:     "status-2",
			CreatedAt:    now.Add(-2 * time.Hour),
			BookmarkedAt: now.Add(-2 * time.Hour),
			SearchText:   "other user bookmark",
			RawJSON:      `{"status":{"account":{"id":"other-user-456","username":"otheruser"}}}`,
			AccountID:    "other-user-456",
		},
		{
			StatusID:     "status-3",
			CreatedAt:    now.Add(-1 * time.Hour),
			BookmarkedAt: now.Add(-30 * time.Minute),
			SearchText:   "my recent bookmark 2",
			RawJSON:      fmt.Sprintf(`{"status":{"account":{"id":"%s","username":"%s"}}}`, userAccount.AccountID, userAccount.Username),
			AccountID:    userAccount.AccountID,
		},
	}

	for _, bookmark := range bookmarks {
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark %s: %v", bookmark.StatusID, err)
		}
	}

	results, err := db.getRecentBookmarks(10, 0, "my_posts")
	if err != nil {
		t.Fatalf("Expected successful recent bookmarks retrieval with my_posts filter, got error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 recent bookmarks with my_posts filter, got %d", len(results))
	}

	// Verify only current user's posts are returned and in correct order
	expectedOrder := []string{"status-3", "status-1"}
	for i, result := range results {
		if result.Bookmark.StatusID != expectedOrder[i] {
			t.Errorf("Expected bookmark %d to have status_id %s, got %s",
				i, expectedOrder[i], result.Bookmark.StatusID)
		}

		// Verify account ownership
		var accountData map[string]interface{}
		err := json.Unmarshal([]byte(result.Bookmark.RawJSON), &accountData)
		if err != nil {
			t.Errorf("Failed to parse raw JSON: %v", err)
			continue
		}

		if status, ok := accountData["status"].(map[string]interface{}); ok {
			if account, ok := status["account"].(map[string]interface{}); ok {
				if accountID, ok := account["id"].(string); ok {
					if accountID != userAccount.AccountID {
						t.Errorf("Expected only current user's posts, found post from account %s", accountID)
					}
				}
			}
		}
	}
}

// =============================================================================
// UNIFIED SEARCH WITH FILTERING TESTS
// =============================================================================

// testSearchOrRecentBookmarksWithAccountFilter is a helper function to reduce code duplication
func testSearchOrRecentBookmarksWithAccountFilter(t *testing.T, query string, content1, content2 string, expectedRankCheck func(float64) bool, rankDescription string) {
	t.Helper()

	db := setupFilterTestDatabase(t)
	defer db.close()

	// Insert user account
	userAccount := createTestAccount()
	err := db.insertUserAccount(&userAccount)
	if err != nil {
		t.Fatalf("Failed to insert user account: %v", err)
	}

	// Insert test bookmarks
	bookmarks := []*DBBookmark{
		createTestBookmarkWithAccount("status-1", content1, userAccount.AccountID, userAccount.Username),
		createTestBookmarkWithAccount("status-2", content2, "other-user-456", "otheruser"),
	}

	for _, bookmark := range bookmarks {
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert test bookmark: %v", err)
		}
	}

	request := &SearchRequest{
		Query:           query,
		Limit:           10,
		FilterByAccount: "my_posts",
	}

	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Fatalf("Expected successful search with filter, got error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 search result with my_posts filter, got %d", len(results))
	}

	// Check rank according to expected criteria
	if !expectedRankCheck(results[0].Rank) {
		t.Errorf("Expected %s, got %f", rankDescription, results[0].Rank)
	}

	// Verify it's the correct user's post
	if results[0].Bookmark.StatusID != "status-1" {
		t.Errorf("Expected status-1 in results, got %s", results[0].Bookmark.StatusID)
	}
}

func TestDatabase_SearchOrRecentBookmarksWithAccountFilter_WithQuery(t *testing.T) {
	testSearchOrRecentBookmarksWithAccountFilter(t,
		"golang",
		"golang programming tutorial",
		"golang web development",
		func(rank float64) bool { return rank < 0 },
		"negative BM25 rank from FTS search")
}

func TestDatabase_SearchOrRecentBookmarksWithAccountFilter_EmptyQuery(t *testing.T) {
	testSearchOrRecentBookmarksWithAccountFilter(t,
		"",
		"test content 1",
		"test content 2",
		func(rank float64) bool { return rank == 0.0 },
		"rank 0.0 from recent bookmarks")
}

// =============================================================================
// ACCOUNT DATA EXTRACTION TESTS
// =============================================================================

// =============================================================================
// MIGRATION TESTS
// =============================================================================

func TestDatabase_UserAccountMigration(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	// Verify user_account table was created
	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='user_account'").Scan(&count)
	if err != nil {
		t.Errorf("Failed to check for user_account table: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected user_account table to exist, found %d tables with that name", count)
	}

	// Verify table structure
	rows, err := db.db.Query("PRAGMA table_info(user_account)")
	if err != nil {
		t.Fatalf("Failed to get table info: %v", err)
	}
	defer rows.Close()

	expectedColumns := map[string]bool{
		"id":           false,
		"account_id":   false,
		"username":     false,
		"display_name": false,
		"acct":         false,
		"avatar":       false,
		"created_at":   false,
		"updated_at":   false,
	}

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue *string
		var pk int

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			t.Errorf("Failed to scan column info: %v", err)
			continue
		}

		if _, expected := expectedColumns[name]; expected {
			expectedColumns[name] = true
		}
	}

	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		t.Errorf("Error during rows iteration: %v", err)
	}

	for column, found := range expectedColumns {
		if !found {
			t.Errorf("Expected column %s not found in user_account table", column)
		}
	}
}

// =============================================================================
// ERROR HANDLING TESTS
// =============================================================================

func TestDatabase_SearchWithAccountFilter_NoUserAccount(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	// Don't insert user account - should handle gracefully
	bookmark := createTestBookmarkWithAccount("status-1", "test content", "user-123", "testuser")
	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	request := &SearchRequest{
		Query:           "test",
		Limit:           10,
		FilterByAccount: "my_posts",
	}

	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Fatalf("Expected search to handle missing user account gracefully, got error: %v", err)
	}

	// Should return empty results when no user account is configured
	if len(results) != 0 {
		t.Errorf("Expected 0 results when no user account configured, got %d", len(results))
	}
}

func TestDatabase_SearchWithAccountFilter_InvalidFilter(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	request := &SearchRequest{
		Query:           "test",
		Limit:           10,
		FilterByAccount: "invalid_filter",
	}

	// Should default to "all" behavior for invalid filter values
	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Errorf("Expected search to handle invalid filter gracefully, got error: %v", err)
	}

	// Should not error, just treat as "all"
	if results == nil {
		t.Errorf("Expected non-nil results for invalid filter")
	}
}

func TestDatabase_SearchWithAccountFilter_EmptyFilter(t *testing.T) {
	db := setupFilterTestDatabase(t)
	defer db.close()

	request := &SearchRequest{
		Query:           "test",
		Limit:           10,
		FilterByAccount: "", // Empty should default to "all"
	}

	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Errorf("Expected search to handle empty filter gracefully, got error: %v", err)
	}

	// Should not error, just treat as "all"
	if results == nil {
		t.Errorf("Expected non-nil results for empty filter")
	}
}

// =============================================================================
// PERFORMANCE TESTS
// =============================================================================

func TestDatabase_SearchWithAccountFilter_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	db := setupFilterTestDatabase(t)
	defer db.close()

	// Insert user account
	userAccount := createTestAccount()
	err := db.insertUserAccount(&userAccount)
	if err != nil {
		t.Fatalf("Failed to insert user account: %v", err)
	}

	// Insert many bookmarks for performance testing
	numBookmarks := 1000
	for i := 0; i < numBookmarks; i++ {
		accountID := userAccount.AccountID
		username := userAccount.Username

		// Mix of user's posts and others
		if i%3 == 0 {
			accountID = fmt.Sprintf("other-user-%d", i)
			username = fmt.Sprintf("otheruser%d", i)
		}

		bookmark := createTestBookmarkWithAccount(
			fmt.Sprintf("status-%d", i),
			fmt.Sprintf("performance test content %d golang programming", i),
			accountID,
			username,
		)

		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert performance test bookmark %d: %v", i, err)
		}
	}

	// Test search performance with filtering
	start := time.Now()

	request := &SearchRequest{
		Query:           "golang",
		Limit:           50,
		FilterByAccount: "my_posts",
	}

	results, err := db.searchOrRecentBookmarks(request)
	if err != nil {
		t.Fatalf("Performance test search failed: %v", err)
	}

	duration := time.Since(start)

	// Should complete within reasonable time (adjust threshold as needed)
	if duration > 500*time.Millisecond {
		t.Errorf("Search with account filter took too long: %v", duration)
	}

	// Should find approximately 2/3 of matching results (user's posts only)
	// This is a rough estimate since we're searching for "golang" which appears in all
	expectedRange := [2]int{600, 800} // Approximately 2/3 of 1000, accounting for search relevance
	if len(results) < expectedRange[0] || len(results) > expectedRange[1] {
		t.Logf("Performance test found %d results in %v", len(results), duration)
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

func TestMastodonClientIntegration_StoreAccountData(t *testing.T) {
	// This test would require actual Mastodon credentials
	// In practice, this would be tested with a mock or test server
	t.Skip("Integration test requires live Mastodon credentials")

	// Placeholder for actual integration test:
	/*
		cfg := &Config{
			Mastodon: struct {
				Server        string `toml:"server"`
				AccessToken   string `toml:"access_token"`
				ClientTimeout string `toml:"client_timeout"`
			}{
				Server:      "https://mastodon.social",
				AccessToken: "test-token",
			},
		}

		client, err := newMastodonClient(cfg)
		if err != nil {
			t.Fatalf("Failed to create Mastodon client: %v", err)
		}

		// Test account data retrieval and storage
		account, err := client.getCurrentAccountInfo()
		if err != nil {
			t.Fatalf("Failed to get current account info: %v", err)
		}

		// Verify account data structure
		if account.AccountID == "" {
			t.Error("Expected non-empty account ID")
		}
		if account.Username == "" {
			t.Error("Expected non-empty username")
		}
	*/
}
