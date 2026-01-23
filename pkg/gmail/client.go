package gmail

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/viper"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailClient wraps the Gmail API service
type GmailClient struct {
	service *gmail.Service
	config  *viper.Viper
	userID  string
}

// NewGmailClient creates a new Gmail API client
func NewGmailClient(ctx context.Context, httpClient *http.Client, config *viper.Viper) (*GmailClient, error) {
	service, err := gmail.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("unable to create Gmail service: %w", err)
	}

	return &GmailClient{
		service: service,
		config:  config,
		userID:  "me", // "me" is a special value that refers to the authenticated user
	}, nil
}

// ListMessages lists all messages matching the query with progress tracking and rate limiting
func (gc *GmailClient) ListMessages(ctx context.Context, query string, tracker *ProgressTracker, limiter *RateLimiter) ([]*gmail.Message, error) {
	var allMessages []*gmail.Message
	pageToken := ""
	listBatchSize := int64(gc.config.GetInt("gmail.batchSizes.listBatchSize"))
	if listBatchSize <= 0 {
		listBatchSize = 500
	}

	for {
		// Apply rate limiting before API call
		if limiter != nil {
			if err := limiter.Allow(ctx); err != nil {
				return nil, fmt.Errorf("rate limiter error: %w", err)
			}
		}

		call := gc.service.Users.Messages.List(gc.userID).Q(query).MaxResults(listBatchSize)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		response, err := call.Context(ctx).Do()
		if err != nil {
			// Handle quota errors with exponential backoff
			if limiter != nil {
				for attempt := 0; attempt < limiter.config.MaxRetries; attempt++ {
					shouldRetry, retryErr := limiter.HandleQuotaError(ctx, err, attempt)
					if !shouldRetry {
						if retryErr != nil {
							return nil, retryErr
						}
						return nil, fmt.Errorf("unable to list messages: %w", err)
					}
					// Retry the call
					response, err = call.Context(ctx).Do()
					if err == nil {
						break
					}
				}
				if err != nil {
					return nil, fmt.Errorf("unable to list messages after retries: %w", err)
				}
			} else {
				return nil, fmt.Errorf("unable to list messages: %w", err)
			}
		}

		allMessages = append(allMessages, response.Messages...)

		// No more pages
		if response.NextPageToken == "" {
			break
		}

		pageToken = response.NextPageToken
	}

	return allMessages, nil
}

// GetMessage fetches a single message with minimal format
func (gc *GmailClient) GetMessage(ctx context.Context, messageID string) (*gmail.Message, error) {
	msg, err := gc.service.Users.Messages.Get(gc.userID, messageID).
		Format("metadata").
		MetadataHeaders("From", "To", "Subject", "Date").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get message %s: %w", messageID, err)
	}

	return msg, nil
}

// GetMessageFull fetches a single message with full format including body
func (gc *GmailClient) GetMessageFull(ctx context.Context, messageID string) (*GmailMessage, error) {
	msg, err := gc.service.Users.Messages.Get(gc.userID, messageID).
		Format("full").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get full message %s: %w", messageID, err)
	}

	return gc.convertToGmailMessage(msg), nil
}

// BatchGetMessages fetches multiple messages efficiently with progress tracking and rate limiting
func (gc *GmailClient) BatchGetMessages(ctx context.Context, messageIDs []string, full bool, tracker *ProgressTracker, limiter *RateLimiter) ([]*GmailMessage, error) {
	messages := make([]*GmailMessage, 0, len(messageIDs))

	// Get batch size from config
	batchSize := gc.config.GetInt("gmail.batchSizes.fetchBatchSize")
	if batchSize <= 0 {
		batchSize = 50
	}

	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}

		batch := messageIDs[i:end]
		for _, msgID := range batch {
			// Apply rate limiting before each API call
			if limiter != nil {
				if err := limiter.Allow(ctx); err != nil {
					return nil, fmt.Errorf("rate limiter error: %w", err)
				}
			}

			var msg *gmail.Message
			var err error

			if full {
				msg, err = gc.service.Users.Messages.Get(gc.userID, msgID).
					Format("full").
					Context(ctx).
					Do()
			} else {
				msg, err = gc.service.Users.Messages.Get(gc.userID, msgID).
					Format("metadata").
					MetadataHeaders("From", "To", "Subject", "Date").
					Context(ctx).
					Do()
			}

			if err != nil {
				// Handle quota errors with exponential backoff
				if limiter != nil {
					for attempt := 0; attempt < limiter.config.MaxRetries; attempt++ {
						shouldRetry, retryErr := limiter.HandleQuotaError(ctx, err, attempt)
						if !shouldRetry {
							if retryErr != nil {
								// Log error but continue with other messages
								fmt.Printf("⚠️  Failed to fetch message %s after retries: %s\n", msgID, retryErr)
							} else {
								fmt.Printf("⚠️  Failed to fetch message %s: %s\n", msgID, err)
							}
							break
						}
						// Retry the call
						if full {
							msg, err = gc.service.Users.Messages.Get(gc.userID, msgID).
								Format("full").
								Context(ctx).
								Do()
						} else {
							msg, err = gc.service.Users.Messages.Get(gc.userID, msgID).
								Format("metadata").
								MetadataHeaders("From", "To", "Subject", "Date").
								Context(ctx).
								Do()
						}
						if err == nil {
							break
						}
					}
				}

				if err != nil {
					// Log error but continue with other messages
					fmt.Printf("⚠️  Failed to fetch message %s: %s\n", msgID, err)
					continue
				}
			}

			messages = append(messages, gc.convertToGmailMessage(msg))

			// Report progress per message
			if tracker != nil {
				tracker.NotifyFetch(1)
			}
		}
	}

	return messages, nil
}

