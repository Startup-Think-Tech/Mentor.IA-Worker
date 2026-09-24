package rabbitmq

import (
	"fmt"
)

func (c *Client) declareTopology(queueName string, dlqName string) error {
	channel, err := c.connection.Channel()
	if err != nil {
		return fmt.Errorf("falha ao abrir canal de topologia do RabbitMQ: %w", err)
	}
	defer channel.Close()

	queue, err := channel.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("falha ao declarar fila do RabbitMQ: %w", err)
	}

	dlq, err := channel.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("falha ao declarar DLQ do RabbitMQ: %w", err)
	}

	c.queue = queue
	c.dlq = dlq
	return nil
}
