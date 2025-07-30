package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/McKael/madon/v3"
	"github.com/microcosm-cc/bluemonday"
	"github.com/peterhellberg/link"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

//go:embed web/*
var webFS embed.FS

// =============================================================================
// VERSION INFORMATION
// =============================================================================

// Version information set by build process
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
	builtBy = "unknown"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

type Config struct {
	Mastodon struct {
		Server        string `toml:"server"`
		AccessToken   string `toml:"access_token"`
		ClientTimeout string `toml:"client_timeout"`
	} `toml:"mastodon"`
	Database struct {
		Path        string `toml:"path"`
		WalMode     bool   `toml:"wal_mode"`
		BusyTimeout string `toml:"busy_timeout"`
	} `toml:"database"`
	Polling struct {
		Interval      string `toml:"interval"`
		BatchSize     int    `toml:"batch_size"`
		BackfillDelay string `toml:"backfill_delay"`
	} `toml:"polling"`
	Web struct {
		Listen string `toml:"listen"`
		Port   int    `toml:"port"`
	} `toml:"web"`
	Logging struct {
		Level  string `toml:"level"`
		Format string `toml:"format"`
	} `toml:"logging"`
	Search struct {
		IndexedFields []string `toml:"indexed_fields"`
	} `toml:"search"`
}

func defaultConfig() Config {
	return Config{
		Mastodon: struct {
			Server        string `toml:"server"`
			AccessToken   string `toml:"access_token"`
			ClientTimeout string `toml:"client_timeout"`
		}{
			Server:        "https://mastodon.social",
			AccessToken:   "your-access-token-here",
			ClientTimeout: "30s",
		},
		Database: struct {
			Path        string `toml:"path"`
			WalMode     bool   `toml:"wal_mode"`
			BusyTimeout string `toml:"busy_timeout"`
		}{
			Path:        "./bookmarchive.db",
			WalMode:     true,
			BusyTimeout: "5s",
		},
		Polling: struct {
			Interval      string `toml:"interval"`
			BatchSize     int    `toml:"batch_size"`
			BackfillDelay string `toml:"backfill_delay"`
		}{
			Interval:      "10m",
			BatchSize:     20,
			BackfillDelay: "10s",
		},
		Web: struct {
			Listen string `toml:"listen"`
			Port   int    `toml:"port"`
		}{
			Listen: "127.0.0.1",
			Port:   8080,
		},
		Logging: struct {
			Level  string `toml:"level"`
			Format string `toml:"format"`
		}{
			Level:  "info",
			Format: "console",
		},
		Search: struct {
			IndexedFields []string `toml:"indexed_fields"`
		}{
			IndexedFields: []string{"content", "spoiler_text", "username", "display_name", "media_descriptions", "hashtags"},
		},
	}
}

func loadConfig(path string, cfg interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// First, read the TOML content to see what fields are actually present
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Use bytes.Reader to avoid reading the file twice
	reader := bytes.NewReader(content)

	// Parse into a map first to see what keys are present
	var tomlMap map[string]interface{}
	if _, err := toml.NewDecoder(reader).Decode(&tomlMap); err != nil {
		return err
	}

	// Reset reader for the next decode
	if _, err := reader.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset reader: %w", err)
	}

	// Now decode into the actual struct
	cfgType := reflect.TypeOf(cfg).Elem()
	partial := reflect.New(cfgType).Interface()
	if _, err := toml.NewDecoder(reader).Decode(partial); err != nil {
		return err
	}

	mergeStructs(cfg, partial, tomlMap)
	return nil
}

func mergeStructs(dst, src interface{}, tomlMap map[string]interface{}) {
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()
	dstType := dstVal.Type()

	for i := 0; i < dstVal.NumField(); i++ {
		field := dstVal.Field(i)
		srcField := srcVal.Field(i)
		fieldType := dstType.Field(i)

		tomlTag := getTomlTag(fieldType)

		if field.Kind() == reflect.Struct {
			// For nested structs, check if the section exists in TOML
			nestedMap := make(map[string]interface{})
			if sectionMap, ok := tomlMap[tomlTag].(map[string]interface{}); ok {
				nestedMap = sectionMap
			}

			mergeStructs(field.Addr().Interface(), srcField.Addr().Interface(), nestedMap)
		} else {
			// Check if this field was actually present in the TOML
			if _, fieldPresent := tomlMap[tomlTag]; fieldPresent {
				// Field was explicitly set in TOML, so merge it regardless of value
				field.Set(srcField)
			} else {
				// Field was not in TOML, so only merge non-zero values
				zero := reflect.Zero(field.Type()).Interface()
				if !reflect.DeepEqual(srcField.Interface(), zero) {
					field.Set(srcField)
				}
			}
		}
	}
}

// getTomlTag extracts the TOML tag from a struct field, or uses the field name if not present.
func getTomlTag(field reflect.StructField) string {
	tag := field.Tag.Get("toml")
	if tag != "" {
		return tag
	}
	return field.Name
}

// =============================================================================
// LOGGING
// =============================================================================

