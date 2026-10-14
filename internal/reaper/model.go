package reaper

import (
	"context"
)

// Repository defines the data-access operations required by Reaper.
type Repository interface {
	ReapExpiredSteps(ctx context.Context) (int64, error)
}

// Reaper defines the contract for periodic background step lock recovery.
type Reaper interface {
	Start(ctx context.Context)
	ReapOnce(ctx context.Context) (int64, error)
}
