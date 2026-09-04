package services

import (
	"context"
	"log"
	"sync"
	"time"
)

// Scheduler periodically runs the football scrape in the background. Each entity
// is still gated by its own sync_tables TTL (ShouldSync), so a frequent tick only
// refreshes tables that are actually due — keeping live-ish data (standings,
// fixtures, topscorers) fresh without manual triggering.
type Scheduler struct {
	football *FootballScraper
	interval time.Duration
	mu       sync.Mutex // prevents overlapping cron runs
}

// NewScheduler builds a scheduler. intervalStr is a Go duration ("30m", "1h");
// it falls back to 30m if unparseable.
func NewScheduler(football *FootballScraper, intervalStr string) *Scheduler {
	d, err := time.ParseDuration(intervalStr)
	if err != nil || d <= 0 {
		d = 30 * time.Minute
	}
	return &Scheduler{football: football, interval: d}
}

// Start launches the scheduler loop (non-blocking). It runs once shortly after
// startup, then every interval, until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	log.Printf("[Scheduler] auto-scrape enabled every %v", s.interval)
	go func() {
		// Small initial delay so startup/migrations settle first.
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		s.runOnce(ctx)

		t := time.NewTicker(s.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.runOnce(ctx)
			}
		}
	}()
}

func (s *Scheduler) runOnce(ctx context.Context) {
	if !s.mu.TryLock() {
		log.Println("[Scheduler] previous run still in progress, skipping this tick")
		return
	}
	defer s.mu.Unlock()

	log.Println("[Scheduler] auto-scrape football starting (TTL-gated)")
	if _, err := s.football.ScrapeAllFootball(ctx, false, 0, 0); err != nil {
		log.Printf("[Scheduler] auto-scrape error: %v", err)
		return
	}
	log.Println("[Scheduler] auto-scrape football done")
}
