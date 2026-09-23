package insights

import (
	"context"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Delivery interface {
	Body() []byte
	Ack(multiple bool) error
	Nack(multiple bool, requeue bool) error
}

type Consumer struct {
	service *Service
	logger  *slog.Logger
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

func NewConsumer(logger *slog.Logger, service *Service) *Consumer {
	return &Consumer{logger: logger, service: service}
}

func (c *Consumer) Run(ctx context.Context, deliveries <-chan amqp.Delivery) {
	c.logger.Info("consumer de insights iniciado")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer de insights finalizado")
			return
		case delivery, ok := <-deliveries:
			if !ok {
				c.logger.Info("canal de mensagens de insights fechado")
				return
			}

			c.handleDelivery(ctx, amqpDelivery{delivery: delivery})
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery Delivery) {
	message, err := ParseMessage(delivery.Body())
	if err != nil {
		c.logger.Error("mensagem de insight invalida", "erro", err)
		if nackErr := delivery.Nack(false, false); nackErr != nil {
			c.logger.Error("falha ao rejeitar mensagem de insight", "erro", nackErr)
		}
		return
	}

	c.logger.Info(
		"mensagem de insight recebida",
		"job_id", message.JobID,
		"aluno_id", message.AlunoID,
	)

	if err := c.service.Process(ctx, message); err != nil {
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
