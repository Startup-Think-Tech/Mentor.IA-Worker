package service

import (
	"context"
	"errors"
	"testing"

	"github.com/daviPeter07/ai-worker/internal/insights/domain"
)

type fakeStore struct {
	beginCalled      bool
	findCalled       bool
	saveCalled       bool
	failureCalled    bool
	alreadyHandled   bool
	beginErr         error
	findErr          error
	saveErr          error
	failureAction    domain.FailureAction
	disciplines      []domain.DisciplinePerformance
	savedContent     string
	savedDisciplines []domain.DisciplinePerformance
}

func (s *fakeStore) BeginProcessing(context.Context, domain.Message) (bool, error) {
	s.beginCalled = true
	return s.alreadyHandled, s.beginErr
}

func (s *fakeStore) FindLowestPerformanceDisciplines(context.Context, string, int) ([]domain.DisciplinePerformance, error) {
	s.findCalled = true
	return s.disciplines, s.findErr
}

func (s *fakeStore) SaveInsightResult(_ context.Context, _ domain.Message, content string, disciplines []domain.DisciplinePerformance) error {
	s.saveCalled = true
	s.savedContent = content
	s.savedDisciplines = disciplines
	return s.saveErr
}

func (s *fakeStore) RegisterFailure(context.Context, domain.Message, int, string, error) (domain.FailureAction, error) {
	s.failureCalled = true
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
