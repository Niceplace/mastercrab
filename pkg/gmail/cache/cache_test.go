package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCache(t *testing.T) {
	t.Run("creates new cache successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		cache, err := NewCache(dbPath)
		require.NoError(t, err)
		require.NotNil(t, cache)
		defer cache.Close()

		// Verify database file was created
		_, err = os.Stat(dbPath)
		assert.NoError(t, err)
	})

	t.Run("expands tilde in path", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Use relative path for testing
		dbPath := filepath.Join(tmpDir, "test.db")

		cache, err := NewCache(dbPath)
		require.NoError(t, err)
		defer cache.Close()

		assert.NotNil(t, cache)
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "nested", "dirs", "test.db")

		cache, err := NewCache(dbPath)
		require.NoError(t, err)
		defer cache.Close()

		// Verify nested directories were created
		_, err = os.Stat(filepath.Dir(dbPath))
		assert.NoError(t, err)
	})
}

func TestUpsertEmail(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cache, err := NewCache(dbPath)
	require.NoError(t, err)
	defer cache.Close()

	t.Run("inserts new email", func(t *testing.T) {
		email := &EmailMessage{
			ID:           "msg-001",
			LabelIDs:     []string{"INBOX", "UNREAD"},
			Snippet:      "This is a test email",
			SizeEstimate: 1024,
			InternalDate: time.Now().Unix() * 1000,
			Headers: map[string]string{
				"From":    "John Doe <john@example.com>",
				"To":      "recipient@test.com",
				"Subject": "Test Subject",
				"Date":    time.Now().Format(time.RFC1123Z),
			},
		}

		err := cache.UpsertEmail(email)
		assert.NoError(t, err)

		// Verify email was inserted
		emails, err := cache.GetCachedEmails()
		require.NoError(t, err)
		assert.Len(t, emails, 1)
		assert.Equal(t, "msg-001", emails[0].ID)
		assert.Equal(t, "john@example.com", emails[0].FromEmail)
	})

	t.Run("updates existing email", func(t *testing.T) {
		email := &EmailMessage{
			ID:           "msg-001",
			LabelIDs:     []string{"INBOX"}, // Changed: removed UNREAD
			Snippet:      "Updated snippet",
			SizeEstimate: 2048, // Changed size
			InternalDate: time.Now().Unix() * 1000,
			Headers: map[string]string{
				"From":    "John Doe <john@example.com>",
				"To":      "recipient@test.com",
				"Subject": "Updated Subject",
				"Date":    time.Now().Format(time.RFC1123Z),
			},
		}

		err := cache.UpsertEmail(email)
		assert.NoError(t, err)

		// Verify email was updated (should still be 1 email total)
		emails, err := cache.GetCachedEmails()
		require.NoError(t, err)
		assert.Len(t, emails, 1)
		assert.Equal(t, "Updated Subject", emails[0].Subject)
		assert.Equal(t, int64(2048), emails[0].Size)
		assert.Contains(t, emails[0].Labels, "INBOX")
		assert.NotContains(t, emails[0].Labels, "UNREAD")
	})
}

func TestUpsertBatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cache, err := NewCache(dbPath)
	require.NoError(t, err)
	defer cache.Close()

	t.Run("inserts multiple emails efficiently", func(t *testing.T) {
		emails := make([]*EmailMessage, 100)
		for i := 0; i < 100; i++ {
			emails[i] = &EmailMessage{
				ID:           "msg-" + string(rune(i)),
				LabelIDs:     []string{"INBOX"},
				Snippet:      "Test email " + string(rune(i)),
				SizeEstimate: int64(1024 * (i + 1)),
				InternalDate: time.Now().Unix() * 1000,
				Headers: map[string]string{
					"From":    "sender" + string(rune(i)) + "@example.com",
					"To":      "recipient@test.com",
					"Subject": "Test " + string(rune(i)),
					"Date":    time.Now().Format(time.RFC1123Z),
				},
			}
		}

		err := cache.UpsertBatch(emails)
		assert.NoError(t, err)

		// Verify all emails were inserted
		cached, err := cache.GetCachedEmails()
		require.NoError(t, err)
		assert.Equal(t, 100, len(cached))
	})

	t.Run("handles empty batch", func(t *testing.T) {
		err := cache.UpsertBatch([]*EmailMessage{})
		assert.NoError(t, err)
	})
}

func TestGetCachedEmails(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cache, err := NewCache(dbPath)
	require.NoError(t, err)
	defer cache.Close()

	// Insert test data
	now := time.Now()
	emails := []*EmailMessage{
		{
			ID:           "msg-001",
			LabelIDs:     []string{"INBOX"},
			Snippet:      "Email 1",
			SizeEstimate: 1024,
			InternalDate: now.Add(-24 * time.Hour).Unix() * 1000,
			Headers: map[string]string{
				"From":    "sender1@example.com",
				"Subject": "Test 1",
				"Date":    now.Add(-24 * time.Hour).Format(time.RFC1123Z),
			},
		},
		{
			ID:           "msg-002",
			LabelIDs:     []string{"INBOX", "IMPORTANT"},
			Snippet:      "Email 2",
			SizeEstimate: 2048,
			InternalDate: now.Unix() * 1000,
			Headers: map[string]string{
				"From":    "sender2@example.com",
				"Subject": "Test 2",
				"Date":    now.Format(time.RFC1123Z),
			},
		},
	}

	err = cache.UpsertBatch(emails)
	require.NoError(t, err)

	t.Run("retrieves all emails", func(t *testing.T) {
		cached, err := cache.GetCachedEmails()
		require.NoError(t, err)
		assert.Len(t, cached, 2)
	})

	t.Run("emails are ordered by date descending", func(t *testing.T) {
		cached, err := cache.GetCachedEmails()
		require.NoError(t, err)

		// Most recent email should be first
		assert.Equal(t, "msg-002", cached[0].ID)
		assert.Equal(t, "msg-001", cached[1].ID)
	})

	t.Run("deserializes labels correctly", func(t *testing.T) {
		cached, err := cache.GetCachedEmails()
		require.NoError(t, err)

		// Check labels for second email
		assert.Contains(t, cached[0].Labels, "INBOX")
		assert.Contains(t, cached[0].Labels, "IMPORTANT")
	})
}

