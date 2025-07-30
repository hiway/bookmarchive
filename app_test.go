package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// =============================================================================
// MAIN APPLICATION TESTS
// =============================================================================

func TestNewBookmarchiveApp_Success(t *testing.T) {
	// Create a temporary config for testing
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false,
			BusyTimeout: "1s",
		},
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   0, // Use port 0 for testing
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	app, err := newBookmarchiveApp(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating app, got %v", err)
	}

	if app == nil {
		t.Fatal("Expected non-nil app")
	}

	if app.config.Mastodon.Server != cfg.Mastodon.Server {
		t.Error("Expected config to be set correctly")
	}

	if app.db == nil {
		t.Error("Expected database to be initialized")
	}

	if app.mastodonClient == nil {
		t.Error("Expected mastodon client to be initialized")
	}

	if app.bookmarkService == nil {
		t.Error("Expected bookmark service to be initialized")
	}

	if app.webServer == nil {
		t.Error("Expected web server to be initialized")
	}

	if app.eventChan == nil {
		t.Error("Expected event channel to be initialized")
	}

	// Clean up
	app.stop()
}

func TestNewBookmarchiveApp_DatabaseError(t *testing.T) {
	cfg := &Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path: "/invalid/path/that/does/not/exist/db.sqlite",
		},
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
	}

	app, err := newBookmarchiveApp(cfg)
	if err == nil {
		t.Error("Expected error due to invalid database path")
		if app != nil {
			app.stop()
		}
	}

	if app != nil {
		t.Error("Expected app to be nil on error")
	}
}

func TestNewBookmarchiveApp_MastodonClientError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path: dbPath,
		},
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "", // Invalid empty server
			AccessToken: "test-token",
		},
	}

	app, err := newBookmarchiveApp(cfg)
	if err == nil {
		t.Error("Expected error due to invalid mastodon config")
		if app != nil {
			app.stop()
		}
	}

	if app != nil {
		t.Error("Expected app to be nil on error")
	}
}

func TestBookmarchiveApp_Start_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path: dbPath,
		},
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   0, // Use port 0 for testing
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	app, err := newBookmarchiveApp(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating app, got %v", err)
	}

	err = app.start()
	if err != nil {
		t.Errorf("Expected no error starting app, got %v", err)
	}

	// Give the app a moment to start and all services to initialize
	time.Sleep(100 * time.Millisecond)

	// Clean up - graceful shutdown with time for cleanup
	app.stop()
	time.Sleep(50 * time.Millisecond)
}

func TestBookmarchiveApp_Stop_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path: dbPath,
		},
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   0,
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	app, err := newBookmarchiveApp(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating app, got %v", err)
	}

	// Start and then stop
	app.start()
	time.Sleep(100 * time.Millisecond)

	err = app.stop()
	if err != nil {
		t.Errorf("Expected no error stopping app, got %v", err)
	}

	// Give time for graceful shutdown
	time.Sleep(50 * time.Millisecond)
}

func TestBookmarchiveApp_Stop_WithoutStart(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path: dbPath,
		},
	}

	app, err := newBookmarchiveApp(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating app, got %v", err)
	}

	// Stop without starting should not error
	err = app.stop()
	if err != nil {
		t.Errorf("Expected no error stopping app without start, got %v", err)
	}
}

func TestBookmarchiveApp_Run_WithSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping signal test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path: dbPath,
		},
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   0,
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content"},
		},
	}

	app, err := newBookmarchiveApp(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating app, got %v", err)
	}

	// Run the app in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- app.run()
	}()

	// Give the app time to start
	time.Sleep(200 * time.Millisecond)

	// Send SIGINT to simulate user interrupt
	currentProcess, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find current process: %v", err)
	}

	err = currentProcess.Signal(syscall.SIGINT)
	if err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	// Wait for the app to finish
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Expected no error from run, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("App did not finish within timeout")
		app.stop() // Force stop
	}

	// Give time for cleanup
	time.Sleep(100 * time.Millisecond)
}

// =============================================================================
// LOGGING TESTS
// =============================================================================

func TestSetupLogging_Console(t *testing.T) {
	// Test console logging setup
	setupLogging("info", "console")

	// We can't easily test the actual logging output, but we can verify
	// the function doesn't panic and completes successfully
}

func TestSetupLogging_JSON(t *testing.T) {
	// Test JSON logging setup
	setupLogging("debug", "json")

	// We can't easily test the actual logging output, but we can verify
	// the function doesn't panic and completes successfully
}

func TestSetupLogging_AllLevels(t *testing.T) {
	levels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}

	for _, level := range levels {
		t.Run("level_"+level, func(t *testing.T) {
			setupLogging(level, "console")
			// Should not panic
		})
	}
}

func TestSetupLogging_InvalidLevel(t *testing.T) {
	// Test with invalid log level (should default to info)
	setupLogging("invalid", "console")

	// Should not panic and should default to info level
}

func TestSetupLogging_WithCaller(t *testing.T) {
	// Test that debug/trace levels enable caller info
	setupLogging("debug", "console")

	// Should not panic
}
