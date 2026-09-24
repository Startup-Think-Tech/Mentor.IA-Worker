package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/service"
	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeDelivery struct {
	body          []byte
	acked         bool
	nacked        bool
	requeueOnNack bool
}

type fakeProcessor struct {
	err error
}

func (p *fakeProcessor) Process(context.Context, domain.Message) error {
	return p.err
}

type fakeDeadLetterPublisher struct {
	rawMessages  [][]byte
	jsonMessages []any
	err          error
}

func (p *fakeDeadLetterPublisher) PublishRawToDLQ(_ context.Context, body []byte) error {
	if p.err != nil {
		return p.err
	}

	p.rawMessages = append(p.rawMessages, body)
	return nil
}

func (p *fakeDeadLetterPublisher) PublishToDLQ(_ context.Context, payload any) error {
	if p.err != nil {
		return p.err
	}

	p.jsonMessages = append(p.jsonMessages, payload)
	return nil
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
		&fakeProcessor{},
		&fakeDeadLetterPublisher{},
		1,
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
	dlqPublisher := &fakeDeadLetterPublisher{}
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&fakeProcessor{},
		dlqPublisher,
		1,
	)

	consumer.handleDelivery(context.Background(), delivery)

	if !delivery.acked {
		t.Fatal("invalid delivery sent to DLQ was not acked")
	}

	if delivery.nacked {
		t.Fatal("invalid delivery sent to DLQ was nacked")
	}

	if len(dlqPublisher.rawMessages) != 1 {
		t.Fatalf("raw DLQ messages = %d, want 1", len(dlqPublisher.rawMessages))
	}
}

func TestConsumerNacksUnexpectedProcessingErrorWithRequeue(t *testing.T) {
	delivery := &fakeDelivery{body: []byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`)}
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&fakeProcessor{err: context.Canceled},
		&fakeDeadLetterPublisher{},
		1,
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
		&fakeProcessor{err: fmt.Errorf("%w: failed", service.ErrRetryScheduled)},
		&fakeDeadLetterPublisher{},
		1,
	)

	consumer.handleDelivery(context.Background(), delivery)

	if !delivery.acked {
		t.Fatal("retry scheduled delivery was not acked")
	}

	if delivery.nacked {
		t.Fatal("retry scheduled delivery was nacked")
	}
}

func TestConsumerPublishesFinalFailureToDLQ(t *testing.T) {
	delivery := &fakeDelivery{body: []byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`)}
	dlqPublisher := &fakeDeadLetterPublisher{}
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&fakeProcessor{err: fmt.Errorf("%w: failed", service.ErrJobFailed)},
		dlqPublisher,
		1,
	)

	consumer.handleDelivery(context.Background(), delivery)

	if !delivery.acked {
		t.Fatal("final failed delivery was not acked")
	}

	if len(dlqPublisher.jsonMessages) != 1 {
		t.Fatalf("json DLQ messages = %d, want 1", len(dlqPublisher.jsonMessages))
	}
}

func TestConsumerStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deliveries := make(chan amqp.Delivery)
	consumer := NewConsumer(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&fakeProcessor{},
		&fakeDeadLetterPublisher{},
		1,
	)
	if err := consumer.Run(ctx, deliveries); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
}
