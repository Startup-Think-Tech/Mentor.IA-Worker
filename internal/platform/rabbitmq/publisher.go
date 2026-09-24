package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (c *Client) openPublisherChannel() error {
	channel, err := c.connection.Channel()
	if err != nil {
		return fmt.Errorf("falha ao abrir canal de publicacao do RabbitMQ: %w", err)
	}

	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		return fmt.Errorf("falha ao habilitar publisher confirms do RabbitMQ: %w", err)
	}

	c.publisherChannel = channel
	c.confirms = channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	c.returns = channel.NotifyReturn(make(chan amqp.Return, 1))
	return nil
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
	c.publisherMu.Lock()
	defer c.publisherMu.Unlock()

	if err := c.publisherChannel.PublishWithContext(ctx, "", routingKey, true, false, publishing); err != nil {
		return fmt.Errorf("falha ao publicar mensagem no RabbitMQ: %w", err)
	}

	for {
		select {
		case returned, ok := <-c.returns:
			if !ok {
				return fmt.Errorf("canal de retorno do RabbitMQ fechado")
			}
			return fmt.Errorf("mensagem nao roteavel no RabbitMQ: codigo=%d texto=%s", returned.ReplyCode, returned.ReplyText)
		case confirmation, ok := <-c.confirms:
			if !ok {
				return fmt.Errorf("canal de confirmacao do RabbitMQ fechado")
			}
			if !confirmation.Ack {
				return fmt.Errorf("RabbitMQ rejeitou publicacao")
			}

			select {
			case returned := <-c.returns:
				return fmt.Errorf("mensagem nao roteavel no RabbitMQ: codigo=%d texto=%s", returned.ReplyCode, returned.ReplyText)
			default:
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("contexto cancelado aguardando confirmacao do RabbitMQ: %w", ctx.Err())
		}
	}
}
