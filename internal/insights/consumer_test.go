package insights

import (
	"context"
	"io"
	"log/slog"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeDelivery struct {
	body          []byte
	acked         bool
	nacked        bool
	requeueOnNack bool
}

func (d *fakeDelivery) Body() []byte {
	return d.body
}

func (d *fakeDelivery) Ack(_ bool) error {
	d.acked = true
	return nil
}

func (d *fakeDelivery) Nack(_ bool, requeue bool) error {
	d.nacked = true
	d.requeueOnNack = requeue
	return nil
}

func TestConsumerAcksValidMessage(t *testing.T) {
	delivery := &fakeDelivery{body: []byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`)}
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewService(&fakeStore{}, &fakeAIClient{content: "Insight real"}, 3),
	)

	consumer.handleDelivery(context.Background(), delivery)

	if !delivery.acked {
		t.Fatal("valid delivery was not acked")
	}

	if delivery.nacked {
		t.Fatal("valid delivery was nacked")
	}
}

func TestConsumerNacksInvalidMessageWithoutRequeue(t *testing.T) {
	delivery := &fakeDelivery{body: []byte(`invalid`)}
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewService(&fakeStore{}, &fakeAIClient{content: "Insight real"}, 3),
	)

	consumer.handleDelivery(context.Background(), delivery)

	if !delivery.nacked {
		t.Fatal("invalid delivery was not nacked")
	}

	if delivery.requeueOnNack {
		t.Fatal("invalid delivery should not be requeued")
	}

	if delivery.acked {
		t.Fatal("invalid delivery was acked")
	}
}

func TestConsumerNacksUnexpectedProcessingErrorWithRequeue(t *testing.T) {
	delivery := &fakeDelivery{body: []byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`)}
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewService(&fakeStore{beginErr: context.Canceled}, &fakeAIClient{content: "Insight real"}, 3),
	)

	consumer.handleDelivery(context.Background(), delivery)

	if !delivery.nacked {
		t.Fatal("failed processing delivery was not nacked")
	}

	if !delivery.requeueOnNack {
		t.Fatal("failed processing delivery should be requeued")
	}

	if delivery.acked {
		t.Fatal("failed processing delivery was acked")
	}
}

func TestConsumerAcksRetryScheduledError(t *testing.T) {
	delivery := &fakeDelivery{body: []byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`)}
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewService(&fakeStore{failureAction: FailureActionRetry}, &fakeAIClient{err: context.Canceled}, 3),
	)

	consumer.handleDelivery(context.Background(), delivery)

	if !delivery.acked {
		t.Fatal("retry scheduled delivery was not acked")
	}

	if delivery.nacked {
		t.Fatal("retry scheduled delivery was nacked")
	}
}

func TestConsumerStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deliveries := make(chan amqp.Delivery)
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewService(&fakeStore{}, &fakeAIClient{content: "Insight real"}, 3),
	)
	consumer.Run(ctx, deliveries)
}
