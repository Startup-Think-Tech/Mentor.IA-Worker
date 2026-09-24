package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
)

type fakeStore struct {
	beginCalled        bool
	findCalled         bool
	saveCalled         bool
	failureCalled      bool
	alreadyHandled     bool
	beginErr           error
	findErr            error
	saveErr            error
	failureAction      domain.FailureAction
	failureMaxAttempts int
	lease              domain.ProcessingLease
	disciplines        []domain.DisciplinePerformance
	savedContent       string
	savedDisciplines   []domain.DisciplinePerformance
}

func (s *fakeStore) BeginProcessing(_ context.Context, message domain.Message) (domain.ProcessingLease, bool, error) {
	s.beginCalled = true
	if s.lease.Token == "" {
		s.lease = domain.ProcessingLease{Message: message, Token: "token-1"}
	}

	return s.lease, s.alreadyHandled, s.beginErr
}

func (s *fakeStore) FindLowestPerformanceDisciplines(context.Context, string, int) ([]domain.DisciplinePerformance, error) {
	s.findCalled = true
	return s.disciplines, s.findErr
}

func (s *fakeStore) SaveInsightResult(_ context.Context, _ domain.ProcessingLease, content string, disciplines []domain.DisciplinePerformance) error {
	s.saveCalled = true
	s.savedContent = content
	s.savedDisciplines = disciplines
	return s.saveErr
}

func (s *fakeStore) RegisterFailure(_ context.Context, _ domain.ProcessingLease, maxAttempts int, _ string, _ error) (domain.FailureAction, error) {
	s.failureCalled = true
	s.failureMaxAttempts = maxAttempts
	if s.failureAction == "" {
		return domain.FailureActionRetry, nil
	}

	return s.failureAction, nil
}

type fakeAIClient struct {
	content string
	err     error
	called  bool
}

type permanentError struct{}

func (permanentError) Error() string   { return "unauthorized" }
func (permanentError) Retryable() bool { return false }

func (c *fakeAIClient) Complete(context.Context, string) (string, error) {
	c.called = true
	return c.content, c.err
}

func TestServiceProcessGeneratesAndSavesInsight(t *testing.T) {
	store := &fakeStore{disciplines: []domain.DisciplinePerformance{{ID: "disciplina-1", Nome: "Matematica", Percentual: 42}}}
	aiClient := &fakeAIClient{content: "Insight real"}
	service := New(store, aiClient, 3)

	if err := service.Process(context.Background(), domain.Message{JobID: "job-1", AlunoID: "aluno-1"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if !store.beginCalled || !store.findCalled || !store.saveCalled {
		t.Fatal("expected store methods to be called")
	}

	if !aiClient.called {
		t.Fatal("expected AI client to be called")
	}

	if store.savedContent != "Insight real" {
		t.Fatalf("savedContent = %q, want Insight real", store.savedContent)
	}
}

func TestServiceProcessSkipsAlreadyHandledJob(t *testing.T) {
	store := &fakeStore{alreadyHandled: true}
	aiClient := &fakeAIClient{content: "Insight real"}
	service := New(store, aiClient, 3)

	if err := service.Process(context.Background(), domain.Message{JobID: "job-1", AlunoID: "aluno-1"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if aiClient.called {
		t.Fatal("AI client should not be called for already handled job")
	}
}

func TestServiceProcessRegistersRetryOnAIError(t *testing.T) {
	store := &fakeStore{failureAction: domain.FailureActionRetry}
	aiClient := &fakeAIClient{err: errors.New("openrouter unavailable")}
	service := New(store, aiClient, 3)

	err := service.Process(context.Background(), domain.Message{JobID: "job-1", AlunoID: "aluno-1"})
	if !errors.Is(err, ErrRetryScheduled) {
		t.Fatalf("Process() error = %v, want ErrRetryScheduled", err)
	}

	if !store.failureCalled {
		t.Fatal("expected failure to be registered")
	}
}

func TestServiceProcessRegistersFinalFailure(t *testing.T) {
	store := &fakeStore{failureAction: domain.FailureActionFailed}
	aiClient := &fakeAIClient{err: errors.New("openrouter failed")}
	service := New(store, aiClient, 3)

	err := service.Process(context.Background(), domain.Message{JobID: "job-1", AlunoID: "aluno-1"})
	if !errors.Is(err, ErrJobFailed) {
		t.Fatalf("Process() error = %v, want ErrJobFailed", err)
	}

}

func TestServiceProcessFailsImmediatelyForPermanentError(t *testing.T) {
	store := &fakeStore{failureAction: domain.FailureActionFailed}
	aiClient := &fakeAIClient{err: permanentError{}}
	service := New(store, aiClient, 3)

	err := service.Process(context.Background(), domain.Message{JobID: "job-1", AlunoID: "aluno-1"})
	if !errors.Is(err, ErrJobFailed) {
		t.Fatalf("Process() error = %v, want ErrJobFailed", err)
	}

	if store.failureMaxAttempts != 1 {
		t.Fatalf("failureMaxAttempts = %d, want 1", store.failureMaxAttempts)
	}
}

func TestServiceProcessSkipsStaleLease(t *testing.T) {
	store := &fakeStore{
		disciplines: []domain.DisciplinePerformance{{ID: "disciplina-1", Nome: "Matematica", Percentual: 42}},
		saveErr:     domain.ErrProcessingLeaseLost,
	}
	aiClient := &fakeAIClient{content: "Insight real"}
	service := New(store, aiClient, 3)

	if err := service.Process(context.Background(), domain.Message{JobID: "job-1", AlunoID: "aluno-1"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if store.failureCalled {
		t.Fatal("stale lease should not register failure")
	}
}
