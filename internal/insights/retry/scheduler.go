package retry

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Store interface {
	ScheduleDueRetryJobs(ctx context.Context, limit int) (int, error)
}

type Scheduler struct {
	logger    *slog.Logger
	store     Store
	interval  time.Duration
	batchSize int
}

func NewScheduler(logger *slog.Logger, store Store, interval time.Duration, batchSize int) *Scheduler {
	return &Scheduler{
		logger:    logger,
		store:     store,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	if s.interval <= 0 || s.batchSize <= 0 {
		return fmt.Errorf("scheduler de retry com configuracao invalida")
	}

	s.logger.Info("scheduler de retry iniciado", "intervalo", s.interval, "lote", s.batchSize)
	s.publishDueJobs(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler de retry finalizado")
			return nil
		case <-ticker.C:
			s.publishDueJobs(ctx)
		}
	}
}

func (s *Scheduler) publishDueJobs(ctx context.Context) {
	count, err := s.store.ScheduleDueRetryJobs(ctx, s.batchSize)
	if err != nil {
		s.logger.Error("falha ao agendar jobs para retry", "erro", err)
		return
	}

	if count > 0 {
		s.logger.Info("jobs de insight agendados para retry via outbox", "quantidade", count)
	}
}
