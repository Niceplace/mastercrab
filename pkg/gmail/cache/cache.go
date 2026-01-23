package cache

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Cache provides SQLite-based caching for Gmail messages
type Cache struct {
	db     *sql.DB
	dbPath string
}

// CachedEmail represents an email in the cache
type CachedEmail struct {
	ID         string
	FromEmail  string
	FromName   string
	ToEmail    string
	Subject    string
	Date       time.Time
	Size       int64
	Labels     []string
	Snippet    string
	LastSynced time.Time
}

// EmailMessage represents a message to be cached (decoupled from gmail package)
type EmailMessage struct {
	ID           string
	LabelIDs     []string
	Snippet      string
	SizeEstimate int64
	InternalDate int64 // Unix milliseconds
	Headers      map[string]string
}

// CacheStats provides cache statistics
type CacheStats struct {
	TotalEmails int64
	TotalSize   int64
	OldestEmail time.Time
	NewestEmail time.Time
	LastSync    time.Time
}

// NewCache creates or opens a cache database
func NewCache(dbPath string) (*Cache, error) {
	// Expand path
	dbPath = expandPath(dbPath)

	// Create parent directories
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Open SQLite connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Execute schema
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &Cache{db: db, dbPath: dbPath}, nil
}

// Close closes the database connection
func (c *Cache) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// UpsertEmail inserts or updates an email in the cache
func (c *Cache) UpsertEmail(email *EmailMessage) error {
	// Serialize labels to JSON
	labelsJSON, err := json.Marshal(email.LabelIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	// Extract from email and name
	from := email.Headers["From"]
	fromEmail := extractEmailAddress(from)
	fromName := from

	// Get date from headers or fallback to internal date
	date := time.Unix(email.InternalDate/1000, 0)
	if dateStr, ok := email.Headers["Date"]; ok {
		if parsedDate, err := time.Parse(time.RFC1123Z, dateStr); err == nil {
			date = parsedDate
		}
	}

	// Insert or replace
	query := `INSERT OR REPLACE INTO emails
		(id, from_email, from_name, to_email, subject, date, size, labels, snippet, last_synced)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = c.db.Exec(query,
		email.ID,
		fromEmail,
		fromName,
		email.Headers["To"],
		email.Headers["Subject"],
		date.Unix(),
		email.SizeEstimate,
		string(labelsJSON),
		email.Snippet,
		time.Now().Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to upsert email: %w", err)
	}

	return nil
}

// UpsertBatch inserts or updates multiple emails efficiently
func (c *Cache) UpsertBatch(emails []*EmailMessage) error {
	if len(emails) == 0 {
		return nil
	}

	// Start transaction
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement
	query := `INSERT OR REPLACE INTO emails
		(id, from_email, from_name, to_email, subject, date, size, labels, snippet, last_synced)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Execute for each email
	now := time.Now().Unix()
	for _, email := range emails {
		labelsJSON, err := json.Marshal(email.LabelIDs)
		if err != nil {
			continue // Skip this email
		}

		from := email.Headers["From"]
		fromEmail := extractEmailAddress(from)

		// Get date from headers or fallback to internal date
		date := time.Unix(email.InternalDate/1000, 0)
		if dateStr, ok := email.Headers["Date"]; ok {
			if parsedDate, err := time.Parse(time.RFC1123Z, dateStr); err == nil {
				date = parsedDate
			}
		}

		_, err = stmt.Exec(
			email.ID,
			fromEmail,
			from,
			email.Headers["To"],
			email.Headers["Subject"],
			date.Unix(),
			email.SizeEstimate,
			string(labelsJSON),
			email.Snippet,
			now,
		)
		if err != nil {
			// Log but continue
			fmt.Printf("⚠️  Failed to insert email %s: %s\n", email.ID, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetExistingMessageIDs checks which message IDs already exist in the cache
func (c *Cache) GetExistingMessageIDs(ids []string) (map[string]bool, error) {
	if len(ids) == 0 {
		return make(map[string]bool), nil
	}

	existing := make(map[string]bool)

	// Query in batches to avoid SQL parameter limits
	batchSize := 500
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		// Build parameterized query
		placeholders := make([]string, len(batch))
		args := make([]interface{}, len(batch))
		for j, id := range batch {
			placeholders[j] = "?"
			args[j] = id
		}

		query := fmt.Sprintf("SELECT id FROM emails WHERE id IN (%s)", strings.Join(placeholders, ","))
		rows, err := c.db.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query existing IDs: %w", err)
		}

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan ID: %w", err)
			}
			existing[id] = true
		}
		rows.Close()
	}

	return existing, nil
}

// GetCachedEmails retrieves all cached emails
func (c *Cache) GetCachedEmails() ([]*CachedEmail, error) {
	query := `SELECT id, from_email, from_name, to_email, subject, date, size, labels, snippet, last_synced
		FROM emails ORDER BY date DESC`

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query emails: %w", err)
	}
	defer rows.Close()

	var emails []*CachedEmail
	for rows.Next() {
		var email CachedEmail
		var dateUnix, lastSyncedUnix int64
		var labelsJSON string

		err := rows.Scan(
			&email.ID,
			&email.FromEmail,
			&email.FromName,
			&email.ToEmail,
			&email.Subject,
			&dateUnix,
			&email.Size,
			&labelsJSON,
			&email.Snippet,
			&lastSyncedUnix,
		)
		if err != nil {
			continue // Skip this row
		}

		// Deserialize labels
		if err := json.Unmarshal([]byte(labelsJSON), &email.Labels); err != nil {
			email.Labels = []string{} // Default to empty array
		}

		// Convert timestamps
		email.Date = time.Unix(dateUnix, 0)
		email.LastSynced = time.Unix(lastSyncedUnix, 0)

		emails = append(emails, &email)
	}

	return emails, nil
}

