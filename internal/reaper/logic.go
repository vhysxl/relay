package reaper

import (
	"context"
	"log"
	"runtime/debug"
	"time"
)

type reaper struct {
	repo     Repository
	interval time.Duration
}

// New creates a new Reaper instance.
func New(repo Repository, interval time.Duration) Reaper {
	if interval <= 0 {
		panic("reaper: interval must be positive")
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
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						log.Printf("[reaper] panic recovered in reap loop: %v\n%s", rec, debug.Stack())
					}
				}()
				count, err := r.ReapOnce(ctx)
				if err != nil && ctx.Err() == nil {
					log.Printf("[reaper] error reaping expired step locks: %v", err)
				} else if count > 0 {
					log.Printf("[reaper] successfully reaped %d expired step locks", count)
				}
			}()
		}
	}
}

// ReapOnce triggers a single reaping iteration (useful for tests and manual invocation).
func (r *reaper) ReapOnce(ctx context.Context) (int64, error) {
	return r.repo.ReapExpiredSteps(ctx)
}