func TestDeleteEmail(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cache, err := NewCache(dbPath)
	require.NoError(t, err)
	defer cache.Close()

	// Insert test data
	email := &EmailMessage{
		ID:           "msg-001",
		LabelIDs:     []string{"INBOX"},
		Snippet:      "Test",
		SizeEstimate: 1024,
		InternalDate: time.Now().Unix() * 1000,
		Headers: map[string]string{
			"From":    "sender@example.com",
			"Subject": "Test",
			"Date":    time.Now().Format(time.RFC1123Z),
		},
	}
	err = cache.UpsertEmail(email)
	require.NoError(t, err)

	t.Run("deletes email successfully", func(t *testing.T) {
		err := cache.DeleteEmail("msg-001")
		assert.NoError(t, err)

		// Verify email was deleted
		emails, err := cache.GetCachedEmails()
		require.NoError(t, err)
		assert.Len(t, emails, 0)
	})

	t.Run("handles non-existent email", func(t *testing.T) {
		err := cache.DeleteEmail("msg-999")
		assert.NoError(t, err) // Should not error
	})
}

func TestSyncTime(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cache, err := NewCache(dbPath)
	require.NoError(t, err)
	defer cache.Close()

	t.Run("returns zero time when never synced", func(t *testing.T) {
		syncTime, err := cache.GetLastSyncTime()
		assert.NoError(t, err)
		assert.True(t, syncTime.IsZero())
	})

	t.Run("updates and retrieves sync time", func(t *testing.T) {
		before := time.Now()

		err := cache.UpdateSyncTime()
		assert.NoError(t, err)

		after := time.Now()

		syncTime, err := cache.GetLastSyncTime()
		require.NoError(t, err)

		// Sync time should be between before and after
		assert.True(t, syncTime.After(before.Add(-time.Second)))
		assert.True(t, syncTime.Before(after.Add(time.Second)))
	})

	t.Run("updates sync time multiple times", func(t *testing.T) {
		before := time.Now()

		err := cache.UpdateSyncTime()
		require.NoError(t, err)

		firstSync, err := cache.GetLastSyncTime()
		require.NoError(t, err)

		err = cache.UpdateSyncTime()
		require.NoError(t, err)

		secondSync, err := cache.GetLastSyncTime()
		require.NoError(t, err)

		after := time.Now()

		// Both syncs should be within the test timeframe
		assert.True(t, firstSync.After(before.Add(-time.Second)))
		assert.True(t, firstSync.Before(after.Add(time.Second)))
		assert.True(t, secondSync.After(before.Add(-time.Second)))
		assert.True(t, secondSync.Before(after.Add(time.Second)))

		// Second sync should be at or after the first (Unix second precision means they might be equal)
		assert.True(t, !secondSync.Before(firstSync))
	})
}

func TestGetStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cache, err := NewCache(dbPath)
	require.NoError(t, err)
	defer cache.Close()

	t.Run("returns empty stats for empty cache", func(t *testing.T) {
		stats, err := cache.GetStats()
		require.NoError(t, err)
		assert.Equal(t, int64(0), stats.TotalEmails)
		assert.Equal(t, int64(0), stats.TotalSize)
	})

	t.Run("calculates stats correctly", func(t *testing.T) {
		now := time.Now()
		emails := []*EmailMessage{
			{
				ID:           "msg-001",
				LabelIDs:     []string{"INBOX"},
				SizeEstimate: 1000,
				InternalDate: now.Add(-48 * time.Hour).Unix() * 1000,
				Headers: map[string]string{
					"From":    "sender1@example.com",
					"Date":    now.Add(-48 * time.Hour).Format(time.RFC1123Z),
					"Subject": "Test",
				},
			},
			{
				ID:           "msg-002",
				LabelIDs:     []string{"INBOX"},
				SizeEstimate: 2000,
				InternalDate: now.Add(-24 * time.Hour).Unix() * 1000,
				Headers: map[string]string{
					"From":    "sender2@example.com",
					"Date":    now.Add(-24 * time.Hour).Format(time.RFC1123Z),
					"Subject": "Test",
				},
			},
			{
				ID:           "msg-003",
				LabelIDs:     []string{"INBOX"},
				SizeEstimate: 3000,
				InternalDate: now.Unix() * 1000,
				Headers: map[string]string{
					"From":    "sender3@example.com",
					"Date":    now.Format(time.RFC1123Z),
					"Subject": "Test",
				},
			},
		}

		err := cache.UpsertBatch(emails)
		require.NoError(t, err)

		err = cache.UpdateSyncTime()
		require.NoError(t, err)

		stats, err := cache.GetStats()
		require.NoError(t, err)

		assert.Equal(t, int64(3), stats.TotalEmails)
		assert.Equal(t, int64(6000), stats.TotalSize)
		assert.False(t, stats.LastSync.IsZero())

		// Oldest should be from 48 hours ago
		assert.True(t, stats.OldestEmail.Before(stats.NewestEmail))
	})
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "expands home directory",
			input:    "~/test/path",
			expected: filepath.Join(os.Getenv("HOME"), "test/path"),
		},
		{
			name:     "leaves absolute path unchanged",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "leaves relative path unchanged",
			input:    "relative/path",
			expected: "relative/path",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