// BatchDelete deletes multiple messages
func (gc *GmailClient) BatchDelete(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	batchSize := gc.config.GetInt("gmail.deletionBatchSize")
	if batchSize <= 0 {
		batchSize = 100
	}

	// Process in batches
	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}

		batch := messageIDs[i:end]

		// Use batch delete for efficiency
		req := &gmail.BatchDeleteMessagesRequest{
			Ids: batch,
		}

		err := gc.service.Users.Messages.BatchDelete(gc.userID, req).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("batch delete failed at messages %d-%d: %w", i, end, err)
		}

		fmt.Printf("🗑️  Deleted %d messages\n", len(batch))

		// Rate limiting between batches
		if end < len(messageIDs) {
			time.Sleep(200 * time.Millisecond)
		}
	}

	return nil
}

// ModifyLabels modifies labels on messages (for safety tagging before deletion)
func (gc *GmailClient) ModifyLabels(ctx context.Context, messageIDs []string, addLabels, removeLabels []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	// Process in batches
	batchSize := 100
	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}

		batch := messageIDs[i:end]

		req := &gmail.BatchModifyMessagesRequest{
			Ids:            batch,
			AddLabelIds:    addLabels,
			RemoveLabelIds: removeLabels,
		}

		err := gc.service.Users.Messages.BatchModify(gc.userID, req).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("batch modify labels failed: %w", err)
		}

		// Rate limiting
		if end < len(messageIDs) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

// convertToGmailMessage converts Gmail API message to our simplified structure
func (gc *GmailClient) convertToGmailMessage(msg *gmail.Message) *GmailMessage {
	headers := make(map[string]string)
	if msg.Payload != nil {
		for _, header := range msg.Payload.Headers {
			headers[header.Name] = header.Value
		}
	}

	gmailMsg := &GmailMessage{
		ID:           msg.Id,
		ThreadID:     msg.ThreadId,
		LabelIDs:     msg.LabelIds,
		Snippet:      msg.Snippet,
		SizeEstimate: msg.SizeEstimate,
		InternalDate: msg.InternalDate,
		Headers:      headers,
	}

	if msg.Payload != nil {
		gmailMsg.Payload = gc.convertPayload(msg.Payload)
	}

	return gmailMsg
}

// convertPayload converts Gmail API payload to our structure
func (gc *GmailClient) convertPayload(payload *gmail.MessagePart) *MessagePayload {
	result := &MessagePayload{
		PartID:   payload.PartId,
		MimeType: payload.MimeType,
		Filename: payload.Filename,
	}

	// Convert headers
	for _, h := range payload.Headers {
		result.Headers = append(result.Headers, MessageHeader{
			Name:  h.Name,
			Value: h.Value,
		})
	}

	// Convert body
	if payload.Body != nil {
		result.Body = &MessagePartBody{
			AttachmentID: payload.Body.AttachmentId,
			Size:         int(payload.Body.Size),
			Data:         payload.Body.Data,
		}
	}

	// Convert parts recursively
	for _, part := range payload.Parts {
		result.Parts = append(result.Parts, gc.convertMessagePart(part))
	}

	return result
}

// convertMessagePart converts Gmail API message part to our structure
func (gc *GmailClient) convertMessagePart(part *gmail.MessagePart) *MessagePart {
	result := &MessagePart{
		PartID:   part.PartId,
		MimeType: part.MimeType,
		Filename: part.Filename,
	}

	// Convert headers
	for _, h := range part.Headers {
		result.Headers = append(result.Headers, MessageHeader{
			Name:  h.Name,
			Value: h.Value,
		})
	}

	// Convert body
	if part.Body != nil {
		result.Body = &MessagePartBody{
			AttachmentID: part.Body.AttachmentId,
			Size:         int(part.Body.Size),
			Data:         part.Body.Data,
		}
	}

	// Convert nested parts recursively
	for _, p := range part.Parts {
		result.Parts = append(result.Parts, gc.convertMessagePart(p))
	}

	return result
}

// GetHeader retrieves a specific header value from a message
func (msg *GmailMessage) GetHeader(name string) string {
	return msg.Headers[name]
}

// GetFrom returns the From header
func (msg *GmailMessage) GetFrom() string {
	return msg.GetHeader("From")
}

// GetSubject returns the Subject header
func (msg *GmailMessage) GetSubject() string {
	return msg.GetHeader("Subject")
}

// GetDate returns the Date header as a time.Time
func (msg *GmailMessage) GetDate() (time.Time, error) {
	dateStr := msg.GetHeader("Date")
	if dateStr == "" {
		// Fallback to internal date
		return time.Unix(msg.InternalDate/1000, 0), nil
	}

	// Try parsing common email date formats
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

// ExtractEmailAddress extracts email address from a From/To header (e.g., "Name <email@example.com>" -> "email@example.com")
func ExtractEmailAddress(header string) string {
	// Find email between < >
	start := strings.Index(header, "<")
	end := strings.Index(header, ">")

	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(header[start+1 : end])
	}

	// No brackets, assume the whole thing is an email
	return strings.TrimSpace(header)
}

// ExtractDomain extracts domain from an email address
func ExtractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return email
}