func setupLogging(level, format string) {
	logLevel := parseLogLevel(level)
	zerolog.SetGlobalLevel(logLevel)

	if format == "console" {
		zlog.Logger = zlog.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		zlog.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	if logLevel <= zerolog.DebugLevel {
		zlog.Logger = zlog.Logger.With().Caller().Logger()
	}
}

func parseLogLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

func validLogLevels() []string {
	return []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
}

// =============================================================================
// DATABASE MODELS
// =============================================================================

type DBBookmark struct {
	StatusID     string    `json:"status_id"`
	CreatedAt    time.Time `json:"created_at"`
	BookmarkedAt time.Time `json:"bookmarked_at"`
	SearchText   string    `json:"search_text"`
	RawJSON      string    `json:"raw_json"`
	AccountID    string    `json:"account_id"`
}

type BackfillState struct {
	LastProcessedID  string     `json:"last_processed_id,omitempty"`
	BackfillComplete bool       `json:"backfill_complete"`
	LastPollTime     *time.Time `json:"last_poll_time,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type SearchResult struct {
	Bookmark *DBBookmark `json:"bookmark"`
	Rank     float64     `json:"rank"`
	Snippet  string      `json:"snippet,omitempty"`
}

type SearchRequest struct {
	Query              string `json:"query"`
	Limit              int    `json:"limit,omitempty"`
	Offset             int    `json:"offset,omitempty"`
	EnableHighlighting bool   `json:"enable_highlighting,omitempty"`
	SnippetLength      int    `json:"snippet_length,omitempty"`
	FilterByAccount    string `json:"filter_by_account,omitempty"`
}

type UserAccount struct {
	AccountID   string    `json:"account_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Acct        string    `json:"acct"`
	Avatar      string    `json:"avatar"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// =============================================================================
// DATABASE
// =============================================================================

type Database struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

func newDatabase(cfg Config) (*Database, error) {
	var busyTimeout time.Duration
	var err error
	if cfg.Database.BusyTimeout != "" {
		busyTimeout, err = time.ParseDuration(cfg.Database.BusyTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid busy timeout: %w", err)
		}
	} else {
		busyTimeout = 5 * time.Second
	}

	dir := filepath.Dir(cfg.Database.Path)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	busyTimeoutMs := int(busyTimeout.Milliseconds())
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMs)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	if cfg.Database.WalMode {
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	database := &Database{db: db, path: cfg.Database.Path}

	if err := database.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return database, nil
}

func (d *Database) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.db == nil {
		return nil
	}
	err := d.db.Close()
	d.db = nil
	return err
}

// getDB safely returns the database connection
func (d *Database) getDB() (*sql.DB, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}
	return d.db, nil
}

func (d *Database) runMigrations() error {
	db, err := d.getDB()
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			zlog.Warn().Err(err).Msg("failed to rollback migration transaction")
		}
	}()

	for _, stmt := range getMigrationStatements() {
		// Handle special case for ALTER TABLE ADD COLUMN account_id
		if stmt == `ALTER TABLE bookmarks ADD COLUMN account_id TEXT` {
			// Check if column already exists
			var count int
			err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('bookmarks') WHERE name = 'account_id'`).Scan(&count)
			if err != nil {
				return fmt.Errorf("failed to check for account_id column existence: %w", err)
			}
			if count > 0 {
				// Column already exists, skip this migration
				continue
			}
		}

		// Handle special case for account_id index creation
		if stmt == `CREATE INDEX IF NOT EXISTS idx_account_id ON bookmarks(account_id)` {
			// Check if column exists before creating index
			var count int
			err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('bookmarks') WHERE name = 'account_id'`).Scan(&count)
			if err != nil {
				return fmt.Errorf("failed to check for account_id column existence for index: %w", err)
			}
			if count == 0 {
				// Column doesn't exist yet, skip index creation
				continue
			}
		}

		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute migration statement: %w", err)
		}
	}

	return tx.Commit()
}

func getMigrationStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS bookmarks (
			status_id TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL,
			bookmarked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			search_text TEXT NOT NULL,
			raw_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_created_at ON bookmarks(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_bookmarked_at ON bookmarks(bookmarked_at)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS bookmarks_fts USING fts5(
			status_id UNINDEXED,
			search_text,
			content='bookmarks',
			content_rowid='rowid',
			tokenize='porter unicode61 remove_diacritics 1'
		)`,
		`CREATE TRIGGER IF NOT EXISTS bookmarks_fts_insert AFTER INSERT ON bookmarks BEGIN
			INSERT INTO bookmarks_fts(rowid, status_id, search_text)
			VALUES (new.rowid, new.status_id, new.search_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS bookmarks_fts_delete AFTER DELETE ON bookmarks BEGIN
			INSERT INTO bookmarks_fts(bookmarks_fts, rowid, status_id, search_text)
			VALUES('delete', old.rowid, old.status_id, old.search_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS bookmarks_fts_update AFTER UPDATE ON bookmarks BEGIN
			INSERT INTO bookmarks_fts(bookmarks_fts, rowid, status_id, search_text)
			VALUES('delete', old.rowid, old.status_id, old.search_text);
			INSERT INTO bookmarks_fts(rowid, status_id, search_text)
			VALUES (new.rowid, new.status_id, new.search_text);
		END`,
		`CREATE TABLE IF NOT EXISTS backfill_state (
			id INTEGER PRIMARY KEY DEFAULT 1,
			last_processed_id TEXT,
			backfill_complete BOOLEAN NOT NULL DEFAULT FALSE,
			last_poll_time DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CHECK (id = 1)
		)`,
		`INSERT OR IGNORE INTO backfill_state (id) VALUES (1)`,
		`ALTER TABLE bookmarks ADD COLUMN account_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_account_id ON bookmarks(account_id)`,
		`CREATE TABLE IF NOT EXISTS user_account (
			id INTEGER PRIMARY KEY DEFAULT 1,
			account_id TEXT NOT NULL,
			username TEXT NOT NULL,
			display_name TEXT NOT NULL,
			acct TEXT NOT NULL,
			avatar TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CHECK (id = 1)
		)`,
		// Populate account_id from existing raw_json data
		`UPDATE bookmarks SET account_id = (
			SELECT json_extract(raw_json, '$.status.account.id')
			WHERE json_extract(raw_json, '$.status.account.id') IS NOT NULL
		) WHERE account_id IS NULL`,
	}
}

