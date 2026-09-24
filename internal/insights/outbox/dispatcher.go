package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/daviPeter07/ai-worker/internal/insights/domain"
)

type Store interface {
	DispatchPendingOutboxEvents(ctx context.Context, limit int, publish domain.OutboxPublisher) (int, error)
}

type MessagePublisher interface {
	PublishInsightMessage(ctx context.Context, payload any) error
}

type Dispatcher struct {
	logger    *slog.Logger
	store     Store
	publisher MessagePublisher
	interval  time.Duration
	batchSize int
}

func NewDispatcher(logger *slog.Logger, store Store, publisher MessagePublisher, interval time.Duration, batchSize int) *Dispatcher {
	return &Dispatcher{
		logger:    logger,
		store:     store,
		publisher: publisher,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	if d.interval <= 0 || d.batchSize <= 0 {
		d.logger.Error("dispatcher de outbox desabilitado por configuracao invalida")
		return
	}

	d.logger.Info("dispatcher de outbox iniciado", "intervalo", d.interval, "lote", d.batchSize)
	d.dispatch(ctx)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("dispatcher de outbox finalizado")
			return
		case <-ticker.C:
			d.dispatch(ctx)
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context) {
	count, err := d.store.DispatchPendingOutboxEvents(ctx, d.batchSize, d.publish)
	if err != nil {
		d.logger.Error("falha ao despachar outbox", "erro", err)
		return
	}

	if count > 0 {
		d.logger.Info("eventos de outbox publicados", "quantidade", count)
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
	default:
		return fmt.Errorf("tipo de evento de outbox desconhecido: %s", event.Type)
	}
}