// DeleteEmail removes an email from the cache
func (c *Cache) DeleteEmail(id string) error {
	_, err := c.db.Exec("DELETE FROM emails WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete email: %w", err)
	}
	return nil
}

// GetLastSyncTime returns when the cache was last synced
func (c *Cache) GetLastSyncTime() (time.Time, error) {
	var valueStr string
	err := c.db.QueryRow("SELECT value FROM sync_metadata WHERE key = ?", "last_sync").Scan(&valueStr)
	if err == sql.ErrNoRows {
		return time.Time{}, nil // No sync yet
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get last sync time: %w", err)
	}

	var timestamp int64
	if err := json.Unmarshal([]byte(valueStr), &timestamp); err != nil {
		return time.Time{}, fmt.Errorf("failed to parse sync time: %w", err)
	}

	return time.Unix(timestamp, 0), nil
}

// UpdateSyncTime updates the last sync timestamp
func (c *Cache) UpdateSyncTime() error {
	now := time.Now().Unix()
	valueJSON, err := json.Marshal(now)
	if err != nil {
		return fmt.Errorf("failed to marshal sync time: %w", err)
	}

	_, err = c.db.Exec(
		"INSERT OR REPLACE INTO sync_metadata (key, value, updated_at) VALUES (?, ?, ?)",
		"last_sync",
		string(valueJSON),
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to update sync time: %w", err)
	}

	return nil
}

// GetStats returns cache statistics
func (c *Cache) GetStats() (*CacheStats, error) {
	stats := &CacheStats{}

	// Get counts and sums
	var oldestUnix, newestUnix int64
	err := c.db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(size), 0),
			COALESCE(MIN(date), 0),
			COALESCE(MAX(date), 0)
		FROM emails
	`).Scan(&stats.TotalEmails, &stats.TotalSize, &oldestUnix, &newestUnix)

	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	// Convert timestamps
	if oldestUnix > 0 {
		stats.OldestEmail = time.Unix(oldestUnix, 0)
	}
	if newestUnix > 0 {
		stats.NewestEmail = time.Unix(newestUnix, 0)
	}

	// Get last sync
	stats.LastSync, _ = c.GetLastSyncTime()

	return stats, nil
}

// Helper function to expand ~ in paths
func expandPath(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// extractEmailAddress extracts the email address from a "Name <email>" format
func extractEmailAddress(from string) string {
	if from == "" {
		return ""
	}

	// Look for < and > brackets
	startIdx := strings.IndexByte(from, '<')
	endIdx := strings.IndexByte(from, '>')

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		return from[startIdx+1 : endIdx]
	}

	// No brackets, assume it's just the email
	return from
}
