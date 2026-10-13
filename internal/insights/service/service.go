package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/prompt"
)

var (
	ErrJobFailed      = errors.New("job de insight falhou definitivamente")
	ErrRetryScheduled = errors.New("job de insight aguardando retentativa")
)

type Store interface {
	BeginProcessing(ctx context.Context, message domain.Message) (domain.ProcessingLease, bool, error)
	FindLowestPerformanceDisciplines(ctx context.Context, alunoID string, limit int) ([]domain.DisciplinePerformance, error)
	SaveInsightResult(ctx context.Context, lease domain.ProcessingLease, content string, disciplines []domain.DisciplinePerformance) error
	RegisterFailure(ctx context.Context, lease domain.ProcessingLease, maxAttempts int, code string, failure error) (domain.FailureAction, error)
}

type AIClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

type Service struct {
	store       Store
	aiClient    AIClient
	maxAttempts int
	logger      *slog.Logger
}

func New(logger *slog.Logger, store Store, aiClient AIClient, maxAttempts int) *Service {
	return &Service{logger: logger, store: store, aiClient: aiClient, maxAttempts: maxAttempts}
}

func (s *Service) Process(ctx context.Context, message domain.Message) (processErr error) {
	if s.store == nil {
		return fmt.Errorf("store de insights nao configurado")
	}

	if s.aiClient == nil {
		return fmt.Errorf("cliente de IA nao configurado")
	}

	if s.maxAttempts <= 0 {
		return fmt.Errorf("maxAttempts deve ser maior que zero")
	}

	lease, alreadyHandled, err := s.store.BeginProcessing(ctx, message)
	if err != nil {
		return fmt.Errorf("falha ao iniciar processamento do insight: %w", err)
	}

	if alreadyHandled {
		return nil
	}

	processingStartedAt := time.Now()
	result := "concluido"
	defer func() {
		if s.logger == nil {
			return
		}

		if processErr != nil {
			result = "erro_infraestrutura"
		}
		if errors.Is(processErr, ErrJobFailed) {
			result = "falhou"
		} else if errors.Is(processErr, ErrRetryScheduled) {
			result = "retentativa_agendada"
		}

		s.logger.Info(
			"metricas de processamento de insight",
			"job_id", message.JobID,
			"aluno_id", message.AlunoID,
			"tentativa", lease.Attempts,
			"espera_fila_ms", processingStartedAt.Sub(lease.CreatedAt).Milliseconds(),
			"duracao_processamento_ms", time.Since(processingStartedAt).Milliseconds(),
			"resultado", result,
			"erro", processErr,
		)
	}()

	disciplines, err := s.store.FindLowestPerformanceDisciplines(ctx, message.AlunoID, 3)
	if err != nil {
		return s.registerFailure(ctx, lease, "DISCIPLINES_QUERY_FAILED", err)
	}

	content, err := s.aiClient.Complete(ctx, prompt.Build(disciplines))
	if err != nil {
		return s.registerFailure(ctx, lease, "AI_COMPLETION_FAILED", err)
	}

	if err := s.store.SaveInsightResult(ctx, lease, content, disciplines); err != nil {
		if errors.Is(err, domain.ErrProcessingLeaseLost) {
			result = "lease_perdido"
			return nil
		}

		return s.registerFailure(ctx, lease, "INSIGHT_SAVE_FAILED", err)
	}

	return nil
}

func (s *Service) registerFailure(ctx context.Context, lease domain.ProcessingLease, code string, failure error) error {
	maxAttempts := s.maxAttempts
	if !isRetryable(failure) {
		maxAttempts = 1
	}

	action, err := s.store.RegisterFailure(ctx, lease, maxAttempts, code, failure)
	if err != nil {
		if errors.Is(err, domain.ErrProcessingLeaseLost) {
			return nil
		}

		return fmt.Errorf("%w; falha adicional ao registrar erro: %w", failure, err)
	}

	if action == domain.FailureActionFailed {
		return fmt.Errorf("%w: %v", ErrJobFailed, failure)
	}

	return fmt.Errorf("%w: %v", ErrRetryScheduled, failure)
}

func isRetryable(err error) bool {
	var retryable interface{ Retryable() bool }
	if errors.As(err, &retryable) {
		return retryable.Retryable()
	}

	return true
}
