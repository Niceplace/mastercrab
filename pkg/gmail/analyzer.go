package gmail

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Analyzer provides email analysis functionality
type Analyzer struct {
	client *GmailClient
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(client *GmailClient) *Analyzer {
	return &Analyzer{
		client: client,
	}
}

// AnalyzeSenderStats analyzes emails by sender and domain
func (a *Analyzer) AnalyzeSenderStats(ctx context.Context, query string, limit int64) ([]SenderStats, error) {
	// List messages (no progress tracking for analysis operations)
	messages, err := a.client.ListMessages(ctx, query, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Apply limit client-side for analysis operations
	if limit > 0 && int64(len(messages)) > limit {
		messages = messages[:limit]
	}

	if len(messages) == 0 {
		return []SenderStats{}, nil
	}

	// Extract message IDs
	messageIDs := make([]string, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.Id
	}

	// Batch fetch message details
	fullMessages, err := a.client.BatchGetMessages(ctx, messageIDs, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message details: %w", err)
	}

	// Aggregate by sender
	senderMap := make(map[string]*SenderStats)

	for _, msg := range fullMessages {
		from := msg.GetFrom()
		email := ExtractEmailAddress(from)
		domain := ExtractDomain(email)

		if _, exists := senderMap[email]; !exists {
			senderMap[email] = &SenderStats{
				Email:      email,
				Domain:     domain,
				Count:      0,
				TotalSize:  0,
				LabelCount: make(map[string]int),
			}
		}

		stat := senderMap[email]
		stat.Count++
		stat.TotalSize += msg.SizeEstimate

		// Track date range
		if date, err := msg.GetDate(); err == nil {
			if stat.FirstSeen.IsZero() || date.Before(stat.FirstSeen) {
				stat.FirstSeen = date
			}
			if stat.LastSeen.IsZero() || date.After(stat.LastSeen) {
				stat.LastSeen = date
			}
		}

		// Check for unread
		for _, label := range msg.LabelIDs {
			if label == "UNREAD" {
				stat.HasUnread = true
			}
			stat.LabelCount[label]++
		}
	}

	// Convert to slice and sort by count
	results := make([]SenderStats, 0, len(senderMap))
	for _, stat := range senderMap {
		results = append(results, *stat)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Count > results[j].Count
	})

	return results, nil
}

// AnalyzeSizeDistribution analyzes emails by size
func (a *Analyzer) AnalyzeSizeDistribution(ctx context.Context, query string, limit int64) ([]EmailSize, error) {
	// List messages
	messages, err := a.client.ListMessages(ctx, query, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Apply limit client-side
	if limit > 0 && int64(len(messages)) > limit {
		messages = messages[:limit]
	}

	if len(messages) == 0 {
		return []EmailSize{}, nil
	}

	// Extract message IDs
	messageIDs := make([]string, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.Id
	}

	// Batch fetch message details
	fullMessages, err := a.client.BatchGetMessages(ctx, messageIDs, true, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message details: %w", err)
	}

	// Build size list
	results := make([]EmailSize, 0, len(fullMessages))

	for _, msg := range fullMessages {
		from := ExtractEmailAddress(msg.GetFrom())
		subject := msg.GetSubject()
		date, _ := msg.GetDate()

		hasAttachments := false
		attachmentSize := int64(0)

		// Check for attachments
		if msg.Payload != nil {
			attachmentSize = a.calculateAttachmentSize(msg.Payload)
			hasAttachments = attachmentSize > 0
		}

		results = append(results, EmailSize{
			MessageID:      msg.ID,
			ThreadID:       msg.ThreadID,
			From:           from,
			Subject:        subject,
			Date:           date,
			Size:           msg.SizeEstimate,
			HasAttachments: hasAttachments,
			AttachmentSize: attachmentSize,
			Labels:         msg.LabelIDs,
		})
	}

	// Sort by size descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Size > results[j].Size
	})

	return results, nil
}

