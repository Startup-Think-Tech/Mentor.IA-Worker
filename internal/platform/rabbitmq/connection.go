package rabbitmq

import (
	"errors"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultPrefetchCount = 1

type Config struct {
	URL       string
	Queue     string
	DLQ       string
	QueueName string
	Prefetch  int
}

type Client struct {
	connection       *amqp.Connection
	consumerChannel  *amqp.Channel
	publisherChannel *amqp.Channel
	connectionErrors <-chan *amqp.Error
	confirms         <-chan amqp.Confirmation
	returns          <-chan amqp.Return
	publisherMu      sync.Mutex
	queue            amqp.Queue
	dlq              amqp.Queue
	prefetch         int
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

	if config.Prefetch == 0 {
		config.Prefetch = defaultPrefetchCount
	}
	if config.Prefetch < 0 {
		return nil, fmt.Errorf("RABBITMQ_PREFETCH deve ser maior que zero")
	}

	connection, err := amqp.Dial(config.URL)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar no RabbitMQ: %w", err)
	}

	client := &Client{
		connection:       connection,
		connectionErrors: connection.NotifyClose(make(chan *amqp.Error, 1)),
		prefetch:         config.Prefetch,
	}

	if err := client.declareTopology(config.Queue, config.DLQ); err != nil {
		_ = connection.Close()
		return nil, err
	}

	if err := client.openConsumerChannel(); err != nil {
		_ = connection.Close()
		return nil, err
	}

	if err := client.openPublisherChannel(); err != nil {
		_ = client.consumerChannel.Close()
		_ = connection.Close()
		return nil, err
	}

	return client, nil
}

func (c *Client) QueueName() string {
	return c.queue.Name
}

func (c *Client) DLQName() string {
	return c.dlq.Name
}

func (c *Client) PrefetchCount() int {
	return c.prefetch
}

func (c *Client) Errors() <-chan *amqp.Error {
	return c.connectionErrors
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	var closeErrs []error
	if c.publisherChannel != nil {
		if err := c.publisherChannel.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			closeErrs = append(closeErrs, err)
		}
	}
	if c.consumerChannel != nil {
		if err := c.consumerChannel.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			closeErrs = append(closeErrs, err)
		}
	}
	if c.connection != nil {
		if err := c.connection.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			closeErrs = append(closeErrs, err)
		}
	}

	return errors.Join(closeErrs...)
}
