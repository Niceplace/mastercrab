package gmail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cli/main/pkg/gmail/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDeleter(t *testing.T) (*Deleter, *cache.Cache) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cacheInstance, err := cache.NewCache(dbPath)
	require.NoError(t, err)

	deleter := NewDeleter(nil, cacheInstance)
	return deleter, cacheInstance
}

func createTestEmails(t *testing.T, cacheInstance *cache.Cache) {
	now := time.Now()
	emails := []*cache.EmailMessage{
		{
			ID:           "msg-001",
			LabelIDs:     []string{"INBOX"},
			Snippet:      "Email from GitHub",
			SizeEstimate: 5000,
			InternalDate: now.Add(-48 * time.Hour).Unix() * 1000,
			Headers: map[string]string{
				"From":    "notifications@github.com",
				"To":      "user@example.com",
				"Subject": "New pull request",
				"Date":    now.Add(-48 * time.Hour).Format(time.RFC1123Z),
			},
		},
		{
			ID:           "msg-002",
			LabelIDs:     []string{"INBOX", "IMPORTANT"},
			Snippet:      "Email from LinkedIn",
			SizeEstimate: 3000,
			InternalDate: now.Add(-24 * time.Hour).Unix() * 1000,
			Headers: map[string]string{
				"From":    "notifications@linkedin.com",
				"To":      "user@example.com",
				"Subject": "Connection request",
				"Date":    now.Add(-24 * time.Hour).Format(time.RFC1123Z),
			},
		},
		{
			ID:           "msg-003",
			LabelIDs:     []string{"INBOX"},
			Snippet:      "Email from work",
			SizeEstimate: 10000,
			InternalDate: now.Add(-1 * time.Hour).Unix() * 1000,
			Headers: map[string]string{
				"From":    "boss@company.com",
				"To":      "user@example.com",
				"Subject": "Important meeting",
				"Date":    now.Add(-1 * time.Hour).Format(time.RFC1123Z),
			},
		},
	}

	err := cacheInstance.UpsertBatch(emails)
	require.NoError(t, err)
}

func TestNewDeleter(t *testing.T) {
	deleter, cacheInstance := setupTestDeleter(t)
	defer cacheInstance.Close()

	assert.NotNil(t, deleter)
	assert.NotNil(t, deleter.cache)
}

func TestBuildPreview(t *testing.T) {
	deleter, cacheInstance := setupTestDeleter(t)
	defer cacheInstance.Close()

	createTestEmails(t, cacheInstance)

	t.Run("generates preview without filters", func(t *testing.T) {
		preview, err := deleter.BuildPreview(nil)
		require.NoError(t, err)

		assert.Equal(t, 3, preview.EmailCount)
		assert.Equal(t, int64(18000), preview.TotalSize)
		assert.Contains(t, preview.SafetyLabel, "crab-deleted-")
		assert.False(t, preview.GeneratedAt.IsZero())
	})

	t.Run("filters by sender email", func(t *testing.T) {
		filters := &DeleteFilters{
			SenderEmails: []string{"notifications@github.com"},
		}

		preview, err := deleter.BuildPreview(filters)
		require.NoError(t, err)

		assert.Equal(t, 1, preview.EmailCount)
		assert.Equal(t, int64(5000), preview.TotalSize)
		assert.Equal(t, "msg-001", preview.Emails[0].ID)
	})

	t.Run("filters by sender domain", func(t *testing.T) {
		filters := &DeleteFilters{
			SenderDomains: []string{"github.com", "linkedin.com"},
		}

		preview, err := deleter.BuildPreview(filters)
		require.NoError(t, err)

		assert.Equal(t, 2, preview.EmailCount)
		assert.Equal(t, int64(8000), preview.TotalSize)
	})

	t.Run("filters by date range", func(t *testing.T) {
		dateBefore := time.Now().Add(-12 * time.Hour)
		filters := &DeleteFilters{
			DateBefore: &dateBefore,
		}

		preview, err := deleter.BuildPreview(filters)
		require.NoError(t, err)

		assert.Equal(t, 2, preview.EmailCount) // Only emails from 48h and 24h ago
	})

	t.Run("filters by size", func(t *testing.T) {
		filters := &DeleteFilters{
			SizeGreaterThan: 3999,
		}

		preview, err := deleter.BuildPreview(filters)
		require.NoError(t, err)

		assert.Equal(t, 2, preview.EmailCount) // 5000 and 10000 bytes
	})

	t.Run("filters by labels", func(t *testing.T) {
		filters := &DeleteFilters{
			Labels: []string{"IMPORTANT"},
		}

		preview, err := deleter.BuildPreview(filters)
		require.NoError(t, err)

		assert.Equal(t, 1, preview.EmailCount)
		assert.Equal(t, "msg-002", preview.Emails[0].ID)
	})

	t.Run("filters by excluded labels", func(t *testing.T) {
		filters := &DeleteFilters{
			ExcludeLabels: []string{"IMPORTANT"},
		}

		preview, err := deleter.BuildPreview(filters)
		require.NoError(t, err)

		assert.Equal(t, 2, preview.EmailCount) // msg-001 and msg-003
	})

	t.Run("combines multiple filters", func(t *testing.T) {
		filters := &DeleteFilters{
			SenderDomains:   []string{"github.com", "linkedin.com"},
			SizeGreaterThan: 3999,
		}

		preview, err := deleter.BuildPreview(filters)
		require.NoError(t, err)

		assert.Equal(t, 1, preview.EmailCount) // Only GitHub email (5000 bytes)
		assert.Equal(t, "msg-001", preview.Emails[0].ID)
	})
}