// AnalyzeDateDistribution analyzes email distribution over time
func (a *Analyzer) AnalyzeDateDistribution(ctx context.Context, query string, limit int64) ([]DateDistribution, error) {
	// List messages
	messages, err := a.client.ListMessages(ctx, query, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Apply limit client-side
	if limit > 0 && int64(len(messages)) > limit {
		messages = messages[:limit]
	}

	if len(messages) == 0 {
		return []DateDistribution{}, nil
	}

	// Extract message IDs
	messageIDs := make([]string, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.Id
	}

	// Batch fetch message details
	fullMessages, err := a.client.BatchGetMessages(ctx, messageIDs, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message details: %w", err)
	}

	// Aggregate by date (day granularity)
	dateMap := make(map[string]*DateDistribution)

	for _, msg := range fullMessages {
		date, err := msg.GetDate()
		if err != nil {
			// Use internal date as fallback
			date = time.Unix(msg.InternalDate/1000, 0)
		}

		// Truncate to day
		dayKey := date.Format("2006-01-02")

		if _, exists := dateMap[dayKey]; !exists {
			dateMap[dayKey] = &DateDistribution{
				Date:  date.Truncate(24 * time.Hour),
				Count: 0,
				Size:  0,
			}
		}

		dateMap[dayKey].Count++
		dateMap[dayKey].Size += msg.SizeEstimate
	}

	// Convert to slice and sort by date
	results := make([]DateDistribution, 0, len(dateMap))
	for _, dist := range dateMap {
		results = append(results, *dist)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Date.Before(results[j].Date)
	})

	return results, nil
}

// AnalyzeAttachments analyzes attachments by type and size
func (a *Analyzer) AnalyzeAttachments(ctx context.Context, query string, limit int64) ([]AttachmentAnalysis, error) {
	// Use query with attachment filter
	attachmentQuery := query
	if !strings.Contains(query, "has:attachment") {
		if query != "" {
			attachmentQuery = query + " has:attachment"
		} else {
			attachmentQuery = "has:attachment"
		}
	}

	// List messages
	messages, err := a.client.ListMessages(ctx, attachmentQuery, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Apply limit client-side
	if limit > 0 && int64(len(messages)) > limit {
		messages = messages[:limit]
	}

	if len(messages) == 0 {
		return []AttachmentAnalysis{}, nil
	}

	// Extract message IDs
	messageIDs := make([]string, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.Id
	}

	// Batch fetch message details (need full format for parts)
	fullMessages, err := a.client.BatchGetMessages(ctx, messageIDs, true, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message details: %w", err)
	}

	// Aggregate by MIME type
	typeMap := make(map[string]*AttachmentAnalysis)

	for _, msg := range fullMessages {
		if msg.Payload == nil {
			continue
		}

		attachments := a.extractAttachments(msg.Payload, msg.ID)
		for _, att := range attachments {
			if _, exists := typeMap[att.MimeType]; !exists {
				typeMap[att.MimeType] = &AttachmentAnalysis{
					MimeType:  att.MimeType,
					Extension: a.mimeTypeToExtension(att.MimeType),
					Count:     0,
					TotalSize: 0,
					Files:     []string{},
				}
			}

			analysis := typeMap[att.MimeType]
			analysis.Count++
			analysis.TotalSize += att.Size

			if att.Size > analysis.LargestSize {
				analysis.LargestSize = att.Size
				analysis.LargestMsg = att.MessageID
			}

			if att.Filename != "" && len(analysis.Files) < 10 {
				analysis.Files = append(analysis.Files, att.Filename)
			}
		}
	}

	// Calculate averages and convert to slice
	results := make([]AttachmentAnalysis, 0, len(typeMap))
	for _, analysis := range typeMap {
		analysis.AvgSize = analysis.TotalSize / int64(analysis.Count)
		results = append(results, *analysis)
	}

	// Sort by total size descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalSize > results[j].TotalSize
	})

	return results, nil
}

