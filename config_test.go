package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	// Test Mastodon defaults
	if cfg.Mastodon.Server != "https://mastodon.social" {
		t.Errorf("Expected mastodon server 'https://mastodon.social', got '%s'", cfg.Mastodon.Server)
	}
	if cfg.Mastodon.AccessToken != "your-access-token-here" {
		t.Errorf("Expected mastodon access token 'your-access-token-here', got '%s'", cfg.Mastodon.AccessToken)
	}
	if cfg.Mastodon.ClientTimeout != "30s" {
		t.Errorf("Expected mastodon client timeout '30s', got '%s'", cfg.Mastodon.ClientTimeout)
	}

	// Test Database defaults
	if cfg.Database.Path != "./bookmarchive.db" {
		t.Errorf("Expected database path './bookmarchive.db', got '%s'", cfg.Database.Path)
	}
	if cfg.Database.WalMode != true {
		t.Errorf("Expected database wal_mode true, got %v", cfg.Database.WalMode)
	}
	if cfg.Database.BusyTimeout != "5s" {
		t.Errorf("Expected database busy_timeout '5s', got '%s'", cfg.Database.BusyTimeout)
	}

	// Test Polling defaults
	if cfg.Polling.Interval != "10m" {
		t.Errorf("Expected polling interval '10m', got '%s'", cfg.Polling.Interval)
	}
	if cfg.Polling.BatchSize != 20 {
		t.Errorf("Expected polling batch_size 20, got %d", cfg.Polling.BatchSize)
	}
	if cfg.Polling.BackfillDelay != "10s" {
		t.Errorf("Expected polling backfill_delay '10s', got '%s'", cfg.Polling.BackfillDelay)
	}

	// Test Web defaults
	if cfg.Web.Listen != "127.0.0.1" {
		t.Errorf("Expected web listen '127.0.0.1', got '%s'", cfg.Web.Listen)
	}
	if cfg.Web.Port != 8080 {
		t.Errorf("Expected web port 8080, got %d", cfg.Web.Port)
	}

	// Test Logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected logging level 'info', got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "console" {
		t.Errorf("Expected logging format 'console', got '%s'", cfg.Logging.Format)
	}

	// Test Search defaults
	expectedFields := []string{"content", "spoiler_text", "username", "display_name", "media_descriptions", "hashtags"}
	if !reflect.DeepEqual(cfg.Search.IndexedFields, expectedFields) {
		t.Errorf("Expected search indexed_fields %v, got %v", expectedFields, cfg.Search.IndexedFields)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	cfg := defaultConfig()
	err := loadConfig("nonexistent.toml", &cfg)

	if err == nil {
		t.Error("Expected error when config file doesn't exist")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Expected file not found error, got: %v", err)
	}
}

