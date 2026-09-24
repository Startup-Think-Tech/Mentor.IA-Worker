package outbox

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
)

type fakeStore struct {
	events    []domain.OutboxEvent
	published []string
	failed    []string
}

func (s *fakeStore) ClaimPendingOutboxEvents(context.Context, int, time.Duration) ([]domain.OutboxEvent, error) {
	return s.events, nil
}

func (s *fakeStore) MarkOutboxEventPublished(_ context.Context, event domain.OutboxEvent) error {
	s.published = append(s.published, event.ID)
	return nil
}

func (s *fakeStore) MarkOutboxEventFailed(_ context.Context, event domain.OutboxEvent, _ int, _ error) (bool, error) {
	s.failed = append(s.failed, event.ID)
	return false, nil
}

type fakePublisher struct {
	failJobID string
	published []string
}

func (p *fakePublisher) PublishInsightMessage(_ context.Context, payload any) error {
	message := payload.(domain.Message)
	if message.JobID == p.failJobID {
		return errors.New("publisher unavailable")
	}

	p.published = append(p.published, message.JobID)
	return nil
}

func TestDispatcherContinuesAfterPoisonEvent(t *testing.T) {
	store := &fakeStore{events: []domain.OutboxEvent{
		{ID: "event-1", Type: domain.OutboxTypeInsightRetryRequested, Payload: []byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`), Attempts: 1},
		{ID: "event-2", Type: domain.OutboxTypeInsightRetryRequested, Payload: []byte(`{"job_id":"job-2","aluno_id":"aluno-1"}`), Attempts: 1},
	}}
	publisher := &fakePublisher{failJobID: "job-1"}
	dispatcher := NewDispatcher(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		store,
		publisher,
		time.Second,
		2,
		time.Second,
		3,
	)

	dispatcher.dispatch(context.Background())

	if len(store.failed) != 1 || store.failed[0] != "event-1" {
		t.Fatalf("failed events = %#v, want event-1", store.failed)
	}

	if len(store.published) != 1 || store.published[0] != "event-2" {
		t.Fatalf("published events = %#v, want event-2", store.published)
	}
}
