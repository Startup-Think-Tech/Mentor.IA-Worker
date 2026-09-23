package insights

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeRetryStore struct {
	messages []Message
	called   bool
}

func (s *fakeRetryStore) ClaimDueRetryJobs(context.Context, int) ([]Message, error) {
	s.called = true
	return s.messages, nil
}

type fakeRetryPublisher struct {
	messages []Message
}

func (p *fakeRetryPublisher) PublishInsightMessage(_ context.Context, payload any) error {
	message, ok := payload.(Message)
	if !ok {
		return nil
	}

	p.messages = append(p.messages, message)
	return nil
}

func TestRetrySchedulerPublishesDueJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := &fakeRetryStore{messages: []Message{{JobID: "job-1", AlunoID: "aluno-1"}}}
	publisher := &fakeRetryPublisher{}
	scheduler := NewRetryScheduler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		store,
		publisher,
		time.Minute,
		10,
	)

	scheduler.Run(ctx)

	if !store.called {
		t.Fatal("store was not called")
	}

	if len(publisher.messages) != 1 {
		t.Fatalf("published messages = %d, want 1", len(publisher.messages))
	}
}
