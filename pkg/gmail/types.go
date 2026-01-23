package gmail

import (
	"fmt"
	"time"
)

// GmailMessage represents a simplified Gmail message structure
type GmailMessage struct {
	ID           string
	ThreadID     string
	LabelIDs     []string
	Snippet      string
	SizeEstimate int64
	InternalDate int64
	Payload      *MessagePayload
	Headers      map[string]string
}

// MessagePayload represents the message body structure
type MessagePayload struct {
	PartID   string
	MimeType string
	Filename string
	Headers  []MessageHeader
	Body     *MessagePartBody
	Parts    []*MessagePart
}

// MessageHeader represents an email header (Subject, From, To, etc.)
type MessageHeader struct {
	Name  string
	Value string
}

// MessagePart represents a part of a multipart message
type MessagePart struct {
	PartID   string
	MimeType string
	Filename string
	Headers  []MessageHeader
	Body     *MessagePartBody
	Parts    []*MessagePart
}

// MessagePartBody represents the body of a message part
type MessagePartBody struct {
	AttachmentID string
	Size         int
	Data         string
}

// SenderStats represents statistics for a sender
type SenderStats struct {
	Email      string
	Domain     string
	Count      int
	TotalSize  int64
	FirstSeen  time.Time
	LastSeen   time.Time
	HasUnread  bool
	LabelCount map[string]int
}

// EmailSize represents size information for an email
type EmailSize struct {
	MessageID      string
	ThreadID       string
	From           string
	To             string
	Subject        string
	Date           time.Time
	Size           int64
	HasAttachments bool
	AttachmentSize int64
	Labels         []string
}

// AttachmentInfo represents information about an attachment
type AttachmentInfo struct {
	Filename  string
	MimeType  string
	Size      int64
	MessageID string
	Date      time.Time
}

// AttachmentAnalysis represents aggregated attachment statistics
type AttachmentAnalysis struct {
	MimeType   string
	Extension  string
	Count      int
	TotalSize  int64
	AvgSize    int64
	LargestMsg string
	LargestSize int64
	Files      []string
}

// DateDistribution represents email distribution over time
type DateDistribution struct {
	Date  time.Time
	Count int
	Size  int64
}

// RegexMatch represents an email matching a regex pattern
type RegexMatch struct {
	MessageID string
	ThreadID  string
	From      string
	Subject   string
	Date      time.Time
	Snippet   string
	MatchedOn string // "subject", "from", "body"
}

// DeleteFilters represents filters for email deletion
type DeleteFilters struct {
	SenderEmails    []string
	SenderDomains   []string
	DateBefore      *time.Time
	DateAfter       *time.Time
	SizeGreaterThan int64
	SizeLessThan    int64
	SubjectRegex    string
	HasAttachment   *bool
	Labels          []string
	ExcludeLabels   []string
}

// DeletionPreview represents a preview of emails to be deleted
type DeletionPreview struct {
	TotalCount       int
	TotalSize        int64
	Messages         []*GmailMessage
	BySender         map[string]int
	ByLabel          map[string]int
	OldestDate       time.Time
	NewestDate       time.Time
	WithAttachments  int
	WithoutAttachments int
}

// QueryBuilder helps construct Gmail query strings
type QueryBuilder struct {
	parts []string
}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		parts: make([]string, 0),
	}
}

// From adds a from filter
func (qb *QueryBuilder) From(email string) *QueryBuilder {
	qb.parts = append(qb.parts, "from:"+email)
	return qb
}

// To adds a to filter
func (qb *QueryBuilder) To(email string) *QueryBuilder {
	qb.parts = append(qb.parts, "to:"+email)
	return qb
}

// Subject adds a subject filter
func (qb *QueryBuilder) Subject(text string) *QueryBuilder {
	qb.parts = append(qb.parts, "subject:"+text)
	return qb
}

// After adds a date after filter
func (qb *QueryBuilder) After(date string) *QueryBuilder {
	qb.parts = append(qb.parts, "after:"+date)
	return qb
}

// Before adds a date before filter
func (qb *QueryBuilder) Before(date string) *QueryBuilder {
	qb.parts = append(qb.parts, "before:"+date)
	return qb
}

// Larger adds a size larger than filter
func (qb *QueryBuilder) Larger(size int64) *QueryBuilder {
	qb.parts = append(qb.parts, fmt.Sprintf("larger:%d", size))
	return qb
}

// Smaller adds a size smaller than filter
func (qb *QueryBuilder) Smaller(size int64) *QueryBuilder {
	qb.parts = append(qb.parts, fmt.Sprintf("smaller:%d", size))
	return qb
}

// HasAttachment adds attachment filter
func (qb *QueryBuilder) HasAttachment() *QueryBuilder {
	qb.parts = append(qb.parts, "has:attachment")
	return qb
}

// Label adds a label filter
func (qb *QueryBuilder) Label(label string) *QueryBuilder {
	qb.parts = append(qb.parts, "label:"+label)
	return qb
}

// Build constructs the final query string
func (qb *QueryBuilder) Build() string {
	result := ""
	for i, part := range qb.parts {
		if i > 0 {
			result += " "
		}
		result += part
	}
	return result
}