func TestLoadConfigCompleteFile(t *testing.T) {
	// Create a temporary config file with all values set
	configContent := `[mastodon]
server = "https://my-mastodon.example.com"
access_token = "my-secret-token"
client_timeout = "45s"

[database]
path = "/tmp/test.db"
wal_mode = false
busy_timeout = "30s"

[web]
listen = "0.0.0.0"
port = 3000

[polling]
interval = "2m"
batch_size = 50
backfill_delay = "5s"

[logging]
level = "debug"
format = "json"

[search]
indexed_fields = ["content", "username"]
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify all values were loaded correctly
	if cfg.Mastodon.Server != "https://my-mastodon.example.com" {
		t.Errorf("Expected mastodon server 'https://my-mastodon.example.com', got '%s'", cfg.Mastodon.Server)
	}
	if cfg.Mastodon.AccessToken != "my-secret-token" {
		t.Errorf("Expected mastodon access token 'my-secret-token', got '%s'", cfg.Mastodon.AccessToken)
	}
	if cfg.Mastodon.ClientTimeout != "45s" {
		t.Errorf("Expected mastodon client timeout '45s', got '%s'", cfg.Mastodon.ClientTimeout)
	}

	if cfg.Database.Path != "/tmp/test.db" {
		t.Errorf("Expected database path '/tmp/test.db', got '%s'", cfg.Database.Path)
	}
	if cfg.Database.WalMode != false {
		t.Errorf("Expected database wal_mode false, got %v", cfg.Database.WalMode)
	}
	if cfg.Database.BusyTimeout != "30s" {
		t.Errorf("Expected database busy_timeout '30s', got '%s'", cfg.Database.BusyTimeout)
	}

	if cfg.Web.Listen != "0.0.0.0" {
		t.Errorf("Expected web listen '0.0.0.0', got '%s'", cfg.Web.Listen)
	}
	if cfg.Web.Port != 3000 {
		t.Errorf("Expected web port 3000, got %d", cfg.Web.Port)
	}

	if cfg.Polling.Interval != "2m" {
		t.Errorf("Expected polling interval '2m', got '%s'", cfg.Polling.Interval)
	}
	if cfg.Polling.BatchSize != 50 {
		t.Errorf("Expected polling batch_size 50, got %d", cfg.Polling.BatchSize)
	}
	if cfg.Polling.BackfillDelay != "5s" {
		t.Errorf("Expected polling backfill_delay '5s', got '%s'", cfg.Polling.BackfillDelay)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected logging level 'debug', got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected logging format 'json', got '%s'", cfg.Logging.Format)
	}

	expectedFields := []string{"content", "username"}
	if !reflect.DeepEqual(cfg.Search.IndexedFields, expectedFields) {
		t.Errorf("Expected search indexed_fields %v, got %v", expectedFields, cfg.Search.IndexedFields)
	}
}

func TestLoadConfigPartialFile(t *testing.T) {
	// Test that partial config files preserve defaults for missing values
	configContent := `[mastodon]
server = "https://custom.mastodon.server"

[database]
wal_mode = false

[polling]
batch_size = 100
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify specified values were loaded
	if cfg.Mastodon.Server != "https://custom.mastodon.server" {
		t.Errorf("Expected mastodon server 'https://custom.mastodon.server', got '%s'", cfg.Mastodon.Server)
	}
	if cfg.Database.WalMode != false {
		t.Errorf("Expected database wal_mode false, got %v", cfg.Database.WalMode)
	}
	if cfg.Polling.BatchSize != 100 {
		t.Errorf("Expected polling batch_size 100, got %d", cfg.Polling.BatchSize)
	}

	// Verify defaults are preserved for missing values
	if cfg.Mastodon.AccessToken != "your-access-token-here" {
		t.Errorf("Expected default mastodon access token 'your-access-token-here', got '%s'", cfg.Mastodon.AccessToken)
	}
	if cfg.Database.Path != "./bookmarchive.db" {
		t.Errorf("Expected default database path './bookmarchive.db', got '%s'", cfg.Database.Path)
	}
	if cfg.Polling.Interval != "10m" {
		t.Errorf("Expected default polling interval '10m', got '%s'", cfg.Polling.Interval)
	}
}

func TestLoadConfigBooleanOverrideDefaults(t *testing.T) {
	// This is the critical test for the reported bug: boolean values from TOML must override defaults
	// even when the TOML value is false and the default is true
	configContent := `[database]
wal_mode = false
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bool_override_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()

	// Verify the default is true
	if cfg.Database.WalMode != true {
		t.Fatalf("Test setup error: expected default wal_mode to be true")
	}

	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// This is the critical test: false in TOML must override true default
	if cfg.Database.WalMode != false {
		t.Errorf("CRITICAL BUG: Expected database wal_mode false (from TOML), got %v (default not properly overridden)", cfg.Database.WalMode)
	}
}

func TestLoadConfigBooleanTrueOverride(t *testing.T) {
	// Test that true values also properly override defaults
	configContent := `[database]
wal_mode = true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bool_true_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	// Change the default to false for this test
	cfg.Database.WalMode = false

	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// true in TOML should override false default
	if cfg.Database.WalMode != true {
		t.Errorf("Expected database wal_mode true (from TOML), got %v", cfg.Database.WalMode)
	}
}

