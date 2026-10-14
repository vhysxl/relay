package reaper

import (
	"context"
	"log"
	"time"
)

type reaper struct {
	repo     Repository
	interval time.Duration
}

// New creates a new Reaper instance.
func New(repo Repository, interval time.Duration) Reaper {
	return NewReaper(repo, interval)
}

// NewReaper creates a new Reaper instance with explicit naming.
func NewReaper(repo Repository, interval time.Duration) Reaper {
	if interval <= 0 {
		panic("reaper: interval must be positive and injected via DI / config")
	}
	return &reaper{
		repo:     repo,
		interval: interval,
	}
}

// Start runs the reaper ticker loop until ctx is cancelled.
func (r *reaper) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := r.ReapOnce(ctx)
			if err != nil && ctx.Err() == nil {
				log.Printf("[reaper] error reaping expired step locks: %v", err)
			} else if count > 0 {
				log.Printf("[reaper] successfully reaped %d expired step locks back to PENDING", count)
			}
		}
	}
}

// ReapOnce triggers a single reaping iteration (useful for tests and manual invocation).
func (r *reaper) ReapOnce(ctx context.Context) (int64, error) {
	return r.repo.ReapExpiredSteps(ctx)
}
