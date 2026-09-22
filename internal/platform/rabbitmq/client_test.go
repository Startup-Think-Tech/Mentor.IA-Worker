package rabbitmq

import (
	"os"
	"testing"

	"github.com/daviPeter07/ai-worker/internal/testsupport"
)

func TestConnectRejectsEmptyURL(t *testing.T) {
	_, err := Connect(Config{QueueName: "insights_queue"})
	if err == nil {
		t.Fatal("Connect() returned nil error")
	}
}

func TestConnectRejectsEmptyQueueName(t *testing.T) {
	_, err := Connect(Config{URL: "amqp://usuario:senha@rabbitmq.example.com:5672"})
	if err == nil {
		t.Fatal("Connect() returned nil error")
	}
}

func TestConnectWithEnv(t *testing.T) {
	testsupport.LoadDotEnv(t)

	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		t.Skip("RABBITMQ_URL nao configurada")
	}

	queueName := os.Getenv("RABBITMQ_INSIGHTS_QUEUE")
	if queueName == "" {
		t.Skip("RABBITMQ_INSIGHTS_QUEUE nao configurada")
	}

	client, err := Connect(Config{
		URL:       rabbitURL,
		QueueName: queueName,
	})
	if err != nil {
		t.Fatalf("Connect() returned error: %v", err)
	}

	if client.QueueName() != queueName {
		t.Fatalf("QueueName() = %q, want %q", client.QueueName(), queueName)
	}

	if client.PrefetchCount() != prefetchCount {
		t.Fatalf("PrefetchCount() = %d, want %d", client.PrefetchCount(), prefetchCount)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}