func (d *Database) insertBookmark(bookmark *DBBookmark) error {
	db, err := d.getDB()
	if err != nil {
		return err
	}

	query := `INSERT OR REPLACE INTO bookmarks 
		(status_id, created_at, bookmarked_at, search_text, raw_json, account_id)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err = db.Exec(query,
		bookmark.StatusID,
		bookmark.CreatedAt.UTC(),
		bookmark.BookmarkedAt.UTC(),
		bookmark.SearchText,
		bookmark.RawJSON,
		bookmark.AccountID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert bookmark: %w", err)
	}
	return nil
}

func (d *Database) getBookmark(statusID string) (*DBBookmark, error) {
	db, err := d.getDB()
	if err != nil {
		return nil, err
	}

	query := `SELECT status_id, created_at, bookmarked_at, search_text, raw_json, COALESCE(account_id, '') as account_id
		FROM bookmarks WHERE status_id = ?`

	var bookmark DBBookmark
	err = db.QueryRow(query, statusID).Scan(
		&bookmark.StatusID,
		&bookmark.CreatedAt,
		&bookmark.BookmarkedAt,
		&bookmark.SearchText,
		&bookmark.RawJSON,
		&bookmark.AccountID,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get bookmark: %w", err)
	}
	return &bookmark, nil
}

func (d *Database) getBackfillState() (*BackfillState, error) {
	db, err := d.getDB()
	if err != nil {
		return nil, err
	}

	var state BackfillState
	var lastProcessedID sql.NullString
	var lastPollTime sql.NullTime

	query := `SELECT last_processed_id, backfill_complete, last_poll_time, created_at, updated_at
		FROM backfill_state WHERE id = 1`

	err = db.QueryRow(query).Scan(
		&lastProcessedID,
		&state.BackfillComplete,
		&lastPollTime,
		&state.CreatedAt,
		&state.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("backfill state not initialized")
		}
		return nil, fmt.Errorf("failed to get backfill state: %w", err)
	}

	if lastProcessedID.Valid {
		state.LastProcessedID = lastProcessedID.String
	}
	if lastPollTime.Valid {
		state.LastPollTime = &lastPollTime.Time
	}

	return &state, nil
}

func (d *Database) insertUserAccount(account *UserAccount) error {
	db, err := d.getDB()
	if err != nil {
		return err
	}

	query := `INSERT OR REPLACE INTO user_account 
		(id, account_id, username, display_name, acct, avatar, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`

	_, err = db.Exec(query,
		account.AccountID,
		account.Username,
		account.DisplayName,
		account.Acct,
		account.Avatar,
	)

	if err != nil {
		return fmt.Errorf("failed to insert user account: %w", err)
	}
	return nil
}

func (d *Database) getUserAccount() (*UserAccount, error) {
	db, err := d.getDB()
	if err != nil {
		return nil, err
	}

	query := `SELECT account_id, username, display_name, acct, COALESCE(avatar, '') as avatar, created_at, updated_at
		FROM user_account WHERE id = 1`

	var account UserAccount
	err = db.QueryRow(query).Scan(
		&account.AccountID,
		&account.Username,
		&account.DisplayName,
		&account.Acct,
		&account.Avatar,
		&account.CreatedAt,
		&account.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user account: %w", err)
	}
	return &account, nil
}

func (d *Database) updateBackfillState(lastProcessedID string, backfillComplete bool, lastPollTime *time.Time) error {
	db, err := d.getDB()
	if err != nil {
		return err
	}

	query := `UPDATE backfill_state 
		SET last_processed_id = ?, backfill_complete = ?, last_poll_time = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = 1`

	var lastPollTimeParam interface{}
	if lastPollTime != nil {
		lastPollTimeParam = lastPollTime.UTC()
	}

	result, err := db.Exec(query, lastProcessedID, backfillComplete, lastPollTimeParam)
	if err != nil {
		return fmt.Errorf("failed to update backfill state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("backfill state not found")
	}
	return nil
}

func (d *Database) searchBookmarksWithFTS5(request *SearchRequest) ([]*SearchResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if request.Query == "" {
		return []*SearchResult{}, nil
	}

	limit := request.Limit
	if limit <= 0 {
		limit = 100
	}

	offset := request.Offset
	if offset < 0 {
		offset = 0
	}

	snippetLength := request.SnippetLength
	if snippetLength <= 0 {
		snippetLength = 200
	}

	searchQuery := prepareFTS5Query(request.Query)

	var query string
	var args []interface{}

	// Build WHERE clause for account filtering
	accountFilter := ""
	var userAccountID string
	if request.FilterByAccount == "my_posts" {
		// Get current user account to filter by
		userAccount, err := d.getUserAccount()
		if err != nil {
			return nil, fmt.Errorf("failed to get user account for filtering: %w", err)
		}
		if userAccount != nil {
			accountFilter = " AND b.account_id = ?"
			userAccountID = userAccount.AccountID
		} else {
			// No user account configured, return empty results for my_posts filter
			return []*SearchResult{}, nil
		}
	}

	if request.EnableHighlighting {
		query = `
			SELECT 
				b.status_id, b.created_at, b.bookmarked_at, b.search_text, b.raw_json, COALESCE(b.account_id, '') as account_id,
				bm25(bookmarks_fts) as rank,
				snippet(bookmarks_fts, 1, '<mark>', '</mark>', '...', ?) as snippet
			FROM bookmarks_fts
			JOIN bookmarks b ON b.rowid = bookmarks_fts.rowid
			WHERE bookmarks_fts MATCH ?` + accountFilter + `
			ORDER BY rank
			LIMIT ? OFFSET ?`
		args = []interface{}{snippetLength, searchQuery}
		if accountFilter != "" {
			args = append(args, userAccountID)
		}
		args = append(args, limit, offset)
	} else {
		query = `
			SELECT 
				b.status_id, b.created_at, b.bookmarked_at, b.search_text, b.raw_json, COALESCE(b.account_id, '') as account_id,
				bm25(bookmarks_fts) as rank
			FROM bookmarks_fts
			JOIN bookmarks b ON b.rowid = bookmarks_fts.rowid
			WHERE bookmarks_fts MATCH ?` + accountFilter + `
			ORDER BY rank
			LIMIT ? OFFSET ?`
		args = []interface{}{searchQuery}
		if accountFilter != "" {
			args = append(args, userAccountID)
		}
		args = append(args, limit, offset)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute FTS5 search: %w", err)
	}
	defer rows.Close()

	results := []*SearchResult{}
	for rows.Next() {
		var bookmark DBBookmark
		var rank float64
		var snippet sql.NullString

		if request.EnableHighlighting {
			err = rows.Scan(
				&bookmark.StatusID,
				&bookmark.CreatedAt,
				&bookmark.BookmarkedAt,
				&bookmark.SearchText,
				&bookmark.RawJSON,
				&bookmark.AccountID,
				&rank,
				&snippet,
			)
		} else {
			err = rows.Scan(
				&bookmark.StatusID,
				&bookmark.CreatedAt,
				&bookmark.BookmarkedAt,
				&bookmark.SearchText,
				&bookmark.RawJSON,
				&bookmark.AccountID,
				&rank,
			)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}

		result := &SearchResult{
			Bookmark: &bookmark,
			Rank:     rank,
		}

		if snippet.Valid {
			result.Snippet = snippet.String
		}

		results = append(results, result)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over search results: %w", err)
	}

	return results, nil
}

func (d *Database) getRecentBookmarks(limit, offset int, filterByAccount string) ([]*SearchResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var query string
	var args []interface{}

	// Build WHERE clause for account filtering
	accountFilter := ""
	if filterByAccount == "my_posts" {
		// Get current user account to filter by
		userAccount, err := d.getUserAccount()
		if err != nil {
			return nil, fmt.Errorf("failed to get user account for filtering: %w", err)
		}
		if userAccount != nil {
			accountFilter = " WHERE account_id = ?"
			args = append(args, userAccount.AccountID)
		} else {
			// No user account configured, return empty results for my_posts filter
			return []*SearchResult{}, nil
		}
	}

	query = `SELECT status_id, created_at, bookmarked_at, search_text, raw_json, COALESCE(account_id, '') as account_id
		FROM bookmarks` + accountFilter + ` ORDER BY bookmarked_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute recent bookmarks query: %w", err)
	}
	defer rows.Close()

	results := []*SearchResult{}
	for rows.Next() {
		var bookmark DBBookmark

		err = rows.Scan(
			&bookmark.StatusID,
			&bookmark.CreatedAt,
			&bookmark.BookmarkedAt,
			&bookmark.SearchText,
			&bookmark.RawJSON,
			&bookmark.AccountID,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan recent bookmark result: %w", err)
		}

		result := &SearchResult{
			Bookmark: &bookmark,
			Rank:     0.0,
		}
		results = append(results, result)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over recent bookmark results: %w", err)
	}

	return results, nil
}

