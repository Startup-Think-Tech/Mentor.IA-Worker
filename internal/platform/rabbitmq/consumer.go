package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (c *Client) openConsumerChannel() error {
	channel, err := c.connection.Channel()
	if err != nil {
		return fmt.Errorf("falha ao abrir canal de consumo do RabbitMQ: %w", err)
	}

	if err := channel.Qos(c.prefetch, 0, false); err != nil {
		_ = channel.Close()
		return fmt.Errorf("falha ao configurar prefetch do RabbitMQ: %w", err)
	}

	c.consumerChannel = channel
	return nil
}

func (c *Client) Consume(consumerName string) (<-chan amqp.Delivery, error) {
	deliveries, err := c.consumerChannel.Consume(c.queue.Name, consumerName, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar consumo da fila RabbitMQ: %w", err)
	}

	return deliveries, nil
}
