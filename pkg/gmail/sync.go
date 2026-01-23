package gmail

import (
	"context"
	"fmt"

	"cli/main/pkg/gmail/cache"
)

// formatNumber formats a number with thousand separators
func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%s,%03d", formatNumber(n/1000), n%1000)
}

// SyncToCache fetches all messages from Gmail and syncs them to the cache with progress tracking
func (c *GmailClient) SyncToCache(ctx context.Context, cacheDB *cache.Cache, query string, tracker *ProgressTracker, limiter *RateLimiter) error {
	// List all messages (no limit) - this just gets IDs, very fast
	messages, err := c.ListMessages(ctx, query, nil, limiter) // Don't track listing, just fetching
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	if len(messages) == 0 {
		return cacheDB.UpdateSyncTime()
	}

	// Extract all message IDs
	allMessageIDs := make([]string, len(messages))
	for i, msg := range messages {
		allMessageIDs[i] = msg.Id
	}

	// Check which messages already exist in cache
	existingIDs, err := cacheDB.GetExistingMessageIDs(allMessageIDs)
	if err != nil {
		return fmt.Errorf("failed to check cached messages: %w", err)
	}

	// Filter to only messages that need fetching
	var missingMessageIDs []string
	for _, id := range allMessageIDs {
		if !existingIDs[id] {
			missingMessageIDs = append(missingMessageIDs, id)
		}
	}

	// Print cache statistics
	cachedCount := len(existingIDs)
	newCount := len(missingMessageIDs)
	totalCount := len(allMessageIDs)

	fmt.Printf("   Found %s total messages\n", formatNumber(int64(totalCount)))
	fmt.Printf("   %s already cached, fetching %s new messages\n\n",
		formatNumber(int64(cachedCount)), formatNumber(int64(newCount)))

	// If everything is cached, we're done
	if newCount == 0 {
		return cacheDB.UpdateSyncTime()
	}

	// Set the total count for progress tracking (only for NEW messages)
	if tracker != nil {
		tracker.SetTotal(int64(newCount))
	}

	// Batch fetch full message details (with headers) - this is the slow operation
	fullMessages, err := c.BatchGetMessages(ctx, missingMessageIDs, false, tracker, limiter)
	if err != nil {
		return fmt.Errorf("failed to fetch message details: %w", err)
	}

	// Convert to cache.EmailMessage format and cache in batches
	cacheBatchSize := c.config.GetInt("gmail.batchSizes.cacheBatchSize")
	if cacheBatchSize <= 0 {
		cacheBatchSize = 100
	}

	for i := 0; i < len(fullMessages); i += cacheBatchSize {
		end := i + cacheBatchSize
		if end > len(fullMessages) {
			end = len(fullMessages)
		}

		batch := fullMessages[i:end]
		cacheMessages := make([]*cache.EmailMessage, len(batch))
		for j, msg := range batch {
			cacheMessages[j] = &cache.EmailMessage{
				ID:           msg.ID,
				LabelIDs:     msg.LabelIDs,
				Snippet:      msg.Snippet,
				SizeEstimate: msg.SizeEstimate,
				InternalDate: msg.InternalDate,
				Headers:      msg.Headers,
			}
		}

		// Batch upsert to cache
		if err := cacheDB.UpsertBatch(cacheMessages); err != nil {
			return fmt.Errorf("failed to update cache: %w", err)
		}

		// Report cache progress
		if tracker != nil {
			tracker.NotifyCache(int64(len(cacheMessages)))
		}
	}

	// Update sync time
	if err := cacheDB.UpdateSyncTime(); err != nil {
		return fmt.Errorf("failed to update sync time: %w", err)
	}

	return nil
}
