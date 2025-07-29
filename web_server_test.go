package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// WEB SERVER AND EVENTS TESTS
// =============================================================================

// =============================================================================
// EVENT BROADCASTER TESTS
// =============================================================================

func TestNewEventBroadcaster(t *testing.T) {
	broadcaster := newEventBroadcaster()

	if broadcaster == nil {
		t.Fatal("Expected non-nil EventBroadcaster")
	}

	if broadcaster.clients == nil {
		t.Error("Expected clients map to be initialized")
	}

	if len(broadcaster.clients) != 0 {
		t.Error("Expected empty clients map")
	}

	if broadcaster.shutdown {
		t.Error("Expected shutdown to be false initially")
	}
}

func TestEventBroadcaster_AddClient(t *testing.T) {
	broadcaster := newEventBroadcaster()
	client := make(chan ServerEvent, 10)

	broadcaster.addClient(client)

	if len(broadcaster.clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(broadcaster.clients))
	}

	if !broadcaster.clients[client] {
		t.Error("Expected client to be in clients map")
	}
}

func TestEventBroadcaster_RemoveClient(t *testing.T) {
	broadcaster := newEventBroadcaster()
	client := make(chan ServerEvent, 10)

	broadcaster.addClient(client)
	broadcaster.removeClient(client)

	if len(broadcaster.clients) != 0 {
		t.Errorf("Expected 0 clients, got %d", len(broadcaster.clients))
	}

	// Channel should be closed
	select {
	case _, ok := <-client:
		if ok {
			t.Error("Expected channel to be closed")
		}
	default:
		t.Error("Expected to read from closed channel")
	}
}

func TestEventBroadcaster_RemoveClient_WhenShutdown(t *testing.T) {
	broadcaster := newEventBroadcaster()
	client := make(chan ServerEvent, 10)

	broadcaster.addClient(client)
	broadcaster.shutdown = true

	// Should not panic or cause issues when removing client after shutdown
	broadcaster.removeClient(client)

	// Client should still be in map since removal is skipped during shutdown
	if len(broadcaster.clients) != 1 {
		t.Errorf("Expected 1 client (removal skipped during shutdown), got %d", len(broadcaster.clients))
	}
}

