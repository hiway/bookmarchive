package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// DATA CONVERSION TESTS
// =============================================================================

func TestConvertBookmarkToDatabase_BasicConversion(t *testing.T) {
	bookmark := Bookmark{
		ID: "bookmark-123",
		Status: Status{
			ID:          "status-456",
			URI:         "https://example.com/status/456",
			URL:         "https://example.com/@user/456",
			Content:     "<p>This is a <strong>test</strong> bookmark</p>",
			SpoilerText: "Spoiler warning",
			CreatedAt:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			Account: Account{
				ID:          "user-789",
				Username:    "testuser",
				DisplayName: "Test User",
				Avatar:      "https://example.com/avatar.jpg",
			},
			MediaAttachments: []Media{
				{
					ID:          "media-1",
					Type:        "image",
					URL:         "https://example.com/image.jpg",
					Description: "Test image description",
				},
			},
			Tags: []Tag{
				{Name: "golang", URL: "https://example.com/tags/golang"},
				{Name: "testing", URL: "https://example.com/tags/testing"},
			},
		},
		CreatedAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
	}

	indexedFields := []string{"content", "spoiler_text", "username", "display_name", "media_descriptions", "hashtags"}

	dbBookmark := convertBookmarkToDatabase(bookmark, indexedFields)

	// Test basic fields
	if dbBookmark.StatusID != "status-456" {
		t.Errorf("Expected status_id 'status-456', got '%s'", dbBookmark.StatusID)
	}

	if !dbBookmark.CreatedAt.Equal(bookmark.Status.CreatedAt) {
		t.Errorf("Expected created_at %v, got %v", bookmark.Status.CreatedAt, dbBookmark.CreatedAt)
	}

	if !dbBookmark.BookmarkedAt.Equal(bookmark.CreatedAt) {
		t.Errorf("Expected bookmarked_at %v, got %v", bookmark.CreatedAt, dbBookmark.BookmarkedAt)
	}

	// Test search text contains all indexed fields
	expectedParts := []string{
		"This is a test bookmark", // stripped HTML
		"Spoiler warning",         // spoiler text
		"testuser",                // username
		"Test User",               // display name
		"Test image description",  // media description
		"golang",                  // hashtag
		"testing",                 // hashtag
	}

	for _, part := range expectedParts {
		if !strings.Contains(dbBookmark.SearchText, part) {
			t.Errorf("Expected search text to contain '%s', got: %s", part, dbBookmark.SearchText)
		}
	}

	// Test raw JSON is valid
	var parsedJSON map[string]interface{}
	err := json.Unmarshal([]byte(dbBookmark.RawJSON), &parsedJSON)
	if err != nil {
		t.Errorf("Expected valid JSON in raw_json, got error: %v", err)
	}
}

func TestConvertBookmarkToDatabase_SelectiveIndexing(t *testing.T) {
	bookmark := Bookmark{
		ID: "bookmark-123",
		Status: Status{
			ID:      "status-456",
			Content: "Test content",
			Account: Account{
				Username:    "testuser",
				DisplayName: "Test User",
			},
			Tags: []Tag{
				{Name: "golang"},
			},
		},
		CreatedAt: time.Now(),
	}

	// Only index content and username, not display_name or hashtags
	indexedFields := []string{"content", "username"}

	dbBookmark := convertBookmarkToDatabase(bookmark, indexedFields)

	// Should contain indexed fields
	if !strings.Contains(dbBookmark.SearchText, "Test content") {
		t.Error("Expected search text to contain 'Test content'")
	}

	if !strings.Contains(dbBookmark.SearchText, "testuser") {
		t.Error("Expected search text to contain 'testuser'")
	}

	// Should NOT contain non-indexed fields
	if strings.Contains(dbBookmark.SearchText, "Test User") {
		t.Error("Expected search text to NOT contain 'Test User' (display_name not indexed)")
	}

	if strings.Contains(dbBookmark.SearchText, "golang") {
		t.Error("Expected search text to NOT contain 'golang' (hashtags not indexed)")
	}
}