func TestExportToJSON(t *testing.T) {
	deleter, cacheInstance := setupTestDeleter(t)
	defer cacheInstance.Close()

	createTestEmails(t, cacheInstance)

	preview, err := deleter.BuildPreview(nil)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "preview.json")

	err = deleter.ExportToJSON(preview, jsonPath)
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(jsonPath)
	assert.NoError(t, err)

	// Verify file content is valid JSON
	data, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "email_count")
	assert.Contains(t, string(data), "safety_label")
}

func TestExportToCSV(t *testing.T) {
	deleter, cacheInstance := setupTestDeleter(t)
	defer cacheInstance.Close()

	createTestEmails(t, cacheInstance)

	preview, err := deleter.BuildPreview(nil)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "preview.csv")

	err = deleter.ExportToCSV(preview, csvPath)
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(csvPath)
	assert.NoError(t, err)

	// Verify file content has CSV header
	data, err := os.ReadFile(csvPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "ID,From Email,From Name")
	assert.Contains(t, string(data), "notifications@github.com")
}

func TestContainsDomain(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		domain   string
		expected bool
	}{
		{
			name:     "matches domain",
			email:    "user@example.com",
			domain:   "example.com",
			expected: true,
		},
		{
			name:     "does not match domain",
			email:    "user@example.com",
			domain:   "other.com",
			expected: false,
		},
		{
			name:     "no @ symbol",
			email:    "invalid-email",
			domain:   "example.com",
			expected: false,
		},
		{
			name:     "empty email",
			email:    "",
			domain:   "example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsDomain(tt.email, tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsLabel(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		target   string
		expected bool
	}{
		{
			name:     "label exists",
			labels:   []string{"INBOX", "IMPORTANT", "UNREAD"},
			target:   "IMPORTANT",
			expected: true,
		},
		{
			name:     "label does not exist",
			labels:   []string{"INBOX", "UNREAD"},
			target:   "IMPORTANT",
			expected: false,
		},
		{
			name:     "empty labels",
			labels:   []string{},
			target:   "INBOX",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsLabel(tt.labels, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesFilters(t *testing.T) {
	deleter, cacheInstance := setupTestDeleter(t)
	defer cacheInstance.Close()

	now := time.Now()
	email := &cache.CachedEmail{
		ID:        "msg-001",
		FromEmail: "test@example.com",
		Subject:   "Test Subject",
		Date:      now.Add(-24 * time.Hour),
		Size:      5000,
		Labels:    []string{"INBOX", "UNREAD"},
	}

	t.Run("no filters matches all", func(t *testing.T) {
		filters := &DeleteFilters{}
		assert.True(t, deleter.matchesFilters(email, filters))
	})

	t.Run("sender email filter matches", func(t *testing.T) {
		filters := &DeleteFilters{
			SenderEmails: []string{"test@example.com"},
		}
		assert.True(t, deleter.matchesFilters(email, filters))
	})

	t.Run("sender email filter does not match", func(t *testing.T) {
		filters := &DeleteFilters{
			SenderEmails: []string{"other@example.com"},
		}
		assert.False(t, deleter.matchesFilters(email, filters))
	})

	t.Run("date before filter matches", func(t *testing.T) {
		dateBefore := now
		filters := &DeleteFilters{
			DateBefore: &dateBefore,
		}
		assert.True(t, deleter.matchesFilters(email, filters))
	})

	t.Run("date before filter does not match", func(t *testing.T) {
		dateBefore := now.Add(-48 * time.Hour)
		filters := &DeleteFilters{
			DateBefore: &dateBefore,
		}
		assert.False(t, deleter.matchesFilters(email, filters))
	})

	t.Run("size filter matches", func(t *testing.T) {
		filters := &DeleteFilters{
			SizeGreaterThan: 4000,
			SizeLessThan:    6000,
		}
		assert.True(t, deleter.matchesFilters(email, filters))
	})

	t.Run("size filter does not match", func(t *testing.T) {
		filters := &DeleteFilters{
			SizeGreaterThan: 10000,
		}
		assert.False(t, deleter.matchesFilters(email, filters))
	})

	t.Run("label filter matches", func(t *testing.T) {
		filters := &DeleteFilters{
			Labels: []string{"INBOX"},
		}
		assert.True(t, deleter.matchesFilters(email, filters))
	})

	t.Run("exclude label filter matches", func(t *testing.T) {
		filters := &DeleteFilters{
			ExcludeLabels: []string{"IMPORTANT"},
		}
		assert.True(t, deleter.matchesFilters(email, filters))
	})

	t.Run("exclude label filter does not match", func(t *testing.T) {
		filters := &DeleteFilters{
			ExcludeLabels: []string{"UNREAD"},
		}
		assert.False(t, deleter.matchesFilters(email, filters))
	})
}