// AnalyzeRegexPatterns finds emails matching regex patterns in subject or body
func (a *Analyzer) AnalyzeRegexPatterns(ctx context.Context, pattern string, searchIn string, query string, limit int64) ([]RegexMatch, error) {
	// Compile regex
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	// List messages
	messages, err := a.client.ListMessages(ctx, query, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Apply limit client-side
	if limit > 0 && int64(len(messages)) > limit {
		messages = messages[:limit]
	}

	if len(messages) == 0 {
		return []RegexMatch{}, nil
	}

	// Extract message IDs
	messageIDs := make([]string, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.Id
	}

	// Batch fetch message details
	fullMessages, err := a.client.BatchGetMessages(ctx, messageIDs, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message details: %w", err)
	}

	// Find matches
	results := []RegexMatch{}

	for _, msg := range fullMessages {
		from := msg.GetFrom()
		subject := msg.GetSubject()
		date, _ := msg.GetDate()

		matched := false
		matchedOn := ""

		// Check subject
		if searchIn == "subject" || searchIn == "all" {
			if regex.MatchString(subject) {
				matched = true
				matchedOn = "subject"
			}
		}

		// Check from
		if !matched && (searchIn == "from" || searchIn == "all") {
			if regex.MatchString(from) {
				matched = true
				matchedOn = "from"
			}
		}

		// Check snippet (body preview)
		if !matched && (searchIn == "body" || searchIn == "all") {
			if regex.MatchString(msg.Snippet) {
				matched = true
				matchedOn = "body"
			}
		}

		if matched {
			results = append(results, RegexMatch{
				MessageID: msg.ID,
				ThreadID:  msg.ThreadID,
				From:      from,
				Subject:   subject,
				Date:      date,
				Snippet:   msg.Snippet,
				MatchedOn: matchedOn,
			})
		}
	}

	return results, nil
}

// Helper: calculateAttachmentSize calculates total attachment size in a payload
func (a *Analyzer) calculateAttachmentSize(payload *MessagePayload) int64 {
	total := int64(0)

	// Check if this part is an attachment
	if payload.Filename != "" && payload.Body != nil {
		total += int64(payload.Body.Size)
	}

	// Recursively check parts
	for _, part := range payload.Parts {
		total += a.calculateAttachmentSizeFromPart(part)
	}

	return total
}

// Helper: calculateAttachmentSizeFromPart calculates attachment size from a part
func (a *Analyzer) calculateAttachmentSizeFromPart(part *MessagePart) int64 {
	total := int64(0)

	// Check if this part is an attachment
	if part.Filename != "" && part.Body != nil {
		total += int64(part.Body.Size)
	}

	// Recursively check nested parts
	for _, p := range part.Parts {
		total += a.calculateAttachmentSizeFromPart(p)
	}

	return total
}

// Helper: extractAttachments extracts all attachments from a payload
func (a *Analyzer) extractAttachments(payload *MessagePayload, messageID string) []AttachmentInfo {
	attachments := []AttachmentInfo{}

	// Check if this payload part is an attachment
	if payload.Filename != "" && payload.Body != nil && payload.Body.Size > 0 {
		attachments = append(attachments, AttachmentInfo{
			Filename:  payload.Filename,
			MimeType:  payload.MimeType,
			Size:      int64(payload.Body.Size),
			MessageID: messageID,
		})
	}

	// Recursively extract from parts
	for _, part := range payload.Parts {
		attachments = append(attachments, a.extractAttachmentsFromPart(part, messageID)...)
	}

	return attachments
}

// Helper: extractAttachmentsFromPart extracts attachments from a message part
func (a *Analyzer) extractAttachmentsFromPart(part *MessagePart, messageID string) []AttachmentInfo {
	attachments := []AttachmentInfo{}

	// Check if this part is an attachment
	if part.Filename != "" && part.Body != nil && part.Body.Size > 0 {
		attachments = append(attachments, AttachmentInfo{
			Filename:  part.Filename,
			MimeType:  part.MimeType,
			Size:      int64(part.Body.Size),
			MessageID: messageID,
		})
	}

	// Recursively check nested parts
	for _, p := range part.Parts {
		attachments = append(attachments, a.extractAttachmentsFromPart(p, messageID)...)
	}

	return attachments
}

// Helper: mimeTypeToExtension converts MIME type to file extension
func (a *Analyzer) mimeTypeToExtension(mimeType string) string {
	extensions := map[string]string{
		"application/pdf":                                                   ".pdf",
		"application/zip":                                                   ".zip",
		"application/x-zip-compressed":                                      ".zip",
		"image/jpeg":                                                        ".jpg",
		"image/png":                                                         ".png",
		"image/gif":                                                         ".gif",
		"application/msword":                                                ".doc",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
		"application/vnd.ms-excel":                                          ".xls",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
		"application/vnd.ms-powerpoint":                                     ".ppt",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
		"text/plain":  ".txt",
		"text/csv":    ".csv",
		"video/mp4":   ".mp4",
		"video/mpeg":  ".mpeg",
		"audio/mpeg":  ".mp3",
		"audio/wav":   ".wav",
		"text/html":   ".html",
		"application/json": ".json",
	}

	if ext, ok := extensions[mimeType]; ok {
		return ext
	}

	// Default to mime type suffix
	parts := strings.Split(mimeType, "/")
	if len(parts) == 2 {
		return "." + parts[1]
	}

	return ""
}
