package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/service"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Delivery interface {
	Body() []byte
	Ack(multiple bool) error
	Nack(multiple bool, requeue bool) error
}

type Processor interface {
	Process(ctx context.Context, message domain.Message) error
}

type Consumer struct {
	processor           Processor
	deadLetterPublisher DeadLetterPublisher
	logger              *slog.Logger
	concurrency         int
}

type DeadLetterPublisher interface {
	PublishRawToDLQ(ctx context.Context, body []byte) error
	PublishToDLQ(ctx context.Context, payload any) error
}

type amqpDelivery struct {
	delivery amqp.Delivery
}

func (d amqpDelivery) Body() []byte {
	return d.delivery.Body
}

func (d amqpDelivery) Ack(multiple bool) error {
	return d.delivery.Ack(multiple)
}

func (d amqpDelivery) Nack(multiple bool, requeue bool) error {
	return d.delivery.Nack(multiple, requeue)
}

func NewConsumer(logger *slog.Logger, processor Processor, deadLetterPublisher DeadLetterPublisher, concurrency int) *Consumer {
	return &Consumer{logger: logger, processor: processor, deadLetterPublisher: deadLetterPublisher, concurrency: concurrency}
}

func (c *Consumer) Run(ctx context.Context, deliveries <-chan amqp.Delivery) error {
	if c.concurrency <= 0 {
		return fmt.Errorf("concorrencia do consumer deve ser maior que zero")
	}

	c.logger.Info("consumer de insights iniciado", "concorrencia", c.concurrency)

	jobs := make(chan amqp.Delivery)
	var workers sync.WaitGroup
	for range c.concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case delivery, ok := <-jobs:
					if !ok {
						return
					}

					c.handleDelivery(ctx, amqpDelivery{delivery: delivery})
				}
			}
		}()
	}

	defer func() {
		close(jobs)
		workers.Wait()
		c.logger.Info("consumer de insights finalizado")
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}

				return fmt.Errorf("canal de mensagens de insights fechado")
			}

			select {
			case jobs <- delivery:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery Delivery) {
	message, err := domain.ParseMessage(delivery.Body())
	if err != nil {
		c.logger.Error("mensagem de insight invalida", "erro", err)
		if c.deadLetterPublisher != nil {
			if dlqErr := c.deadLetterPublisher.PublishRawToDLQ(ctx, delivery.Body()); dlqErr == nil {
				if ackErr := delivery.Ack(false); ackErr != nil {
					c.logger.Error("falha ao confirmar mensagem invalida enviada para DLQ", "erro", ackErr)
				}
				return
			} else {
				c.logger.Error("falha ao enviar mensagem invalida para DLQ", "erro", dlqErr)
			}
		}
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			c.logger.Error("falha ao rejeitar mensagem de insight", "erro", nackErr)
		}
		return
	}

	c.logger.Info(
		"mensagem de insight recebida",
		"job_id", message.JobID,
		"aluno_id", message.AlunoID,
	)

	if err := c.processor.Process(ctx, message); err != nil {
		if errors.Is(err, service.ErrJobFailed) {
			c.logger.Error(
				"processamento de insight falhou definitivamente",
				"job_id", message.JobID,
				"erro", err,
			)
			if ackErr := delivery.Ack(false); ackErr != nil {
				c.logger.Error("falha ao confirmar mensagem com erro persistido na outbox", "erro", ackErr)
			}
			return
		}

		if errors.Is(err, service.ErrRetryScheduled) {
			c.logger.Error(
				"processamento de insight agendado para retentativa",
				"job_id", message.JobID,
				"erro", err,
			)
			if ackErr := delivery.Ack(false); ackErr != nil {
				c.logger.Error("falha ao confirmar mensagem com retry persistido", "erro", ackErr)
			}
			return
		}

		c.logger.Error(
			"falha ao processar mensagem de insight",
			"job_id", message.JobID,
			"erro", err,
		)
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			c.logger.Error("falha ao reenfileirar mensagem de insight", "erro", nackErr)
		}
		return
	}

	if err := delivery.Ack(false); err != nil {
		c.logger.Error("falha ao confirmar mensagem de insight", "erro", err)
		return
	}

	c.logger.Info("mensagem de insight confirmada", "job_id", message.JobID)
}
