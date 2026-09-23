package insights

import (
	"context"
	"errors"
	"testing"
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
	failureAction    FailureAction
	disciplines      []DisciplinePerformance
	savedContent     string
	savedDisciplines []DisciplinePerformance
}

func (s *fakeStore) BeginProcessing(context.Context, Message) (bool, error) {
	s.beginCalled = true
	return s.alreadyHandled, s.beginErr
}

func (s *fakeStore) FindLowestPerformanceDisciplines(context.Context, string, int) ([]DisciplinePerformance, error) {
	s.findCalled = true
	return s.disciplines, s.findErr
}

func (s *fakeStore) SaveInsightResult(_ context.Context, _ Message, content string, disciplines []DisciplinePerformance) error {
	s.saveCalled = true
	s.savedContent = content
	s.savedDisciplines = disciplines
	return s.saveErr
}

func (s *fakeStore) RegisterFailure(context.Context, Message, int, string, error) (FailureAction, error) {
	s.failureCalled = true
	if s.failureAction == "" {
		return FailureActionRetry, nil
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
	store := &fakeStore{disciplines: []DisciplinePerformance{{ID: "disciplina-1", Nome: "Matematica", Percentual: 42}}}
	aiClient := &fakeAIClient{content: "Insight real"}
	service := NewService(store, aiClient, 3)

	if err := service.Process(context.Background(), Message{JobID: "job-1", AlunoID: "aluno-1"}); err != nil {
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
	service := NewService(store, aiClient, 3)

	if err := service.Process(context.Background(), Message{JobID: "job-1", AlunoID: "aluno-1"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if aiClient.called {
		t.Fatal("AI client should not be called for already handled job")
	}
}

func TestServiceProcessRegistersRetryOnAIError(t *testing.T) {
	store := &fakeStore{failureAction: FailureActionRetry}
	aiClient := &fakeAIClient{err: errors.New("openrouter unavailable")}
	service := NewService(store, aiClient, 3)

	err := service.Process(context.Background(), Message{JobID: "job-1", AlunoID: "aluno-1"})
	if !errors.Is(err, ErrRetryScheduled) {
		t.Fatalf("Process() error = %v, want ErrRetryScheduled", err)
	}

	if !store.failureCalled {
		t.Fatal("expected failure to be registered")
	}
}

func TestServiceProcessRegistersFinalFailure(t *testing.T) {
	store := &fakeStore{failureAction: FailureActionFailed}
	aiClient := &fakeAIClient{err: errors.New("openrouter failed")}
	service := NewService(store, aiClient, 3)

	err := service.Process(context.Background(), Message{JobID: "job-1", AlunoID: "aluno-1"})
	if !errors.Is(err, ErrJobFailed) {
		t.Fatalf("Process() error = %v, want ErrJobFailed", err)
	}
}
