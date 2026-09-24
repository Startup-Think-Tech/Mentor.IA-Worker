package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const prefetchCount = 1

type Config struct {
	URL       string
	Queue     string
	DLQ       string
	QueueName string
}

type Client struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	confirmMu  sync.Mutex
	confirms   <-chan amqp.Confirmation
	queue      amqp.Queue
	dlq        amqp.Queue
}

func Connect(config Config) (*Client, error) {
	if config.Queue == "" {
		config.Queue = config.QueueName
	}

	if config.URL == "" {
		return nil, fmt.Errorf("RABBITMQ_URL nao pode estar vazio")
	}

	if config.Queue == "" {
		return nil, fmt.Errorf("RABBITMQ_INSIGHTS_QUEUE nao pode estar vazio")
	}

	if config.DLQ == "" {
		return nil, fmt.Errorf("RABBITMQ_INSIGHTS_DLQ nao pode estar vazio")
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

	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("falha ao habilitar publisher confirms do RabbitMQ: %w", err)
	}
	confirms := channel.NotifyPublish(make(chan amqp.Confirmation, 1))

	queue, err := channel.QueueDeclare(
		config.Queue,
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

	dlq, err := channel.QueueDeclare(
		config.DLQ,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("falha ao declarar DLQ do RabbitMQ: %w", err)
	}

	if err := channel.Qos(prefetchCount, 0, false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("falha ao configurar prefetch do RabbitMQ: %w", err)
	}

	return &Client{
		connection: connection,
		channel:    channel,
		confirms:   confirms,
		queue:      queue,
		dlq:        dlq,
	}, nil
}

func (c *Client) QueueName() string {
	return c.queue.Name
}

func (c *Client) PrefetchCount() int {
	return prefetchCount
}

func (c *Client) DLQName() string {
	return c.dlq.Name
}

func (c *Client) PublishInsightMessage(ctx context.Context, payload any) error {
	return c.publishJSON(ctx, c.queue.Name, payload)
}

func (c *Client) PublishToDLQ(ctx context.Context, payload any) error {
	return c.publishJSON(ctx, c.dlq.Name, payload)
}

func (c *Client) PublishRawToDLQ(ctx context.Context, body []byte) error {
	return c.publish(ctx, c.dlq.Name, amqp.Publishing{
		ContentType:  "application/octet-stream",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
}

func (c *Client) publishJSON(ctx context.Context, routingKey string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("falha ao serializar mensagem RabbitMQ: %w", err)
	}

	return c.publish(ctx, routingKey, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
}

func (c *Client) publish(ctx context.Context, routingKey string, publishing amqp.Publishing) error {
	c.confirmMu.Lock()
	defer c.confirmMu.Unlock()

	if err := c.channel.PublishWithContext(
		ctx,
		"",
		routingKey,
		false,
		false,
		publishing,
	); err != nil {
		return fmt.Errorf("falha ao publicar mensagem no RabbitMQ: %w", err)
	}

	select {
	case confirmation, ok := <-c.confirms:
		if !ok {
			return fmt.Errorf("canal de confirmacao do RabbitMQ fechado")
		}

		if !confirmation.Ack {
			return fmt.Errorf("RabbitMQ rejeitou publicacao")
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("contexto cancelado aguardando confirmacao do RabbitMQ: %w", ctx.Err())
	}
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