func (d *Database) searchOrRecentBookmarks(request *SearchRequest) ([]*SearchResult, error) {
	if strings.TrimSpace(request.Query) == "" {
		return d.getRecentBookmarks(request.Limit, request.Offset, request.FilterByAccount)
	}
	return d.searchBookmarksWithFTS5(request)
}

func prepareFTS5Query(query string) string {
	query = strings.TrimSpace(query)

	if strings.HasPrefix(query, "\"") && strings.HasSuffix(query, "\"") {
		return query
	}

	upperQuery := strings.ToUpper(query)
	if strings.Contains(upperQuery, " AND ") || strings.Contains(upperQuery, " OR ") || strings.Contains(upperQuery, " NOT ") {
		return query
	}

	if !strings.HasSuffix(query, "*") && !strings.HasSuffix(query, "\"") && !strings.Contains(query, "\"") {
		words := strings.Fields(query)
		for i, word := range words {
			if !strings.HasSuffix(word, "*") && !strings.HasSuffix(word, "\"") && !strings.Contains(word, "\"") {
				words[i] = word + "*"
			}
		}
		return strings.Join(words, " ")
	}

	return query
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// =============================================================================
// MASTODON CLIENT
// =============================================================================

type MastodonClient struct {
	server      string
	accessToken string
	timeout     time.Duration
	madonClient *madon.Client
}

func newMastodonClient(cfg *Config) (*MastodonClient, error) {
	if cfg.Mastodon.Server == "" {
		return nil, fmt.Errorf("mastodon server URL is required")
	}
	if cfg.Mastodon.AccessToken == "" {
		return nil, fmt.Errorf("mastodon access token is required")
	}

	timeout := 30 * time.Second
	if cfg.Mastodon.ClientTimeout != "" {
		var err error
		timeout, err = time.ParseDuration(cfg.Mastodon.ClientTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid client timeout: %w", err)
		}
	}

	return &MastodonClient{
		server:      cfg.Mastodon.Server,
		accessToken: cfg.Mastodon.AccessToken,
		timeout:     timeout,
		madonClient: nil,
	}, nil
}

func (c *MastodonClient) initMadonClient() error {
	if c.madonClient != nil {
		return nil
	}

	userToken := &madon.UserToken{
		AccessToken: c.accessToken,
		TokenType:   "Bearer",
	}

	madonClient, err := madon.RestoreApp("bookmarchive", c.server, "dummy-app-id", "dummy-app-secret", userToken)
	if err != nil {
		return fmt.Errorf("failed to create madon client: %w", err)
	}

	c.madonClient = madonClient
	return nil
}

func (c *MastodonClient) verifyCredentials() error {
	if err := c.initMadonClient(); err != nil {
		return err
	}

	_, err := c.madonClient.GetCurrentAccount()
	if err != nil {
		return fmt.Errorf("failed to verify credentials: %w", err)
	}
	return nil
}

func (c *MastodonClient) getMadonClient() (*madon.Client, error) {
	if err := c.initMadonClient(); err != nil {
		return nil, err
	}
	return c.madonClient, nil
}

// =============================================================================
// BOOKMARK SERVICE MODELS
// =============================================================================

type Bookmark struct {
	ID        string    `json:"id"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Status struct {
	ID               string    `json:"id"`
	URI              string    `json:"uri"`
	URL              string    `json:"url"`
	Content          string    `json:"content"`
	SpoilerText      string    `json:"spoiler_text"`
	CreatedAt        time.Time `json:"created_at"`
	Account          Account   `json:"account"`
	MediaAttachments []Media   `json:"media_attachments"`
	Tags             []Tag     `json:"tags"`
}

type Account struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
}

type Media struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type Tag struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type BookmarkClient interface {
	GetBookmarks(ctx context.Context, limit int, nextURL string) ([]Bookmark, string, error)
}

// =============================================================================
// RATE LIMITER
// =============================================================================

type RateLimiter struct {
	maxRequests int
	timeWindow  time.Duration
	requests    []time.Time
}

func newRateLimiter(maxRequests int, timeWindow time.Duration) *RateLimiter {
	return &RateLimiter{
		maxRequests: maxRequests,
		timeWindow:  timeWindow,
		requests:    make([]time.Time, 0),
	}
}

func (rl *RateLimiter) allow() bool {
	now := time.Now()
	cutoff := now.Add(-rl.timeWindow)
	newRequests := make([]time.Time, 0, len(rl.requests))
	for _, req := range rl.requests {
		if req.After(cutoff) {
			newRequests = append(newRequests, req)
		}
	}
	rl.requests = newRequests

	if len(rl.requests) >= rl.maxRequests {
		return false
	}

	rl.requests = append(rl.requests, now)
	return true
}

func (rl *RateLimiter) wait(ctx context.Context) error {
	for !rl.allow() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil
}

// =============================================================================
// MASTODON BOOKMARK CLIENT
// =============================================================================

type MastodonBookmarkClient struct {
	client      *madon.Client
	rateLimiter *RateLimiter
	maxRetries  int
}

func newMastodonBookmarkClient(client *madon.Client, maxRetries int) *MastodonBookmarkClient {
	rateLimiter := newRateLimiter(150, 5*time.Minute)

	return &MastodonBookmarkClient{
		client:      client,
		rateLimiter: rateLimiter,
		maxRetries:  maxRetries,
	}
}

func (bc *MastodonBookmarkClient) GetBookmarks(ctx context.Context, limit int, nextURL string) ([]Bookmark, string, error) {
	if err := bc.rateLimiter.wait(ctx); err != nil {
		return nil, "", fmt.Errorf("rate limit wait failed: %w", err)
	}

	if bc.client == nil {
		return []Bookmark{}, "", nil
	}

	var requestURL string
	if nextURL != "" {
		requestURL = nextURL
	} else {
		baseURL := fmt.Sprintf("%s/api/v1/bookmarks", bc.client.InstanceURL)
		reqURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, "", fmt.Errorf("failed to parse URL: %w", err)
		}

		params := url.Values{}
		if limit > 0 {
			params.Set("limit", strconv.Itoa(limit))
		}
		reqURL.RawQuery = params.Encode()
		requestURL = reqURL.String()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	if bc.client.UserToken != nil && bc.client.UserToken.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bc.client.UserToken.AccessToken))
	}
	req.Header.Set("User-Agent", "bookmarchive/1.0")

	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= bc.maxRetries; attempt++ {
		resp, lastErr = http.DefaultClient.Do(req)
		if lastErr == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		if resp != nil && resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break
		}

		if attempt < bc.maxRetries {
			delay := time.Duration(attempt+1) * time.Second
			time.Sleep(delay)
		}
	}

	if lastErr != nil {
		return nil, "", fmt.Errorf("HTTP request failed after %d retries: %w", bc.maxRetries, lastErr)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var nextURLFromHeader string
	links := link.ParseResponse(resp)
	for _, l := range links {
		if l.Rel == "next" {
			nextURLFromHeader = l.URI
			break
		}
	}

	var madonStatuses []madon.Status
	if err := json.NewDecoder(resp.Body).Decode(&madonStatuses); err != nil {
		return nil, "", fmt.Errorf("failed to decode JSON response: %w", err)
	}

	bookmarks := make([]Bookmark, len(madonStatuses))
	for i, status := range madonStatuses {
		bookmarks[i] = convertMadonStatusToBookmark(status)
	}

	return bookmarks, nextURLFromHeader, nil
}

func convertMadonStatusToBookmark(status madon.Status) Bookmark {
	account := Account{
		ID:          string(status.Account.ID),
		Username:    status.Account.Username,
		DisplayName: status.Account.DisplayName,
		Avatar:      status.Account.Avatar,
	}

	var mediaAttachments []Media
	for _, media := range status.MediaAttachments {
		description := ""
		if media.Description != nil {
			description = *media.Description
		}

		mediaAttachments = append(mediaAttachments, Media{
			ID:          string(media.ID),
			Type:        media.Type,
			URL:         media.URL,
			Description: description,
		})
	}

	var tags []Tag
	for _, tag := range status.Tags {
		tags = append(tags, Tag{
			Name: tag.Name,
			URL:  tag.URL,
		})
	}

	serviceStatus := Status{
		ID:               string(status.ID),
		URI:              status.URI,
		URL:              status.URL,
		Content:          status.Content,
		SpoilerText:      status.SpoilerText,
		CreatedAt:        status.CreatedAt,
		Account:          account,
		MediaAttachments: mediaAttachments,
		Tags:             tags,
	}

	return Bookmark{
		ID:        string(status.ID),
		Status:    serviceStatus,
		CreatedAt: status.CreatedAt,
	}
}

func convertBookmarkToDatabase(bookmark Bookmark, indexedFields []string) *DBBookmark {
	rawJSON, err := json.Marshal(bookmark)
	if err != nil {
		rawJSON = []byte(fmt.Sprintf(`{"id":"%s","status_id":"%s","created_at":"%s"}`,
			bookmark.ID,
			bookmark.Status.ID,
			bookmark.CreatedAt.Format(time.RFC3339)))
	}

	searchText := buildSearchText(bookmark, indexedFields)

	// Extract account_id from the status account
	accountID := ""
	if bookmark.Status.Account.ID != "" {
		accountID = bookmark.Status.Account.ID
	}

	return &DBBookmark{
		StatusID:     bookmark.Status.ID,
		CreatedAt:    bookmark.Status.CreatedAt,
		BookmarkedAt: bookmark.CreatedAt,
		SearchText:   searchText,
		RawJSON:      string(rawJSON),
		AccountID:    accountID,
	}
}

func buildSearchText(bookmark Bookmark, indexedFields []string) string {
	var searchParts []string

	shouldIndex := func(field string) bool {
		for _, f := range indexedFields {
			if f == field {
				return true
			}
		}
		return false
	}

	if shouldIndex("content") && bookmark.Status.Content != "" {
		searchParts = append(searchParts, stripHTML(bookmark.Status.Content))
	}

	if shouldIndex("spoiler_text") && bookmark.Status.SpoilerText != "" {
		searchParts = append(searchParts, bookmark.Status.SpoilerText)
	}

	if shouldIndex("username") && bookmark.Status.Account.Username != "" {
		searchParts = append(searchParts, bookmark.Status.Account.Username)
	}

	if shouldIndex("display_name") && bookmark.Status.Account.DisplayName != "" {
		searchParts = append(searchParts, bookmark.Status.Account.DisplayName)
	}

	if shouldIndex("media_descriptions") {
		for _, media := range bookmark.Status.MediaAttachments {
			if media.Description != "" {
				searchParts = append(searchParts, media.Description)
			}
		}
	}

	if shouldIndex("hashtags") {
		for _, tag := range bookmark.Status.Tags {
			if tag.Name != "" {
				searchParts = append(searchParts, tag.Name)
			}
		}
	}

	return strings.Join(searchParts, " ")
}

func stripHTML(html string) string {
	// Use BlueMondaty's StrictPolicy to strip all HTML tags securely
	policy := bluemonday.StrictPolicy()
	// Add space when stripping tags to prevent words from being merged
	policy = policy.AddSpaceWhenStrippingTag(true)

	// Strip all HTML tags and return clean text
	cleaned := policy.Sanitize(html)

	// Clean up extra whitespace
	cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// =============================================================================
// BOOKMARK SERVICE
// =============================================================================

type BookmarkService struct {
	config    *Config
	db        *Database
	client    BookmarkClient
	ctx       context.Context
	cancel    context.CancelFunc
	eventChan chan<- ServerEvent
}

func newBookmarkService(cfg *Config, db *Database, eventChan chan<- ServerEvent) (*BookmarkService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if cfg.Mastodon.Server == "" {
		return nil, fmt.Errorf("mastodon server URL is required")
	}
	if cfg.Mastodon.AccessToken == "" {
		return nil, fmt.Errorf("mastodon access token is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &BookmarkService{
		config:    cfg,
		db:        db,
		ctx:       ctx,
		cancel:    cancel,
		eventChan: eventChan,
	}

	return service, nil
}

func (s *BookmarkService) start() error {
	client, err := s.createBookmarkClient()
	if err != nil {
		return fmt.Errorf("failed to create bookmark client: %w", err)
	}
	s.client = client

	if err := s.runBackfill(); err != nil {
		return fmt.Errorf("backfill failed: %w", err)
	}

	return s.startPolling()
}

func (s *BookmarkService) stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *BookmarkService) createBookmarkClient() (BookmarkClient, error) {
	mastodonClient, err := newMastodonClient(s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create mastodon client: %w", err)
	}

	if err := mastodonClient.verifyCredentials(); err != nil {
		return nil, fmt.Errorf("failed to verify mastodon credentials: %w", err)
	}

	// After successful verification, store the current user's account information
	madonClient, err := mastodonClient.getMadonClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get madon client: %w", err)
	}

	// Get and store current user account information
	account, err := madonClient.GetCurrentAccount()
	if err != nil {
		zlog.Warn().Err(err).Msg("Failed to get current account information")
	} else {
		userAccount := &UserAccount{
			AccountID:   string(account.ID),
			Username:    account.Username,
			DisplayName: account.DisplayName,
			Acct:        account.Acct,
			Avatar:      account.Avatar,
		}

		if err := s.db.insertUserAccount(userAccount); err != nil {
			zlog.Warn().Err(err).Msg("Failed to store user account information")
			// Don't fail the creation if we can't store the account info
		} else {
			zlog.Info().Str("account_id", userAccount.AccountID).Str("username", userAccount.Username).Msg("Stored user account information")
		}
	}

	maxRetries := 3
	client := newMastodonBookmarkClient(madonClient, maxRetries)
	return client, nil
}

func (s *BookmarkService) runBackfill() error {
	zlog.Info().Msg("Starting bookmark backfill")

	state, err := s.db.getBackfillState()
	if err != nil {
		return fmt.Errorf("failed to get backfill state: %w", err)
	}

	if state.BackfillComplete {
		zlog.Info().Msg("Backfill already complete, skipping")
		return nil
	}

	batchSize := s.config.Polling.BatchSize
	if batchSize <= 0 {
		batchSize = 40
	}

	nextURL := state.LastProcessedID
	if nextURL == "" {
		zlog.Info().Msg("Starting backfill from the beginning")
	} else {
		zlog.Info().Str("next_url", nextURL).Msg("Resuming backfill from last position")
	}

	totalProcessed := 0

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		zlog.Debug().Str("next_url", nextURL).Int("batch_size", batchSize).Msg("Fetching bookmark batch")

		bookmarks, newNextURL, err := s.client.GetBookmarks(s.ctx, batchSize, nextURL)
		if err != nil {
			return fmt.Errorf("failed to fetch bookmarks: %w", err)
		}

		if len(bookmarks) == 0 {
			zlog.Info().Int("total_processed", totalProcessed).Msg("Backfill complete - no more bookmarks")
			if err := s.db.updateBackfillState("", true, nil); err != nil {
				return fmt.Errorf("failed to mark backfill complete: %w", err)
			}
			break
		}

		if newNextURL == "" {
			zlog.Info().
				Int("count", len(bookmarks)).
				Int("total_processed", totalProcessed+len(bookmarks)).
				Msg("Processing final bookmark batch - no next URL")

			if err := s.processBookmarkBatch(bookmarks); err != nil {
				return fmt.Errorf("failed to process final bookmark batch: %w", err)
			}

			totalProcessed += len(bookmarks)

			if err := s.db.updateBackfillState("", true, nil); err != nil {
				return fmt.Errorf("failed to mark backfill complete: %w", err)
			}

			zlog.Info().Int("total_bookmarks", totalProcessed).Msg("Backfill completed successfully")
			break
		}

		zlog.Info().
			Int("count", len(bookmarks)).
			Int("total_so_far", totalProcessed).
			Str("next_url", newNextURL).
			Msg("Processing bookmark batch")

		if err := s.processBookmarkBatch(bookmarks); err != nil {
			return fmt.Errorf("failed to process bookmark batch: %w", err)
		}

		totalProcessed += len(bookmarks)

		if err := s.db.updateBackfillState(newNextURL, false, nil); err != nil {
			return fmt.Errorf("failed to update backfill state: %w", err)
		}

		nextURL = newNextURL

		delay := 10 * time.Second
		if s.config.Polling.BackfillDelay != "" {
			if parsedDelay, err := time.ParseDuration(s.config.Polling.BackfillDelay); err == nil {
				delay = parsedDelay
			}
		}

		zlog.Debug().Dur("delay", delay).Msg("Waiting between batches")

		timer := time.NewTimer(delay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return s.ctx.Err()
		case <-timer.C:
		}
	}

	if s.eventChan != nil {
		select {
		case s.eventChan <- ServerEvent{
			Type: "backfill_complete",
			Payload: map[string]interface{}{
				"total_processed": totalProcessed,
			},
		}:
		default:
		}
	}

	zlog.Info().Int("total_bookmarks", totalProcessed).Msg("Backfill completed successfully")
	return nil
}

func (s *BookmarkService) startPolling() error {
	intervalStr := s.config.Polling.Interval
	if intervalStr == "" {
		intervalStr = "5m"
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return fmt.Errorf("invalid polling interval: %w", err)
	}

	if interval <= 0 {
		return fmt.Errorf("polling interval must be positive")
	}

	zlog.Info().Dur("interval", interval).Msg("Starting bookmark polling")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			zlog.Info().Msg("Stopping bookmark polling")
			return s.ctx.Err()
		case <-ticker.C:
			zlog.Debug().Msg("Running scheduled bookmark poll")
			if err := s.pollBookmarks(); err != nil {
				zlog.Error().Err(err).Msg("Bookmark polling failed")
				continue
			}
		}
	}
}

func (s *BookmarkService) pollBookmarks() error {
	zlog.Debug().Msg("Checking for new bookmarks")

	state, err := s.db.getBackfillState()
	if err != nil {
		return fmt.Errorf("failed to get backfill state: %w", err)
	}

	batchSize := s.config.Polling.BatchSize
	if batchSize <= 0 {
		batchSize = 40
	}

	bookmarks, _, err := s.client.GetBookmarks(s.ctx, batchSize, "")
	if err != nil {
		return fmt.Errorf("failed to fetch bookmarks: %w", err)
	}

	if len(bookmarks) == 0 {
		zlog.Debug().Msg("No new bookmarks found")
		return nil
	}

	zlog.Info().Int("count", len(bookmarks)).Msg("Found new bookmarks to process")

	if err := s.processBookmarkBatch(bookmarks); err != nil {
		return fmt.Errorf("failed to process bookmark batch: %w", err)
	}

	now := time.Now()
	if err := s.db.updateBackfillState(state.LastProcessedID, state.BackfillComplete, &now); err != nil {
		return fmt.Errorf("failed to update poll time: %w", err)
	}

	zlog.Info().Int("processed", len(bookmarks)).Time("poll_time", now).Msg("Bookmark polling completed")
	return nil
}

func (s *BookmarkService) processBookmarkBatch(bookmarks []Bookmark) error {
	zlog.Debug().Int("count", len(bookmarks)).Msg("Processing bookmark batch")

	actualProcessed := 0

	if s.eventChan != nil {
		select {
		case s.eventChan <- ServerEvent{
			Type: "batch_start",
			Payload: map[string]interface{}{
				"total_bookmarks": len(bookmarks),
			},
		}:
		default:
		}
	}

	for i, bookmark := range bookmarks {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		zlog.Debug().Int("index", i+1).Int("total", len(bookmarks)).Str("bookmark_id", bookmark.ID).Msg("Processing bookmark")

		existingBookmark, err := s.db.getBookmark(bookmark.Status.ID)
		if err != nil {
			zlog.Error().Err(err).Str("bookmark_id", bookmark.ID).Msg("Failed to check if bookmark exists")
			continue
		}

		if existingBookmark != nil {
			zlog.Debug().Str("bookmark_id", bookmark.ID).Msg("Bookmark already exists in database, skipping")
			continue
		}

		dbBookmark := convertBookmarkToDatabase(bookmark, s.config.Search.IndexedFields)

		if err := s.db.insertBookmark(dbBookmark); err != nil {
			zlog.Error().Err(err).Str("bookmark_id", bookmark.ID).Msg("Failed to insert new bookmark")
			continue
		}

		actualProcessed++
		zlog.Debug().Str("bookmark_id", bookmark.ID).Msg("New bookmark saved to database")

		if s.eventChan != nil {
			select {
			case s.eventChan <- ServerEvent{
				Type: "bookmark_processed",
				Payload: map[string]interface{}{
					"bookmark_id":     bookmark.ID,
					"status_id":       bookmark.Status.ID,
					"username":        bookmark.Status.Account.Username,
					"content_preview": stripHTML(bookmark.Status.Content)[:minInt(100, len(stripHTML(bookmark.Status.Content)))],
					"processed_count": actualProcessed,
					"total_count":     len(bookmarks),
				},
			}:
			default:
			}
		}
	}

	if s.eventChan != nil {
		select {
		case s.eventChan <- ServerEvent{
			Type: "batch_complete",
			Payload: map[string]interface{}{
				"processed": actualProcessed,
				"total":     len(bookmarks),
				"skipped":   len(bookmarks) - actualProcessed,
			},
		}:
		default:
		}
	}

	zlog.Info().Int("processed", actualProcessed).Int("total", len(bookmarks)).Int("skipped", len(bookmarks)-actualProcessed).Msg("Bookmark batch processing completed")

	return nil
}

// =============================================================================
// WEB SERVER AND EVENTS
// =============================================================================

type ServerEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type EventBroadcaster struct {
	clients  map[chan ServerEvent]bool
	mutex    sync.RWMutex
	shutdown bool
}

func newEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		clients: make(map[chan ServerEvent]bool),
	}
}

func (eb *EventBroadcaster) addClient(client chan ServerEvent) {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()
	eb.clients[client] = true
}

func (eb *EventBroadcaster) removeClient(client chan ServerEvent) {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	if eb.shutdown {
		return
	}

	if _, exists := eb.clients[client]; exists {
		delete(eb.clients, client)
		close(client)
	}
}

func (eb *EventBroadcaster) broadcast(event ServerEvent) {
	eb.mutex.RLock()
	defer eb.mutex.RUnlock()

	if eb.shutdown {
		return
	}

	for client := range eb.clients {
		select {
		case client <- event:
		default:
		}
	}
}

func (eb *EventBroadcaster) closeAllClients() {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	eb.shutdown = true
	for client := range eb.clients {
		close(client)
	}
	eb.clients = make(map[chan ServerEvent]bool)
}

type WebServer struct {
	config      *Config
	db          *Database
	broadcaster *EventBroadcaster
	server      *http.Server
}

func newWebServer(cfg *Config, db *Database, eventChan <-chan ServerEvent) *WebServer {
	broadcaster := newEventBroadcaster()

	go func() {
		for event := range eventChan {
			broadcaster.broadcast(event)
		}
	}()

	return &WebServer{
		config:      cfg,
		db:          db,
		broadcaster: broadcaster,
	}
}

func (ws *WebServer) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	webSubFS, err := fs.Sub(webFS, "web")
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to create web sub-filesystem")
		return mux
	}

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(webSubFS))))
	mux.HandleFunc("/", ws.handleIndex)
	mux.HandleFunc("/api/search", ws.handleSearch)
	mux.HandleFunc("/api/stats", ws.handleStats)
	mux.HandleFunc("/api/events", ws.handleEvents)

	return mux
}

func (ws *WebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		zlog.Error().Err(err).Msg("Failed to read index.html")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	if _, err := w.Write(data); err != nil {
		zlog.Error().Err(err).Msg("failed to write response data")
	}
}

func (ws *WebServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	results, err := ws.db.searchOrRecentBookmarks(&request)
	if err != nil {
		zlog.Error().Err(err).Str("query", request.Query).Msg("Search failed")
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	if err := json.NewEncoder(w).Encode(results); err != nil {
		zlog.Error().Err(err).Msg("Failed to encode search results")
	}
}

func (ws *WebServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var totalCount int
	err := ws.db.db.QueryRow("SELECT COUNT(*) FROM bookmarks").Scan(&totalCount)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to get bookmark count")
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	backfillState, err := ws.db.getBackfillState()
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to get backfill state")
		backfillState = &BackfillState{}
	}

	stats := map[string]interface{}{
		"total_bookmarks":   totalCount,
		"backfill_complete": backfillState.BackfillComplete,
		"last_poll_time":    backfillState.LastPollTime,
		"updated_at":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		zlog.Error().Err(err).Msg("Failed to encode stats")
	}
}

func (ws *WebServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
	w.Header().Set("X-Accel-Buffering", "no")

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	client := make(chan ServerEvent, 50)
	ws.broadcaster.addClient(client)
	defer ws.broadcaster.removeClient(client)

	ctx := r.Context()

	fmt.Fprint(w, "data: {\"type\":\"connected\"}\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	go func() {
		var totalCount int
		if err := ws.db.db.QueryRow("SELECT COUNT(*) FROM bookmarks").Scan(&totalCount); err == nil {
			select {
			case client <- ServerEvent{
				Type: "stats",
				Payload: map[string]interface{}{
					"total_bookmarks": totalCount,
				},
			}:
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-client:
			if !ok {
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				zlog.Debug().Err(err).Msg("Failed to marshal SSE event")
				continue
			}

			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

		case <-ticker.C:
			if _, err := fmt.Fprint(w, "data: {\"type\":\"heartbeat\"}\n\n"); err != nil {
				return
			}

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

func (ws *WebServer) start() error {
	addr := fmt.Sprintf("%s:%d", ws.config.Web.Listen, ws.config.Web.Port)

	ws.server = &http.Server{
		Addr:         addr,
		Handler:      ws.setupRoutes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	zlog.Info().Str("address", addr).Msg("Starting web server")

	go func() {
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zlog.Error().Err(err).Msg("Web server error")
		}
	}()

	return nil
}

func (ws *WebServer) stop() error {
	if ws.server == nil {
		return nil
	}

	zlog.Info().Msg("Stopping web server")

	if ws.broadcaster != nil {
		ws.broadcaster.closeAllClients()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := ws.server.Shutdown(ctx); err != nil {
		if err := ws.server.Close(); err != nil {
			return fmt.Errorf("failed to force close server: %w", err)
		}
		zlog.Debug().Msg("Web server force closed after graceful shutdown timeout")
		return nil
	}

	return nil
}

// =============================================================================
// MAIN APPLICATION
// =============================================================================

type BookmarchiveApp struct {
	config          Config
	ctx             context.Context
	cancel          context.CancelFunc
	db              *Database
	mastodonClient  *MastodonClient
	bookmarkService *BookmarkService
	webServer       *WebServer
	eventChan       chan ServerEvent
}

func newBookmarchiveApp(cfg *Config) (*BookmarchiveApp, error) {
	zlog.Info().Msg("Starting bookmarchive service")

	ctx, cancel := context.WithCancel(context.Background())

	db, err := newDatabase(*cfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	mastodonClient, err := newMastodonClient(cfg)
	if err != nil {
		db.close()
		cancel()
		return nil, fmt.Errorf("failed to create mastodon client: %w", err)
	}

	eventChan := make(chan ServerEvent, 100)

	bookmarkService, err := newBookmarkService(cfg, db, eventChan)
	if err != nil {
		db.close()
		cancel()
		return nil, fmt.Errorf("failed to create bookmark service: %w", err)
	}

	webServer := newWebServer(cfg, db, eventChan)

	return &BookmarchiveApp{
		config:          *cfg,
		ctx:             ctx,
		cancel:          cancel,
		db:              db,
		mastodonClient:  mastodonClient,
		bookmarkService: bookmarkService,
		webServer:       webServer,
		eventChan:       eventChan,
	}, nil
}

func (app *BookmarchiveApp) start() error {
	zlog.Info().Msg("Bookmarchive service started")

	if err := app.webServer.start(); err != nil {
		return fmt.Errorf("failed to start web server: %w", err)
	}

	go func() {
		if err := app.bookmarkService.start(); err != nil {
			if err == context.Canceled {
				zlog.Debug().Msg("Bookmark service stopped due to context cancellation")
			} else {
				zlog.Error().Err(err).Msg("Bookmark service error")
			}
		}
	}()

	return nil
}

func (app *BookmarchiveApp) run() error {
	if err := app.start(); err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	zlog.Info().Msg("Shutdown signal received")

	return app.stop()
}

func (app *BookmarchiveApp) stop() error {
	zlog.Info().Msg("Stopping bookmarchive service")

	if app.cancel != nil {
		app.cancel()
	}

	if app.eventChan != nil {
		close(app.eventChan)
	}

	if app.bookmarkService != nil {
		if err := app.bookmarkService.stop(); err != nil {
			zlog.Error().Err(err).Msg("Error stopping bookmark service")
		}
	}

	if app.webServer != nil {
		if err := app.webServer.stop(); err != nil {
			zlog.Debug().Err(err).Msg("Web server stop completed with timeout - this is normal during shutdown")
		}
	}

	if app.db != nil {
		if err := app.db.close(); err != nil {
			zlog.Error().Err(err).Msg("Error closing database")
		}
	}

	zlog.Info().Msg("Bookmarchive service stopped")
	return nil
}

// =============================================================================
// MAIN ENTRY POINT
// =============================================================================

func main() {
	configPath := flag.String("config", "config.toml", "path to configuration file")
	logLevel := flag.String("l", "", "log level (trace, debug, info, warn, error, fatal, panic)")
	logLevelLong := flag.String("log-level", "", "log level (trace, debug, info, warn, error, fatal, panic)")
	showVersion := flag.Bool("version", false, "show version information")
	showHelp := flag.Bool("help", false, "show help information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("bookmarchive %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built: %s\n", date)
		fmt.Printf("  built by: %s\n", builtBy)
		return
	}

	if *showHelp {
		fmt.Println("bookmarchive - Archive and search your Fediverse bookmarks")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s [options]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Configuration:")
		fmt.Println("  Copy config.toml.sample to config.toml and edit as needed.")
		fmt.Println("  The application will create a SQLite database at the configured path.")
		fmt.Println()
		fmt.Printf("Version: %s (%s)\n", version, commit)
		return
	}

	var cliLogLevel string
	if *logLevel != "" {
		cliLogLevel = *logLevel
	} else if *logLevelLong != "" {
		cliLogLevel = *logLevelLong
	}

	if cliLogLevel != "" {
		validLevels := validLogLevels()
		valid := false
		for _, level := range validLevels {
			if strings.ToLower(cliLogLevel) == level {
				valid = true
				break
			}
		}
		if !valid {
			log.Fatalf("Invalid log level: %s. Valid levels: %s", cliLogLevel, strings.Join(validLevels, ", "))
		}
	}

	cfg := defaultConfig()
	if err := loadConfig(*configPath, &cfg); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logLevelToUse := cfg.Logging.Level
	if cliLogLevel != "" {
		logLevelToUse = cliLogLevel
	}
	setupLogging(logLevelToUse, cfg.Logging.Format)

	app, err := newBookmarchiveApp(&cfg)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	if err := app.run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
