package gmail

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"cli/main/pkg/gmail/cache"

	gmailapi "google.golang.org/api/gmail/v1"
)

// Deleter handles email deletion with safety features
type Deleter struct {
	client *GmailClient
	cache  *cache.Cache
}

// CachedDeletionPreview provides information about cached emails to be deleted
type CachedDeletionPreview struct {
	EmailCount  int                  `json:"email_count"`
	TotalSize   int64                `json:"total_size"`
	Emails      []*cache.CachedEmail `json:"emails"`
	SafetyLabel string               `json:"safety_label"`
	GeneratedAt time.Time            `json:"generated_at"`
}

// NewDeleter creates a new Deleter instance
func NewDeleter(client *GmailClient, cacheInstance *cache.Cache) *Deleter {
	return &Deleter{
		client: client,
		cache:  cacheInstance,
	}
}

// BuildPreview generates a deletion preview from cached data
func (d *Deleter) BuildPreview(filters *DeleteFilters) (*CachedDeletionPreview, error) {
	// Get all cached emails
	emails, err := d.cache.GetCachedEmails()
	if err != nil {
		return nil, fmt.Errorf("failed to get cached emails: %w", err)
	}

	// Apply filters
	filtered := d.applyFilters(emails, filters)

	// Calculate totals
	var totalSize int64
	for _, email := range filtered {
		totalSize += email.Size
	}

	// Generate safety label
	safetyLabel := fmt.Sprintf("crab-deleted-%d", time.Now().Unix())

	preview := &CachedDeletionPreview{
		EmailCount:  len(filtered),
		TotalSize:   totalSize,
		Emails:      filtered,
		SafetyLabel: safetyLabel,
		GeneratedAt: time.Now(),
	}

	return preview, nil
}

// applyFilters filters emails based on DeleteFilters criteria
func (d *Deleter) applyFilters(emails []*cache.CachedEmail, filters *DeleteFilters) []*cache.CachedEmail {
	if filters == nil {
		return emails
	}

	var filtered []*cache.CachedEmail

	for _, email := range emails {
		if d.matchesFilters(email, filters) {
			filtered = append(filtered, email)
		}
	}

	return filtered
}

// matchesFilters checks if an email matches all filter criteria
func (d *Deleter) matchesFilters(email *cache.CachedEmail, filters *DeleteFilters) bool {
	// Filter by sender emails
	if len(filters.SenderEmails) > 0 {
		match := false
		for _, sender := range filters.SenderEmails {
			if email.FromEmail == sender {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Filter by sender domains
	if len(filters.SenderDomains) > 0 {
		match := false
		for _, domain := range filters.SenderDomains {
			if containsDomain(email.FromEmail, domain) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Filter by date range
	if filters.DateBefore != nil && email.Date.After(*filters.DateBefore) {
		return false
	}
	if filters.DateAfter != nil && email.Date.Before(*filters.DateAfter) {
		return false
	}

	// Filter by size
	if filters.SizeGreaterThan > 0 && email.Size <= filters.SizeGreaterThan {
		return false
	}
	if filters.SizeLessThan > 0 && email.Size >= filters.SizeLessThan {
		return false
	}

	// Filter by labels (include)
	if len(filters.Labels) > 0 {
		match := false
		for _, filterLabel := range filters.Labels {
			if containsLabel(email.Labels, filterLabel) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Filter by labels (exclude)
	if len(filters.ExcludeLabels) > 0 {
		for _, excludeLabel := range filters.ExcludeLabels {
			if containsLabel(email.Labels, excludeLabel) {
				return false
			}
		}
	}

	return true
}

// ApplySafetyLabel tags emails with a safety label before deletion
func (d *Deleter) ApplySafetyLabel(messageIDs []string, safetyLabel string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	// Create label if it doesn't exist and get its ID
	labelID, err := d.ensureLabelExists(safetyLabel)
	if err != nil {
		return fmt.Errorf("failed to create safety label: %w", err)
	}

	// Batch modify to add label
	req := &gmailapi.ModifyMessageRequest{
		AddLabelIds: []string{labelID},
	}

	// Apply label to all messages
	for _, msgID := range messageIDs {
		if _, err := d.client.service.Users.Messages.Modify("me", msgID, req).Do(); err != nil {
			return fmt.Errorf("failed to apply safety label to message %s: %w", msgID, err)
		}
	}

	return nil
}

// ensureLabelExists creates a label if it doesn't exist and returns its ID
func (d *Deleter) ensureLabelExists(labelName string) (string, error) {
	// List all labels to check if it exists
	resp, err := d.client.service.Users.Labels.List("me").Do()
	if err != nil {
		return "", fmt.Errorf("failed to list labels: %w", err)
	}

	// Check if label already exists
	for _, label := range resp.Labels {
		if label.Name == labelName {
			return label.Id, nil // Label exists
		}
	}

	// Create new label
	label := &gmailapi.Label{
		Name:                  labelName,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}

	created, err := d.client.service.Users.Labels.Create("me", label).Do()
	if err != nil {
		return "", fmt.Errorf("failed to create label: %w", err)
	}

	return created.Id, nil
}

// ExecuteDeletion performs batch deletion with progress updates
func (d *Deleter) ExecuteDeletion(messageIDs []string, batchSize int, progressCh chan<- int) error {
	if len(messageIDs) == 0 {
		return nil
	}

	defer func() {
		if progressCh != nil {
			close(progressCh)
		}
	}()

	// Process in batches
	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}

		batch := messageIDs[i:end]

		// Delete each message in the batch
		for _, msgID := range batch {
			if err := d.client.service.Users.Messages.Delete("me", msgID).Do(); err != nil {
				return fmt.Errorf("failed to delete message %s: %w", msgID, err)
			}

			// Update cache
			if err := d.cache.DeleteEmail(msgID); err != nil {
				// Log but don't fail
				fmt.Printf("⚠️  Failed to delete from cache: %s\n", err)
			}

			// Send progress update
			if progressCh != nil {
				progressCh <- 1
			}
		}
	}

	return nil
}

// ExportToJSON exports the deletion preview to a JSON file
func (d *Deleter) ExportToJSON(preview *CachedDeletionPreview, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(preview); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// ExportToCSV exports the deletion preview to a CSV file
func (d *Deleter) ExportToCSV(preview *CachedDeletionPreview, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"ID", "From Email", "From Name", "To Email", "Subject", "Date", "Size (bytes)", "Labels"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write email data
	for _, email := range preview.Emails {
		labelsJSON, _ := json.Marshal(email.Labels)
		row := []string{
			email.ID,
			email.FromEmail,
			email.FromName,
			email.ToEmail,
			email.Subject,
			email.Date.Format(time.RFC3339),
			fmt.Sprintf("%d", email.Size),
			string(labelsJSON),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// Helper functions

// containsDomain checks if an email address contains a domain
func containsDomain(email, domain string) bool {
	for i := 0; i < len(email); i++ {
		if email[i] == '@' && i+1 < len(email) {
			return email[i+1:] == domain
		}
	}
	return false
}

// containsLabel checks if a label exists in a slice of labels
func containsLabel(labels []string, target string) bool {
	for _, label := range labels {
		if label == target {
			return true
		}
	}
	return false
}