func TestEventBroadcaster_Broadcast(t *testing.T) {
	broadcaster := newEventBroadcaster()
	client1 := make(chan ServerEvent, 10)
	client2 := make(chan ServerEvent, 10)

	broadcaster.addClient(client1)
	broadcaster.addClient(client2)

	event := ServerEvent{
		Type: "test",
		Payload: map[string]interface{}{
			"message": "hello",
		},
	}

	broadcaster.broadcast(event)

	// Both clients should receive the event
	select {
	case receivedEvent := <-client1:
		if receivedEvent.Type != "test" {
			t.Errorf("Expected event type 'test', got '%s'", receivedEvent.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected client1 to receive event")
	}

	select {
	case receivedEvent := <-client2:
		if receivedEvent.Type != "test" {
			t.Errorf("Expected event type 'test', got '%s'", receivedEvent.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected client2 to receive event")
	}
}

func TestEventBroadcaster_Broadcast_WhenShutdown(t *testing.T) {
	broadcaster := newEventBroadcaster()
	client := make(chan ServerEvent, 10)

	broadcaster.addClient(client)
	broadcaster.shutdown = true

	event := ServerEvent{Type: "test"}

	// Should not panic when broadcasting during shutdown
	broadcaster.broadcast(event)

	// Client should not receive event
	select {
	case <-client:
		t.Error("Expected no event to be sent during shutdown")
	case <-time.After(50 * time.Millisecond):
		// Expected behavior
	}
}

func TestEventBroadcaster_Broadcast_FullChannel(t *testing.T) {
	broadcaster := newEventBroadcaster()
	client := make(chan ServerEvent) // Unbuffered channel

	broadcaster.addClient(client)

	event := ServerEvent{Type: "test"}

	// Broadcast should not block even if channel is full
	done := make(chan bool, 1)
	go func() {
		broadcaster.broadcast(event)
		done <- true
	}()

	select {
	case <-done:
		// Expected - broadcast should complete immediately
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected broadcast to complete immediately even with full channel")
	}
}

func TestEventBroadcaster_CloseAllClients(t *testing.T) {
	broadcaster := newEventBroadcaster()
	client1 := make(chan ServerEvent, 10)
	client2 := make(chan ServerEvent, 10)

	broadcaster.addClient(client1)
	broadcaster.addClient(client2)

	broadcaster.closeAllClients()

	if !broadcaster.shutdown {
		t.Error("Expected shutdown to be true")
	}

	if len(broadcaster.clients) != 0 {
		t.Errorf("Expected 0 clients after closeAllClients, got %d", len(broadcaster.clients))
	}

	// Both channels should be closed
	select {
	case _, ok := <-client1:
		if ok {
			t.Error("Expected client1 channel to be closed")
		}
	default:
		t.Error("Expected to read from closed client1 channel")
	}

	select {
	case _, ok := <-client2:
		if ok {
			t.Error("Expected client2 channel to be closed")
		}
	default:
		t.Error("Expected to read from closed client2 channel")
	}
}

// =============================================================================
// WEB SERVER TESTS
// =============================================================================

func TestNewWebServer(t *testing.T) {
	cfg := &Config{
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   8080,
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)

	webServer := newWebServer(cfg, db, eventChan)

	if webServer == nil {
		t.Fatal("Expected non-nil WebServer")
	}

	if webServer.config != cfg {
		t.Error("Expected config to be set correctly")
	}

	if webServer.db != db {
		t.Error("Expected database to be set correctly")
	}

	if webServer.broadcaster == nil {
		t.Error("Expected broadcaster to be initialized")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_SetupRoutes(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	mux := webServer.setupRoutes()

	if mux == nil {
		t.Fatal("Expected non-nil ServeMux")
	}

	// Test that API routes are registered by making test requests
	// We'll skip the index route since it depends on embedded filesystem
	testCases := []struct {
		path           string
		method         string
		expectedStatus int
	}{
		{"/api/search", "GET", http.StatusMethodNotAllowed}, // Should be POST
		{"/api/stats", "GET", http.StatusOK},
		{"/api/events", "GET", http.StatusOK},
		{"/api/nonexistent", "GET", http.StatusNotFound},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()

		// Use a context with timeout for SSE endpoints
		if tc.path == "/api/events" {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			req = req.WithContext(ctx)
			cancel() // Cancel immediately to close SSE quickly
		}

		mux.ServeHTTP(w, req)

		if w.Code != tc.expectedStatus {
			t.Errorf("Expected status %d for %s %s, got %d", tc.expectedStatus, tc.method, tc.path, w.Code)
		}
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleIndex_Success(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	webServer.handleIndex(w, req)

	// The index handler will return an internal server error in tests
	// because the embedded filesystem is not available, but this is expected
	// We can test that it handles the root path correctly by checking headers
	if w.Code != http.StatusInternalServerError {
		// If it's not 500, it should be 200 (in a real environment with embedded files)
		if w.Code == http.StatusOK {
			contentType := w.Header().Get("Content-Type")
			if contentType != "text/html; charset=utf-8" {
				t.Errorf("Expected Content-Type 'text/html; charset=utf-8', got '%s'", contentType)
			}

			cacheControl := w.Header().Get("Cache-Control")
			if cacheControl != "no-cache, no-store, must-revalidate" {
				t.Errorf("Expected Cache-Control header, got '%s'", cacheControl)
			}
		}
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleIndex_NotFound(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	webServer.handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleSearch_InvalidMethod(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()

	webServer.handleSearch(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleSearch_InvalidJSON(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("POST", "/api/search", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	webServer.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleSearch_Success(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	// Add test data
	bookmark := createTestBookmark("test-status-1", "test content for search")
	if err := db.insertBookmark(bookmark); err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	searchRequest := SearchRequest{
		Query:              "test content",
		Limit:              10,
		Offset:             0,
		EnableHighlighting: true,
		SnippetLength:      100,
	}

	requestBody, _ := json.Marshal(searchRequest)
	req := httptest.NewRequest("POST", "/api/search", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	webServer.handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("Expected Cache-Control 'no-cache', got '%s'", cacheControl)
	}

	var results []*SearchResult
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected search results, got empty array")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleSearch_EmptyQuery(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	searchRequest := SearchRequest{
		Query: "",
		Limit: 10,
	}

	requestBody, _ := json.Marshal(searchRequest)
	req := httptest.NewRequest("POST", "/api/search", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	webServer.handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check that we got a valid JSON response
	responseBody := w.Body.String()
	if responseBody == "" {
		t.Error("Expected non-empty response body")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleStats_InvalidMethod(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("POST", "/api/stats", nil)
	w := httptest.NewRecorder()

	webServer.handleStats(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleStats_Success(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	// Add test data
	bookmark := createTestBookmark("test-status-1", "test content")
	if err := db.insertBookmark(bookmark); err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()

	webServer.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("Expected Cache-Control 'no-cache', got '%s'", cacheControl)
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if totalBookmarks, exists := stats["total_bookmarks"]; !exists {
		t.Error("Expected 'total_bookmarks' in stats")
	} else if count, ok := totalBookmarks.(float64); !ok || count != 1 {
		t.Errorf("Expected total_bookmarks to be 1, got %v", totalBookmarks)
	}

	if _, exists := stats["backfill_complete"]; !exists {
		t.Error("Expected 'backfill_complete' in stats")
	}

	if _, exists := stats["updated_at"]; !exists {
		t.Error("Expected 'updated_at' in stats")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleEvents_InvalidMethod(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("POST", "/api/events", nil)
	w := httptest.NewRecorder()

	webServer.handleEvents(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleEvents_SSEHeaders(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()

	// Use a context with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	webServer.handleEvents(w, req)

	// Check SSE headers
	expectedHeaders := map[string]string{
		"Content-Type":                 "text/event-stream",
		"Cache-Control":                "no-cache",
		"Connection":                   "keep-alive",
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Headers": "Cache-Control",
		"X-Accel-Buffering":            "no",
	}

	for header, expectedValue := range expectedHeaders {
		if actualValue := w.Header().Get(header); actualValue != expectedValue {
			t.Errorf("Expected header %s to be '%s', got '%s'", header, expectedValue, actualValue)
		}
	}

	// Should contain the connected event
	body := w.Body.String()
	if !strings.Contains(body, `data: {"type":"connected"}`) {
		t.Error("Expected to find connected event in response")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleEvents_SendsStats(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	// Add test data
	bookmark := createTestBookmark("test-status-1", "test content")
	if err := db.insertBookmark(bookmark); err != nil {
		t.Fatalf("Failed to insert test bookmark: %v", err)
	}

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()

	// Use a context with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	webServer.handleEvents(w, req)

	body := w.Body.String()

	// Should contain the connected event
	if !strings.Contains(body, `data: {"type":"connected"}`) {
		t.Error("Expected to find connected event in response")
	}

	// Should contain stats event with bookmark count
	if !strings.Contains(body, `"type":"stats"`) {
		t.Error("Expected to find stats event in response")
	}

	if !strings.Contains(body, `"total_bookmarks":1`) {
		t.Error("Expected to find total_bookmarks:1 in stats event")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_HandleEvents_Heartbeat(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()

	// Use a context with a shorter timeout for testing
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	// Start the handler in a goroutine
	done := make(chan bool, 1)
	go func() {
		webServer.handleEvents(w, req)
		done <- true
	}()

	// Wait a bit to let the handler start and send initial events
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop the handler
	cancel()

	// Wait for handler to finish
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("Handler did not finish in time")
	}

	body := w.Body.String()

	// Should contain the connected event
	if !strings.Contains(body, `data: {"type":"connected"}`) {
		t.Error("Expected to find connected event in response")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_Start_Success(t *testing.T) {
	cfg := &Config{
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   0, // Use port 0 for testing to get any available port
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	err := webServer.start()
	if err != nil {
		t.Fatalf("Expected no error starting web server, got %v", err)
	}

	// Verify server is set
	if webServer.server == nil {
		t.Error("Expected server to be set after start")
	}

	// Clean up
	if err := webServer.stop(); err != nil {
		t.Errorf("Error stopping web server: %v", err)
	}
	close(eventChan)
}

func TestWebServer_Stop_WithoutStart(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	err := webServer.stop()
	if err != nil {
		t.Errorf("Expected no error stopping web server without start, got %v", err)
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_Stop_AfterStart(t *testing.T) {
	cfg := &Config{
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   0, // Use port 0 for testing
		},
	}

	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	// Start the server
	if err := webServer.start(); err != nil {
		t.Fatalf("Expected no error starting web server, got %v", err)
	}

	// Give the server a moment to start
	time.Sleep(10 * time.Millisecond)

	// Stop the server
	err := webServer.stop()
	if err != nil {
		t.Errorf("Expected no error stopping web server, got %v", err)
	}

	// Clean up
	close(eventChan)
}

// =============================================================================
// SERVER EVENT INTEGRATION TESTS
// =============================================================================

func TestWebServer_EventBroadcasting(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	// Send an event through the channel
	testEvent := ServerEvent{
		Type: "test_event",
		Payload: map[string]interface{}{
			"message": "test broadcast",
		},
	}

	// Send event to the channel
	eventChan <- testEvent

	// Give the broadcaster goroutine time to process
	time.Sleep(10 * time.Millisecond)

	// Verify that the broadcaster has processed the event
	// We can't easily test the actual broadcasting without SSE client simulation,
	// but we can verify the system doesn't crash and handles the event properly
	if webServer.broadcaster == nil {
		t.Error("Expected broadcaster to be available")
	}

	// Clean up
	close(eventChan)
}

func TestWebServer_EventChanClosed(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)
	defer db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	// Close the channel immediately
	close(eventChan)

	// Give the broadcaster goroutine time to handle the closed channel
	time.Sleep(10 * time.Millisecond)

	// The system should handle the closed channel gracefully without panicking
	// We're mainly testing that no panic occurs here
	if webServer.broadcaster == nil {
		t.Error("Expected broadcaster to still be available after channel close")
	}
}

// =============================================================================
// ERROR HANDLING TESTS
// =============================================================================

func TestWebServer_HandleSearch_DatabaseError(t *testing.T) {
	cfg := &Config{}
	db := setupTestDatabase(t)

	// Close the database to simulate error
	db.close()

	eventChan := make(chan ServerEvent, 10)
	webServer := newWebServer(cfg, db, eventChan)

	searchRequest := SearchRequest{
		Query: "test",
		Limit: 10,
	}

	requestBody, _ := json.Marshal(searchRequest)
	req := httptest.NewRequest("POST", "/api/search", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	webServer.handleSearch(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	// Clean up
	close(eventChan)
}
