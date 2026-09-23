package insights

import (
	"context"
	"log/slog"
	"time"
)

type RetryStore interface {
	ClaimDueRetryJobs(ctx context.Context, limit int) ([]Message, error)
}

type RetryPublisher interface {
	PublishInsightMessage(ctx context.Context, payload any) error
}

type RetryScheduler struct {
	logger    *slog.Logger
	store     RetryStore
	publisher RetryPublisher
	interval  time.Duration
	batchSize int
}

func NewRetryScheduler(logger *slog.Logger, store RetryStore, publisher RetryPublisher, interval time.Duration, batchSize int) *RetryScheduler {
	return &RetryScheduler{
		logger:    logger,
		store:     store,
		publisher: publisher,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (s *RetryScheduler) Run(ctx context.Context) {
	if s.interval <= 0 || s.batchSize <= 0 {
		s.logger.Error("scheduler de retry desabilitado por configuracao invalida")
		return
	}

	s.logger.Info("scheduler de retry iniciado", "intervalo", s.interval, "lote", s.batchSize)
	s.publishDueJobs(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler de retry finalizado")
			return
		case <-ticker.C:
			s.publishDueJobs(ctx)
		}
	}
}

func (s *RetryScheduler) publishDueJobs(ctx context.Context) {
	messages, err := s.store.ClaimDueRetryJobs(ctx, s.batchSize)
	if err != nil {
		s.logger.Error("falha ao buscar jobs para retry", "erro", err)
		return
	}

	for _, message := range messages {
		if err := s.publisher.PublishInsightMessage(ctx, message); err != nil {
			s.logger.Error("falha ao republicar job de insight", "job_id", message.JobID, "erro", err)
			continue
		}

		s.logger.Info("job de insight republicado para retry", "job_id", message.JobID)
	}
}