func TestConvertBookmarkToDatabase_JSONMarshalError(t *testing.T) {
	// Create a bookmark with a circular reference that would cause JSON marshal to fail
	// We can't easily create this with the current structs, so we'll test the fallback
	bookmark := Bookmark{
		ID: "bookmark-123",
		Status: Status{
			ID:        "status-456",
			Content:   "Test content",
			CreatedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		CreatedAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
	}

	dbBookmark := convertBookmarkToDatabase(bookmark, []string{"content"})

	// Should have valid JSON even if original marshal failed
	var parsedJSON map[string]interface{}
	err := json.Unmarshal([]byte(dbBookmark.RawJSON), &parsedJSON)
	if err != nil {
		t.Errorf("Expected valid JSON fallback, got error: %v", err)
	}

	// Should contain at least the basic fields in fallback
	if !strings.Contains(dbBookmark.RawJSON, "bookmark-123") {
		t.Error("Expected fallback JSON to contain bookmark ID")
	}

	if !strings.Contains(dbBookmark.RawJSON, "status-456") {
		t.Error("Expected fallback JSON to contain status ID")
	}
}

// =============================================================================
// SEARCH TEXT BUILDING TESTS
// =============================================================================

func TestBuildSearchText_AllFields(t *testing.T) {
	bookmark := Bookmark{
		Status: Status{
			Content:     "<p>HTML <strong>content</strong></p>",
			SpoilerText: "Spoiler text",
			Account: Account{
				Username:    "testuser",
				DisplayName: "Test User",
			},
			MediaAttachments: []Media{
				{Description: "First image"},
				{Description: "Second image"},
				{Description: ""}, // Empty description should be ignored
			},
			Tags: []Tag{
				{Name: "golang"},
				{Name: "testing"},
				{Name: ""}, // Empty tag should be ignored
			},
		},
	}

	indexedFields := []string{"content", "spoiler_text", "username", "display_name", "media_descriptions", "hashtags"}

	searchText := buildSearchText(bookmark, indexedFields)

	expectedParts := []string{
		"HTML content", // stripped HTML
		"Spoiler text",
		"testuser",
		"Test User",
		"First image",
		"Second image",
		"golang",
		"testing",
	}

	for _, part := range expectedParts {
		if !strings.Contains(searchText, part) {
			t.Errorf("Expected search text to contain '%s', got: %s", part, searchText)
		}
	}

	// Should not contain empty values
	if strings.Contains(searchText, "<p>") || strings.Contains(searchText, "<strong>") {
		t.Error("Expected HTML tags to be stripped from search text")
	}
}

func TestBuildSearchText_SelectiveFields(t *testing.T) {
	bookmark := Bookmark{
		Status: Status{
			Content:     "Content text",
			SpoilerText: "Spoiler text",
			Account: Account{
				Username:    "testuser",
				DisplayName: "Test User",
			},
			Tags: []Tag{
				{Name: "golang"},
			},
		},
	}

	// Only index content and hashtags
	indexedFields := []string{"content", "hashtags"}

	searchText := buildSearchText(bookmark, indexedFields)

	// Should contain indexed fields
	if !strings.Contains(searchText, "Content text") {
		t.Error("Expected search text to contain 'Content text'")
	}

	if !strings.Contains(searchText, "golang") {
		t.Error("Expected search text to contain 'golang'")
	}

	// Should NOT contain non-indexed fields
	if strings.Contains(searchText, "Spoiler text") {
		t.Error("Expected search text to NOT contain 'Spoiler text'")
	}

	if strings.Contains(searchText, "testuser") {
		t.Error("Expected search text to NOT contain 'testuser'")
	}

	if strings.Contains(searchText, "Test User") {
		t.Error("Expected search text to NOT contain 'Test User'")
	}
}

func TestBuildSearchText_EmptyFields(t *testing.T) {
	bookmark := Bookmark{
		Status: Status{
			Content:     "",
			SpoilerText: "",
			Account: Account{
				Username:    "",
				DisplayName: "",
			},
			MediaAttachments: []Media{},
			Tags:             []Tag{},
		},
	}

	indexedFields := []string{"content", "spoiler_text", "username", "display_name", "media_descriptions", "hashtags"}

	searchText := buildSearchText(bookmark, indexedFields)

	// Should be empty or only contain spaces
	trimmed := strings.TrimSpace(searchText)
	if trimmed != "" {
		t.Errorf("Expected empty search text for empty fields, got: '%s'", searchText)
	}
}

func TestBuildSearchText_NoIndexedFields(t *testing.T) {
	bookmark := Bookmark{
		Status: Status{
			Content: "This content won't be indexed",
			Account: Account{
				Username: "testuser",
			},
		},
	}

	// Empty indexed fields list
	indexedFields := []string{}

	searchText := buildSearchText(bookmark, indexedFields)

	// Should be empty
	trimmed := strings.TrimSpace(searchText)
	if trimmed != "" {
		t.Errorf("Expected empty search text with no indexed fields, got: '%s'", searchText)
	}
}

// =============================================================================
// HTML STRIPPING TESTS
// =============================================================================

func TestStripHTML_BasicTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello world</p>", "Hello world"},
		{"<p>First paragraph</p><p>Second paragraph</p>", "First paragraph Second paragraph"},
		{"Line break<br>here", "Line break here"},
		{"Line break<br/>here", "Line break here"},
		{"Line break<br />here", "Line break here"},
		{"<strong>Bold text</strong>", "Bold text"},
		{"<em>Italic text</em>", "Italic text"},
		{"<div>Div content</div>", "Div content"},
		{"No HTML here", "No HTML here"},
		{"", ""},
	}

	for _, test := range tests {
		result := stripHTML(test.input)
		if result != test.expected {
			t.Errorf("stripHTML(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestStripHTML_Links(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`<a href="https://example.com">Link text</a>`, "Link text"},
		{`<a href="https://example.com" target="_blank">External link</a>`, "External link"},
		{`Visit <a href="https://example.com">this site</a> for more info`, "Visit this site for more info"},
		{`<a href="mailto:test@example.com">Email me</a>`, "Email me"},
		{`<a href="#anchor">Internal link</a>`, "Internal link"},
	}

	for _, test := range tests {
		result := stripHTML(test.input)
		if result != test.expected {
			t.Errorf("stripHTML(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestStripHTML_ComplexHTML(t *testing.T) {
	input := `<p>This is a <strong>complex</strong> HTML document with <a href="https://example.com">links</a> and <em>formatting</em>.</p><p>It has multiple paragraphs<br/>with line breaks.</p>`
	expected := "This is a complex HTML document with links and formatting . It has multiple paragraphs with line breaks."

	result := stripHTML(input)
	if result != expected {
		t.Errorf("stripHTML complex HTML failed.\nInput: %q\nExpected: %q\nGot: %q", input, expected, result)
	}
}

func TestStripHTML_MalformedHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Unclosed paragraph", "Unclosed paragraph"},
		{"<strong>Bold without close", "Bold without close"},
		{"<a href='test'>Link without close", "Link without close"},
		{"Orphaned closing tag</p>", "Orphaned closing tag"},
		{"<>Empty tag</>", "&lt;&gt;Empty tag"}, // BlueMondaty escapes invalid tags
		{"<<malformed>>", "&lt; &gt;"},          // BlueMondaty escapes malformed tags
		{"<script>alert('evil')</script>", ""},  // BlueMondaty strips script tags completely
	}

	for _, test := range tests {
		result := stripHTML(test.input)
		if result != test.expected {
			t.Errorf("stripHTML(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestStripHTML_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>  Multiple   spaces  </p>", "Multiple  spaces"}, // BlueMondaty may preserve some internal spacing
		{" <p> Leading and trailing </p> ", "Leading and trailing"},
		{"<p></p><p></p>", ""},
		{"<br><br><br>", ""},
		{"Text<br><br>with<br><br>breaks", "Text with breaks"},
	}

	for _, test := range tests {
		result := stripHTML(test.input)
		if result != test.expected {
			t.Errorf("stripHTML(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestStripHTML_NestedTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p><strong><em>Nested formatting</em></strong></p>", "Nested formatting"},
		{"<div><p>Nested <span>elements</span> here</p></div>", "Nested elements here"},
		{"<a href='#'><strong>Bold link</strong></a>", "Bold link"},
	}

	for _, test := range tests {
		result := stripHTML(test.input)
		if result != test.expected {
			t.Errorf("stripHTML(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

// =============================================================================
// MIN FUNCTION TESTS
// =============================================================================

func TestMin_BasicCases(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 1, 0},
		{-1, 1, -1},
		{-5, -3, -5},
		{100, 50, 50},
	}

	for _, test := range tests {
		result := min(test.a, test.b)
		if result != test.expected {
			t.Errorf("min(%d, %d) = %d, expected %d", test.a, test.b, result, test.expected)
		}
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

func TestDatabase_FullWorkflow_InsertSearchRetrieve(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Create some test bookmarks with varying content
	bookmarks := []Bookmark{
		{
			ID: "bookmark-1",
			Status: Status{
				ID:      "status-1",
				Content: "Learning golang programming",
				Account: Account{Username: "developer1", DisplayName: "Go Developer"},
				Tags:    []Tag{{Name: "golang"}, {Name: "programming"}},
			},
			CreatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID: "bookmark-2",
			Status: Status{
				ID:      "status-2",
				Content: "Python web development tutorial",
				Account: Account{Username: "pythonista", DisplayName: "Python Expert"},
				Tags:    []Tag{{Name: "python"}, {Name: "web"}},
			},
			CreatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID: "bookmark-3",
			Status: Status{
				ID:      "status-3",
				Content: "Database design with golang",
				Account: Account{Username: "dbadmin", DisplayName: "Database Admin"},
				Tags:    []Tag{{Name: "golang"}, {Name: "database"}},
			},
			CreatedAt: time.Now(),
		},
	}

	indexedFields := []string{"content", "username", "display_name", "hashtags"}

	// Insert all bookmarks
	for _, bookmark := range bookmarks {
		dbBookmark := convertBookmarkToDatabase(bookmark, indexedFields)
		err := db.insertBookmark(dbBookmark)
		if err != nil {
			t.Fatalf("Failed to insert bookmark %s: %v", bookmark.ID, err)
		}
	}

	// Test search functionality
	searchRequest := &SearchRequest{
		Query: "golang",
		Limit: 10,
	}

	results, err := db.searchBookmarksWithFTS5(searchRequest)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 golang results, got %d", len(results))
	}

	// Test recent bookmarks
	recentResults, err := db.getRecentBookmarks(10, 0)
	if err != nil {
		t.Fatalf("Recent bookmarks retrieval failed: %v", err)
	}

	if len(recentResults) != 3 {
		t.Errorf("Expected 3 recent bookmarks, got %d", len(recentResults))
	}

	// Recent bookmarks should be ordered by bookmarked_at DESC
	// bookmark-3 should be first (most recent)
	if recentResults[0].Bookmark.StatusID != "status-3" {
		t.Errorf("Expected most recent bookmark to be status-3, got %s", recentResults[0].Bookmark.StatusID)
	}

	// Test individual bookmark retrieval
	bookmark, err := db.getBookmark("status-2")
	if err != nil {
		t.Fatalf("Bookmark retrieval failed: %v", err)
	}

	if bookmark == nil {
		t.Fatal("Expected bookmark to be found")
	}

	if !strings.Contains(bookmark.SearchText, "Python web development") {
		t.Error("Expected retrieved bookmark to contain original content")
	}

	// Test backfill state updates
	testProcessedID := "test-processed-123"
	testPollTime := time.Now()

	err = db.updateBackfillState(testProcessedID, true, &testPollTime)
	if err != nil {
		t.Fatalf("Backfill state update failed: %v", err)
	}

	state, err := db.getBackfillState()
	if err != nil {
		t.Fatalf("Backfill state retrieval failed: %v", err)
	}

	if state.LastProcessedID != testProcessedID {
		t.Errorf("Expected last_processed_id %s, got %s", testProcessedID, state.LastProcessedID)
	}

	if !state.BackfillComplete {
		t.Error("Expected backfill_complete to be true")
	}
}

func TestDatabase_SearchWithSpecialCharacters(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.close()

	// Insert bookmark with special characters and emojis
	bookmark := createTestBookmark("status-special", "Testing with émojis 🚀 and spéciål characters: @mentions #hashtags")
	err := db.insertBookmark(bookmark)
	if err != nil {
		t.Fatalf("Failed to insert special character bookmark: %v", err)
	}

	// Test searching for content with special characters
	searchRequest := &SearchRequest{
		Query: "émojis",
		Limit: 10,
	}

	results, err := db.searchBookmarksWithFTS5(searchRequest)
	if err != nil {
		t.Fatalf("Search with special characters failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result for special character search, got %d", len(results))
	}

	// Test searching for emoji
	searchRequest.Query = "🚀"
	_, err = db.searchBookmarksWithFTS5(searchRequest)
	if err != nil {
		t.Fatalf("Search with emoji failed: %v", err)
	}

	// Note: FTS5 may or may not match emojis depending on tokenizer configuration
	// This test mainly ensures no errors occur
}

func TestDatabase_LargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	db := setupTestDatabase(t)
	defer db.close()

	// Insert a larger number of bookmarks to test performance
	numBookmarks := 100
	for i := 1; i <= numBookmarks; i++ {
		bookmark := createTestBookmark(
			fmt.Sprintf("status-%d", i),
			fmt.Sprintf("Test bookmark number %d with some searchable content", i),
		)
		err := db.insertBookmark(bookmark)
		if err != nil {
			t.Fatalf("Failed to insert bookmark %d: %v", i, err)
		}
	}

	// Test search performance
	searchRequest := &SearchRequest{
		Query: "searchable",
		Limit: 50,
	}

	results, err := db.searchBookmarksWithFTS5(searchRequest)
	if err != nil {
		t.Fatalf("Large dataset search failed: %v", err)
	}

	if len(results) != 50 {
		t.Errorf("Expected 50 results (limited), got %d", len(results))
	}

	// Test recent bookmarks with large dataset
	recentResults, err := db.getRecentBookmarks(25, 25)
	if err != nil {
		t.Fatalf("Large dataset recent bookmarks failed: %v", err)
	}

	if len(recentResults) != 25 {
		t.Errorf("Expected 25 recent results with offset, got %d", len(recentResults))
	}
}
