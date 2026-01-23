package gmail

import (
	"sync"
	"time"
)

// ProgressCallback is called when progress events occur
type ProgressCallback func(event *ProgressEvent)

// ProgressEvent contains information about sync progress
type ProgressEvent struct {
	Stage              string        // "fetching" or "caching"
	Current            int64         // Current item count
	Total              int64         // Total items (0 if unknown)
	ItemsPerSec        float64       // Processing rate
	EstimatedRemaining time.Duration // Estimated time remaining
}

// ProgressTracker tracks sync progress with dual callbacks
type ProgressTracker struct {
	fetchCallback ProgressCallback
	cacheCallback ProgressCallback

	fetchedCount int64
	cachedCount  int64
	totalCount   int64

	startTime time.Time
	mu        sync.Mutex
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(fetchCB, cacheCB ProgressCallback) *ProgressTracker {
	return &ProgressTracker{
		fetchCallback: fetchCB,
		cacheCallback: cacheCB,
		startTime:     time.Now(),
	}
}

// SetTotal sets the total count for progress calculations
func (p *ProgressTracker) SetTotal(total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalCount = total
}

// NotifyFetch reports progress for fetching messages from API
func (p *ProgressTracker) NotifyFetch(count int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.fetchedCount += count

	if p.fetchCallback != nil {
		event := p.calculateStats("fetching", p.fetchedCount)
		p.fetchCallback(event)
	}
}

// NotifyCache reports progress for caching messages to database
func (p *ProgressTracker) NotifyCache(count int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cachedCount += count

	if p.cacheCallback != nil {
		event := p.calculateStats("caching", p.cachedCount)
		p.cacheCallback(event)
	}
}

// calculateStats computes progress statistics (must be called with lock held)
func (p *ProgressTracker) calculateStats(stage string, current int64) *ProgressEvent {
	elapsed := time.Since(p.startTime).Seconds()
	var itemsPerSec float64
	if elapsed > 0 {
		itemsPerSec = float64(current) / elapsed
	}

	var estimatedRemaining time.Duration
	if p.totalCount > 0 && current > 0 && itemsPerSec > 0 {
		remaining := p.totalCount - current
		secondsRemaining := float64(remaining) / itemsPerSec
		estimatedRemaining = time.Duration(secondsRemaining * float64(time.Second))
	}

	return &ProgressEvent{
		Stage:              stage,
		Current:            current,
		Total:              p.totalCount,
		ItemsPerSec:        itemsPerSec,
		EstimatedRemaining: estimatedRemaining,
	}
}

// Stats returns current progress statistics
func (p *ProgressTracker) Stats() (fetched, cached, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fetchedCount, p.cachedCount, p.totalCount
}
