package gmail

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMimeTypeToExtension(t *testing.T) {
	analyzer := &Analyzer{}

	tests := []struct {
		mimeType string
		expected string
	}{
		{"application/pdf", ".pdf"},
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"application/zip", ".zip"},
		{"application/msword", ".doc"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
		{"application/vnd.ms-excel", ".xls"},
		{"text/plain", ".txt"},
		{"video/mp4", ".mp4"},
		{"audio/mpeg", ".mp3"},
		{"unknown/type", ".type"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			result := analyzer.mimeTypeToExtension(tt.mimeType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateAttachmentSize(t *testing.T) {
	analyzer := &Analyzer{}

	t.Run("single attachment", func(t *testing.T) {
		payload := &MessagePayload{
			Filename: "test.pdf",
			MimeType: "application/pdf",
			Body: &MessagePartBody{
				Size: 1024000,
			},
		}

		size := analyzer.calculateAttachmentSize(payload)
		assert.Equal(t, int64(1024000), size)
	})

	t.Run("nested attachments", func(t *testing.T) {
		payload := &MessagePayload{
			Parts: []*MessagePart{
				{
					Filename: "doc1.pdf",
					Body: &MessagePartBody{
						Size: 500000,
					},
				},
				{
					Filename: "doc2.pdf",
					Body: &MessagePartBody{
						Size: 750000,
					},
				},
			},
		}

		size := analyzer.calculateAttachmentSize(payload)
		assert.Equal(t, int64(1250000), size)
	})

	t.Run("no attachments", func(t *testing.T) {
		payload := &MessagePayload{
			MimeType: "text/plain",
			Body: &MessagePartBody{
				Size: 100,
			},
		}

		size := analyzer.calculateAttachmentSize(payload)
		assert.Equal(t, int64(0), size)
	})
}

func TestExtractAttachments(t *testing.T) {
	analyzer := &Analyzer{}

	t.Run("extract single attachment", func(t *testing.T) {
		payload := &MessagePayload{
			Filename: "document.pdf",
			MimeType: "application/pdf",
			Body: &MessagePartBody{
				Size: 1024000,
			},
		}

		attachments := analyzer.extractAttachments(payload, "msg-123")
		assert.Len(t, attachments, 1)
		assert.Equal(t, "document.pdf", attachments[0].Filename)
		assert.Equal(t, "application/pdf", attachments[0].MimeType)
		assert.Equal(t, int64(1024000), attachments[0].Size)
		assert.Equal(t, "msg-123", attachments[0].MessageID)
	})

	t.Run("extract multiple attachments from parts", func(t *testing.T) {
		payload := &MessagePayload{
			Parts: []*MessagePart{
				{
					Filename: "file1.jpg",
					MimeType: "image/jpeg",
					Body: &MessagePartBody{
						Size: 200000,
					},
				},
				{
					Filename: "file2.png",
					MimeType: "image/png",
					Body: &MessagePartBody{
						Size: 300000,
					},
				},
			},
		}

		attachments := analyzer.extractAttachments(payload, "msg-456")
		assert.Len(t, attachments, 2)
		assert.Equal(t, "file1.jpg", attachments[0].Filename)
		assert.Equal(t, "file2.png", attachments[1].Filename)
	})

	t.Run("no attachments", func(t *testing.T) {
		payload := &MessagePayload{
			MimeType: "text/plain",
			Body: &MessagePartBody{
				Size: 100,
			},
		}

		attachments := analyzer.extractAttachments(payload, "msg-789")
		assert.Len(t, attachments, 0)
	})
}

func TestNewAnalyzer(t *testing.T) {
	config := createTestClientConfig()
	// We can't create a real client without auth, but we can test the structure
	assert.NotNil(t, config)

	// Create analyzer with nil client for structure test
	analyzer := NewAnalyzer(nil)
	assert.NotNil(t, analyzer)
}

// Integration test helpers would go here
// These would use mock Gmail API responses to test the full analysis flow
