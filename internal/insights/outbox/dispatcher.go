package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
)

type Store interface {
	ClaimPendingOutboxEvents(ctx context.Context, limit int, lockDuration time.Duration) ([]domain.OutboxEvent, error)
	MarkOutboxEventPublished(ctx context.Context, event domain.OutboxEvent) error
	MarkOutboxEventFailed(ctx context.Context, event domain.OutboxEvent, maxAttempts int, failure error) (bool, error)
}

type MessagePublisher interface {
	PublishInsightMessage(ctx context.Context, payload any) error
	PublishToDLQ(ctx context.Context, payload any) error
}

type Dispatcher struct {
	logger      *slog.Logger
	store       Store
	publisher   MessagePublisher
	interval    time.Duration
	batchSize   int
	lock        time.Duration
	maxAttempts int
}

func NewDispatcher(logger *slog.Logger, store Store, publisher MessagePublisher, interval time.Duration, batchSize int, lock time.Duration, maxAttempts int) *Dispatcher {
	return &Dispatcher{
		logger:      logger,
		store:       store,
		publisher:   publisher,
		interval:    interval,
		batchSize:   batchSize,
		lock:        lock,
		maxAttempts: maxAttempts,
	}
}

func (d *Dispatcher) Run(ctx context.Context) error {
	if d.interval <= 0 || d.batchSize <= 0 || d.lock <= 0 || d.maxAttempts <= 0 {
		return fmt.Errorf("dispatcher de outbox com configuracao invalida")
	}

	d.logger.Info("dispatcher de outbox iniciado", "intervalo", d.interval, "lote", d.batchSize)
	d.dispatch(ctx)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("dispatcher de outbox finalizado")
			return nil
		case <-ticker.C:
			d.dispatch(ctx)
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context) {
	events, err := d.store.ClaimPendingOutboxEvents(ctx, d.batchSize, d.lock)
	if err != nil {
		d.logger.Error("falha ao adquirir eventos da outbox", "erro", err)
		return
	}

	published := 0
	for _, event := range events {
		if err := d.publish(ctx, event); err != nil {
			failed, markErr := d.store.MarkOutboxEventFailed(ctx, event, d.maxAttempts, err)
			if markErr != nil {
				d.logger.Error("falha ao registrar erro de evento da outbox", "evento_id", event.ID, "erro", markErr)
				continue
			}

			d.logger.Error("falha ao publicar evento da outbox", "evento_id", event.ID, "tentativa", event.Attempts, "falhou_definitivamente", failed, "erro", err)
			continue
		}

		if err := d.store.MarkOutboxEventPublished(ctx, event); err != nil {
			d.logger.Error("falha ao confirmar evento publicado da outbox", "evento_id", event.ID, "erro", err)
			continue
		}

		published++
	}

	if published > 0 {
		d.logger.Info("eventos de outbox publicados", "quantidade", published)
	}
}

func (d *Dispatcher) publish(ctx context.Context, event domain.OutboxEvent) error {
	switch event.Type {
	case domain.OutboxTypeInsightRetryRequested:
		var message domain.Message
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return fmt.Errorf("payload de retry invalido: %w", err)
		}

		return d.publisher.PublishInsightMessage(ctx, message)
	case domain.OutboxTypeInsightFailed:
		var failure domain.InsightFailedEvent
		if err := json.Unmarshal(event.Payload, &failure); err != nil {
			return fmt.Errorf("payload de falha definitiva invalido: %w", err)
		}

		return d.publisher.PublishToDLQ(ctx, failure)
	default:
		return fmt.Errorf("tipo de evento de outbox desconhecido: %s", event.Type)
	}
}