func TestLoadConfigZeroValueOverrides(t *testing.T) {
	// Test that zero values in TOML properly override non-zero defaults
	configContent := `[web]
port = 0

[polling]
batch_size = 0

[mastodon]
server = ""
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "zero_values_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Zero values from TOML should override defaults
	if cfg.Web.Port != 0 {
		t.Errorf("Expected web port 0 (from TOML), got %d", cfg.Web.Port)
	}
	if cfg.Polling.BatchSize != 0 {
		t.Errorf("Expected polling batch_size 0 (from TOML), got %d", cfg.Polling.BatchSize)
	}
	if cfg.Mastodon.Server != "" {
		t.Errorf("Expected mastodon server \"\" (from TOML), got '%s'", cfg.Mastodon.Server)
	}
}

func TestLoadConfigInvalidTOML(t *testing.T) {
	// Test handling of invalid TOML syntax
	configContent := `[mastodon
server = "missing closing bracket"`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	err := loadConfig(configPath, &cfg)

	if err == nil {
		t.Error("Expected error when loading invalid TOML")
	}
}

func TestLoadConfigArrayOverrides(t *testing.T) {
	// Test that arrays from TOML properly override defaults
	configContent := `[search]
indexed_fields = ["content"]
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "array_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	expectedFields := []string{"content"}
	if !reflect.DeepEqual(cfg.Search.IndexedFields, expectedFields) {
		t.Errorf("Expected search indexed_fields %v (from TOML), got %v", expectedFields, cfg.Search.IndexedFields)
	}
}

func TestLoadConfigEmptyArrayOverrides(t *testing.T) {
	// Test that empty arrays from TOML properly override defaults
	configContent := `[search]
indexed_fields = []
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "empty_array_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Empty array from TOML should override default array
	if len(cfg.Search.IndexedFields) != 0 {
		t.Errorf("Expected empty search indexed_fields array (from TOML), got %v", cfg.Search.IndexedFields)
	}
}

func TestMergeStructsDirectly(t *testing.T) {
	// Test the mergeStructs function directly to understand its behavior
	type TestStruct struct {
		StringField string `toml:"string_field"`
		BoolField   bool   `toml:"bool_field"`
		IntField    int    `toml:"int_field"`
	}

	// Test case 1: explicit false boolean should override true default
	dst := TestStruct{StringField: "default", BoolField: true, IntField: 42}
	src := TestStruct{BoolField: false} // Only bool field set to false
	tomlMap := map[string]interface{}{
		"bool_field": false, // Explicitly present in TOML
	}

	mergeStructs(&dst, &src, tomlMap)

	if dst.BoolField != false {
		t.Errorf("Expected bool_field false (explicitly set in TOML), got %v", dst.BoolField)
	}
	if dst.StringField != "default" {
		t.Errorf("Expected string_field 'default' (not in TOML), got '%s'", dst.StringField)
	}
	if dst.IntField != 42 {
		t.Errorf("Expected int_field 42 (not in TOML), got %d", dst.IntField)
	}
}

func TestMergeStructsNestedStructs(t *testing.T) {
	// Test nested struct merging behavior
	type NestedStruct struct {
		NestedBool   bool   `toml:"nested_bool"`
		NestedString string `toml:"nested_string"`
	}

	type ParentStruct struct {
		Nested NestedStruct `toml:"nested"`
	}

	// Set up defaults
	dst := ParentStruct{
		Nested: NestedStruct{
			NestedBool:   true,
			NestedString: "default",
		},
	}

	// Source with only the boolean set to false
	src := ParentStruct{
		Nested: NestedStruct{
			NestedBool: false,
		},
	}

	// TOML map indicating the boolean was explicitly set
	tomlMap := map[string]interface{}{
		"nested": map[string]interface{}{
			"nested_bool": false,
		},
	}

	mergeStructs(&dst, &src, tomlMap)

	if dst.Nested.NestedBool != false {
		t.Errorf("Expected nested.nested_bool false (explicitly set in TOML), got %v", dst.Nested.NestedBool)
	}
	if dst.Nested.NestedString != "default" {
		t.Errorf("Expected nested.nested_string 'default' (not in TOML), got '%s'", dst.Nested.NestedString)
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"trace", "trace"},
		{"TRACE", "trace"},
		{"debug", "debug"},
		{"DEBUG", "debug"},
		{"info", "info"},
		{"INFO", "info"},
		{"warn", "warn"},
		{"WARN", "warn"},
		{"warning", "warn"},
		{"WARNING", "warn"},
		{"error", "error"},
		{"ERROR", "error"},
		{"fatal", "fatal"},
		{"FATAL", "fatal"},
		{"panic", "panic"},
		{"PANIC", "panic"},
		{"invalid", "info"}, // Should default to info
		{"", "info"},        // Should default to info
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			level := parseLogLevel(test.input)
			levelStr := level.String()
			if levelStr != test.expected {
				t.Errorf("parseLogLevel(%q) = %s, expected %s", test.input, levelStr, test.expected)
			}
		})
	}
}

func TestValidLogLevels(t *testing.T) {
	levels := validLogLevels()
	expected := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}

	if !reflect.DeepEqual(levels, expected) {
		t.Errorf("validLogLevels() = %v, expected %v", levels, expected)
	}
}

func TestConfigurationSourceOfTruthPrinciple(t *testing.T) {
	// This test verifies the "principle of least astonishment" - what's in config must be the source of truth
	configContent := `[mastodon]
server = ""
access_token = ""
client_timeout = "1s"

[database]
path = ""
wal_mode = false
busy_timeout = "1s"

[web]
listen = ""
port = 0

[polling]
interval = "1s"
batch_size = 0
backfill_delay = "0s"

[logging]
level = ""
format = ""

[search]
indexed_fields = []
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "source_of_truth_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Every value explicitly set in TOML should override defaults, even if they are "empty" or "zero" values
	if cfg.Mastodon.Server != "" {
		t.Errorf("Expected mastodon server '' (from TOML), got '%s'", cfg.Mastodon.Server)
	}
	if cfg.Mastodon.AccessToken != "" {
		t.Errorf("Expected mastodon access_token '' (from TOML), got '%s'", cfg.Mastodon.AccessToken)
	}
	if cfg.Mastodon.ClientTimeout != "1s" {
		t.Errorf("Expected mastodon client_timeout '1s' (from TOML), got '%s'", cfg.Mastodon.ClientTimeout)
	}

	if cfg.Database.Path != "" {
		t.Errorf("Expected database path '' (from TOML), got '%s'", cfg.Database.Path)
	}
	if cfg.Database.WalMode != false {
		t.Errorf("Expected database wal_mode false (from TOML), got %v", cfg.Database.WalMode)
	}
	if cfg.Database.BusyTimeout != "1s" {
		t.Errorf("Expected database busy_timeout '1s' (from TOML), got '%s'", cfg.Database.BusyTimeout)
	}

	if cfg.Web.Listen != "" {
		t.Errorf("Expected web listen '' (from TOML), got '%s'", cfg.Web.Listen)
	}
	if cfg.Web.Port != 0 {
		t.Errorf("Expected web port 0 (from TOML), got %d", cfg.Web.Port)
	}

	if cfg.Polling.Interval != "1s" {
		t.Errorf("Expected polling interval '1s' (from TOML), got '%s'", cfg.Polling.Interval)
	}
	if cfg.Polling.BatchSize != 0 {
		t.Errorf("Expected polling batch_size 0 (from TOML), got %d", cfg.Polling.BatchSize)
	}
	if cfg.Polling.BackfillDelay != "0s" {
		t.Errorf("Expected polling backfill_delay '0s' (from TOML), got '%s'", cfg.Polling.BackfillDelay)
	}

	if cfg.Logging.Level != "" {
		t.Errorf("Expected logging level '' (from TOML), got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "" {
		t.Errorf("Expected logging format '' (from TOML), got '%s'", cfg.Logging.Format)
	}

	if len(cfg.Search.IndexedFields) != 0 {
		t.Errorf("Expected search indexed_fields [] (from TOML), got %v", cfg.Search.IndexedFields)
	}
}

func TestConfigurationIntegrationWithTimeouts(t *testing.T) {
	// Test that configuration works with actual time.Duration parsing
	configContent := `[mastodon]
server = "https://test.example.com"
access_token = "test-token"
client_timeout = "45s"

[database]
busy_timeout = "30s"

[polling]
interval = "5m"
backfill_delay = "15s"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "integration_config.toml")

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg := defaultConfig()
	if err := loadConfig(configPath, &cfg); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test that the durations can be parsed by the actual application code
	clientTimeout, err := time.ParseDuration(cfg.Mastodon.ClientTimeout)
	if err != nil {
		t.Errorf("Failed to parse mastodon client_timeout: %v", err)
	}
	if clientTimeout != 45*time.Second {
		t.Errorf("Expected client timeout 45s, got %v", clientTimeout)
	}

	busyTimeout, err := time.ParseDuration(cfg.Database.BusyTimeout)
	if err != nil {
		t.Errorf("Failed to parse database busy_timeout: %v", err)
	}
	if busyTimeout != 30*time.Second {
		t.Errorf("Expected busy timeout 30s, got %v", busyTimeout)
	}

	interval, err := time.ParseDuration(cfg.Polling.Interval)
	if err != nil {
		t.Errorf("Failed to parse polling interval: %v", err)
	}
	if interval != 5*time.Minute {
		t.Errorf("Expected polling interval 5m, got %v", interval)
	}

	backfillDelay, err := time.ParseDuration(cfg.Polling.BackfillDelay)
	if err != nil {
		t.Errorf("Failed to parse polling backfill_delay: %v", err)
	}
	if backfillDelay != 15*time.Second {
		t.Errorf("Expected backfill delay 15s, got %v", backfillDelay)
	}
}
