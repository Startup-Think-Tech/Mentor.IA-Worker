package retry

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeStore struct {
	called bool
}

func (s *fakeStore) ScheduleDueRetryJobs(context.Context, int) (int, error) {
	s.called = true
	return 1, nil
}

func TestSchedulerSchedulesDueJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := &fakeStore{}
	scheduler := NewScheduler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		store,
		time.Minute,
		10,
	)

	if err := scheduler.Run(ctx); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if !store.called {
		t.Fatal("store was not called")
	}
}
