package insights

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrJobFailed      = errors.New("job de insight falhou definitivamente")
	ErrRetryScheduled = errors.New("job de insight aguardando retentativa")
)

type Store interface {
	BeginProcessing(ctx context.Context, message Message) (bool, error)
	FindLowestPerformanceDisciplines(ctx context.Context, alunoID string, limit int) ([]DisciplinePerformance, error)
	SaveInsightResult(ctx context.Context, message Message, content string, disciplines []DisciplinePerformance) error
	RegisterFailure(ctx context.Context, message Message, maxAttempts int, code string, failure error) (FailureAction, error)
}

type AIClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

type Service struct {
	store       Store
	aiClient    AIClient
	maxAttempts int
}

func NewService(store Store, aiClient AIClient, maxAttempts int) *Service {
	return &Service{store: store, aiClient: aiClient, maxAttempts: maxAttempts}
}

func (s *Service) Process(ctx context.Context, message Message) error {
	if s.store == nil {
		return fmt.Errorf("store de insights nao configurado")
	}

	if s.aiClient == nil {
		return fmt.Errorf("cliente de IA nao configurado")
	}

	if s.maxAttempts <= 0 {
		return fmt.Errorf("maxAttempts deve ser maior que zero")
	}

	alreadyHandled, err := s.store.BeginProcessing(ctx, message)
	if err != nil {
		return fmt.Errorf("falha ao iniciar processamento do insight: %w", err)
	}

	if alreadyHandled {
		return nil
	}

	disciplines, err := s.store.FindLowestPerformanceDisciplines(ctx, message.AlunoID, 3)
	if err != nil {
		return s.registerFailure(ctx, message, "DISCIPLINES_QUERY_FAILED", err)
	}

	content, err := s.aiClient.Complete(ctx, BuildPrompt(disciplines))
	if err != nil {
		return s.registerFailure(ctx, message, "AI_COMPLETION_FAILED", err)
	}

	if err := s.store.SaveInsightResult(ctx, message, content, disciplines); err != nil {
		return s.registerFailure(ctx, message, "INSIGHT_SAVE_FAILED", err)
	}

	return nil
}

func (s *Service) registerFailure(ctx context.Context, message Message, code string, failure error) error {
	action, err := s.store.RegisterFailure(ctx, message, s.maxAttempts, code, failure)
	if err != nil {
		return fmt.Errorf("%w; falha adicional ao registrar erro: %w", failure, err)
	}

	if action == FailureActionFailed {
		return fmt.Errorf("%w: %v", ErrJobFailed, failure)
	}

	return fmt.Errorf("%w: %v", ErrRetryScheduled, failure)
}
