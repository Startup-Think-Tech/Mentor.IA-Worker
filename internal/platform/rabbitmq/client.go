package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const prefetchCount = 1

type Config struct {
	URL       string
	QueueName string
}

type Client struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	queue      amqp.Queue
}

func Connect(config Config) (*Client, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("RABBITMQ_URL nao pode estar vazio")
	}

	if config.QueueName == "" {
		return nil, fmt.Errorf("RABBITMQ_INSIGHTS_QUEUE nao pode estar vazio")
	}

	connection, err := amqp.Dial(config.URL)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar no RabbitMQ: %w", err)
	}

	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("falha ao abrir canal do RabbitMQ: %w", err)
	}

	queue, err := channel.QueueDeclare(
		config.QueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("falha ao declarar fila do RabbitMQ: %w", err)
	}

	if err := channel.Qos(prefetchCount, 0, false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("falha ao configurar prefetch do RabbitMQ: %w", err)
	}

	return &Client{
		connection: connection,
		channel:    channel,
		queue:      queue,
	}, nil
}

func (c *Client) QueueName() string {
	return c.queue.Name
}

func (c *Client) PrefetchCount() int {
	return prefetchCount
}

func (c *Client) Consume(consumerName string) (<-chan amqp.Delivery, error) {
	deliveries, err := c.channel.Consume(
		c.queue.Name,
		consumerName,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar consumo da fila RabbitMQ: %w", err)
	}

	return deliveries, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			return err
		}
	}

	if c.connection != nil {
		return c.connection.Close()
	}

	return nil
}
