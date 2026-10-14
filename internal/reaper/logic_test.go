package reaper_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/reaper"
)

type mockRepository struct {
	reapCount int64
	reapErr   error
	reapCalls int
}

func (m *mockRepository) ReapExpiredSteps(ctx context.Context) (int64, error) {
	m.reapCalls++
	return m.reapCount, m.reapErr
}

func TestReaper_ReapOnce(t *testing.T) {
	mockRepo := &mockRepository{
		reapCount: 3,
		reapErr:   nil,
	}

	r := reaper.New(mockRepo, time.Second)
	if r == nil {
		t.Fatal("expected New to return non-nil instance")
	}

	count, err := r.ReapOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
	if mockRepo.reapCalls != 1 {
		t.Errorf("expected 1 call, got %d", mockRepo.reapCalls)
	}
}

func TestReaper_InvalidInterval_Panics(t *testing.T) {
	mockRepo := &mockRepository{}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on non-positive interval, got none")
		}
	}()

	_ = reaper.New(mockRepo, 0)
}

func TestReaper_ReapOnce_Error(t *testing.T) {
	expectedErr := errors.New("database query error")
	mockRepo := &mockRepository{
		reapErr: expectedErr,
	}

	r := reaper.NewReaper(mockRepo, time.Second)
	_, err := r.ReapOnce(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestReaper_StartGracefulShutdown(t *testing.T) {
	mockRepo := &mockRepository{}
	r := reaper.New(mockRepo, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// success: reaper exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("reaper did not stop cleanly within timeout")
	}

	if mockRepo.reapCalls == 0 {
		t.Error("expected at least one reap call before shutdown")
	}
}
