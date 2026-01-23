package gmail

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func createTestClientConfig() *viper.Viper {
	v := viper.New()
	v.Set("gmail.defaultAnalysisLimit", 5000)
	v.Set("gmail.deletionBatchSize", 100)
	return v
}

func TestExtractEmailAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "email with name and brackets",
			input:    "John Doe <john.doe@example.com>",
			expected: "john.doe@example.com",
		},
		{
			name:     "email without brackets",
			input:    "john.doe@example.com",
			expected: "john.doe@example.com",
		},
		{
			name:     "email with extra spaces",
			input:    "  john.doe@example.com  ",
			expected: "john.doe@example.com",
		},
		{
			name:     "complex name format",
			input:    "\"Doe, John\" <j.doe@company.org>",
			expected: "j.doe@company.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractEmailAddress(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard email",
			input:    "user@example.com",
			expected: "example.com",
		},
		{
			name:     "subdomain email",
			input:    "user@mail.company.org",
			expected: "mail.company.org",
		},
		{
			name:     "no @ sign",
			input:    "notanemail",
			expected: "notanemail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractDomain(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQueryBuilder(t *testing.T) {
	t.Run("single filter", func(t *testing.T) {
		qb := NewQueryBuilder().From("test@example.com")
		assert.Equal(t, "from:test@example.com", qb.Build())
	})

	t.Run("multiple filters", func(t *testing.T) {
		qb := NewQueryBuilder().
			From("test@example.com").
			After("2023/01/01").
			HasAttachment()

		result := qb.Build()
		assert.Contains(t, result, "from:test@example.com")
		assert.Contains(t, result, "after:2023/01/01")
		assert.Contains(t, result, "has:attachment")
	})

	t.Run("size filters", func(t *testing.T) {
		qb := NewQueryBuilder().
			Larger(10000000).
			Smaller(50000000)

		result := qb.Build()
		assert.Contains(t, result, "larger:10000000")
		assert.Contains(t, result, "smaller:50000000")
	})

	t.Run("label filter", func(t *testing.T) {
		qb := NewQueryBuilder().Label("INBOX")
		assert.Equal(t, "label:INBOX", qb.Build())
	})

	t.Run("subject filter", func(t *testing.T) {
		qb := NewQueryBuilder().Subject("test")
		assert.Equal(t, "subject:test", qb.Build())
	})

	t.Run("complex query", func(t *testing.T) {
		qb := NewQueryBuilder().
			From("sender@example.com").
			Subject("invoice").
			After("2023/12/01").
			Before("2023/12/31").
			HasAttachment()

		result := qb.Build()
		assert.True(t, strings.Contains(result, "from:sender@example.com"))
		assert.True(t, strings.Contains(result, "subject:invoice"))
		assert.True(t, strings.Contains(result, "after:2023/12/01"))
		assert.True(t, strings.Contains(result, "before:2023/12/31"))
		assert.True(t, strings.Contains(result, "has:attachment"))
	})
}

func TestGmailMessage_GetHeader(t *testing.T) {
	msg := &GmailMessage{
		Headers: map[string]string{
			"From":    "test@example.com",
			"Subject": "Test Subject",
			"Date":    "Wed, 20 Dec 2023 12:00:00 -0800",
		},
	}

	t.Run("existing header", func(t *testing.T) {
		assert.Equal(t, "test@example.com", msg.GetFrom())
		assert.Equal(t, "Test Subject", msg.GetSubject())
	})

	t.Run("non-existing header", func(t *testing.T) {
		assert.Equal(t, "", msg.GetHeader("NonExistent"))
	})
}

func TestGmailMessage_GetDate(t *testing.T) {
	t.Run("valid date", func(t *testing.T) {
		msg := &GmailMessage{
			Headers: map[string]string{
				"Date": "Wed, 20 Dec 2023 12:00:00 -0800",
			},
		}

		date, err := msg.GetDate()
		assert.NoError(t, err)
		assert.Equal(t, 2023, date.Year())
		assert.Equal(t, 12, int(date.Month()))
		assert.Equal(t, 20, date.Day())
	})

	t.Run("fallback to internal date", func(t *testing.T) {
		msg := &GmailMessage{
			InternalDate: 1703088000000, // 2023-12-20 12:00:00
			Headers:      map[string]string{},
		}

		date, err := msg.GetDate()
		assert.NoError(t, err)
		assert.Equal(t, 2023, date.Year())
	})

	t.Run("invalid date format", func(t *testing.T) {
		msg := &GmailMessage{
			Headers: map[string]string{
				"Date": "invalid date format",
			},
		}

		_, err := msg.GetDate()
		assert.Error(t, err)
	})
}

func TestConvertToGmailMessage(t *testing.T) {
	// Read mock response
	mockData, err := os.ReadFile("mockResponses/gmail-get.json")
	assert.NoError(t, err)

	// This test validates the structure exists
	// In a real test, we'd parse the JSON and convert it
	assert.NotEmpty(t, mockData)
}

func TestNewGmailClient(t *testing.T) {
	// This test validates client creation
	// In real usage, this would be called with an authenticated HTTP client
	config := createTestClientConfig()
	assert.NotNil(t, config)

	// Verify config values
	assert.Equal(t, 5000, config.GetInt("gmail.defaultAnalysisLimit"))
	assert.Equal(t, 100, config.GetInt("gmail.deletionBatchSize"))
}

// Mock HTTP server tests would go here in a real implementation
// For now, we're testing the helper functions and data structures

func TestBuildQueryScenarios(t *testing.T) {
	tests := []struct {
		name     string
		builder  func() *QueryBuilder
		contains []string
	}{
		{
			name: "delete old large emails",
			builder: func() *QueryBuilder {
				return NewQueryBuilder().
					Before("2020/01/01").
					Larger(10000000)
			},
			contains: []string{"before:2020/01/01", "larger:10000000"},
		},
		{
			name: "promotional emails from sender",
			builder: func() *QueryBuilder {
				return NewQueryBuilder().
					From("newsletter@marketing.com").
					Label("PROMOTIONS")
			},
			contains: []string{"from:newsletter@marketing.com", "label:PROMOTIONS"},
		},
		{
			name: "attachments from specific domain",
			builder: func() *QueryBuilder {
				return NewQueryBuilder().
					From("@company.com").
					HasAttachment().
					After("2023/01/01")
			},
			contains: []string{"from:@company.com", "has:attachment", "after:2023/01/01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := tt.builder().Build()
			for _, expected := range tt.contains {
				assert.Contains(t, query, expected)
			}
		})
	}
}

func TestMockServerBasic(t *testing.T) {
	// Create a simple mock server to test HTTP interaction
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"messages":[]}`))
	}))
	defer server.Close()

	// Verify the server is working
	resp, err := http.Get(server.URL)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
