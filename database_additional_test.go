package main

import (
	"path/filepath"
	"testing"
)

// =============================================================================
// DATABASE ADDITIONAL TESTS
// =============================================================================

func TestNewDatabase_WALMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_wal.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     true,  // Enable WAL mode
			BusyTimeout: "1s",
		},
	}

	db, err := newDatabase(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating database with WAL mode, got %v", err)
	}
	defer db.close()

	if db.db == nil {
		t.Error("Expected database connection to be initialized")
	}
}

func TestNewDatabase_InvalidTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_timeout.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false,
			BusyTimeout: "invalid-timeout", // Invalid timeout format
		},
	}

	db, err := newDatabase(cfg)
	if err == nil {
		t.Error("Expected error due to invalid timeout format")
		if db != nil {
			db.close()
		}
	}
}

func TestNewDatabase_NestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a nested path that doesn't exist
	dbPath := filepath.Join(tmpDir, "nested", "dir", "test.db")

	cfg := Config{
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        dbPath,
			WalMode:     false,
			BusyTimeout: "500ms",
		},
	}

	db, err := newDatabase(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating database with nested path, got %v", err)
	}
	defer db.close()

	if db.db == nil {
		t.Error("Expected database connection to be initialized")
	}
}

// =============================================================================
// BOOKMARK SERVICE ADDITIONAL TESTS
// =============================================================================

func TestNewBookmarkService_ValidConfig(t *testing.T) {
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
			Server:      "https://mastodon.example.com",
			AccessToken: "test-token",
		},
	}

	// Create database first
	db, err := newDatabase(*cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.close()

	service, err := newBookmarkService(cfg, db, nil)
	if err != nil {
		t.Fatalf("Expected no error creating bookmark service, got %v", err)
	}

	if service == nil {
		t.Error("Expected non-nil bookmark service")
	}

	if service.db != db {
		t.Error("Expected service database to match provided database")
	}
}

func TestCreateBookmarkClient_NetworkError(t *testing.T) {
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
			Server:      "https://nonexistent.invalid.domain.example",
			AccessToken: "test-token",
		},
	}

	// Create database first
	db, err := newDatabase(*cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.close()

	service, err := newBookmarkService(cfg, db, nil)
	if err != nil {
		t.Fatalf("Failed to create bookmark service: %v", err)
	}
	
	client, err := service.createBookmarkClient()
	if err == nil {
		t.Error("Expected error due to network failure")
	}

	// Should still be nil after failed creation
	if client != nil {
		t.Error("Expected bookmark client to remain nil after failed creation")
	}
}
