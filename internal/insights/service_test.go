package insights

import (
	"context"
	"errors"
	"testing"
)

type fakeProcessor struct {
	message Message
	err     error
	called  bool
}

func (p *fakeProcessor) ProcessWithFakeResult(_ context.Context, message Message) error {
	p.called = true
	p.message = message
	return p.err
}

func TestServiceProcess(t *testing.T) {
	processor := &fakeProcessor{}
	service := NewService(processor)
	message := Message{JobID: "job-1", AlunoID: "aluno-1"}

	if err := service.Process(context.Background(), message); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if !processor.called {
		t.Fatal("processor was not called")
	}

	if processor.message != message {
		t.Fatalf("message = %#v, want %#v", processor.message, message)
	}
}

func TestServiceProcessReturnsProcessorError(t *testing.T) {
	expectedErr := errors.New("database failed")
	service := NewService(&fakeProcessor{err: expectedErr})

	err := service.Process(context.Background(), Message{JobID: "job-1", AlunoID: "aluno-1"})
	if err == nil {
		t.Fatal("Process() returned nil error")
	}
}
