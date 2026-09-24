package rabbitmq

import (
	"os"
	"testing"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/testsupport"
)

func TestConnectRejectsEmptyURL(t *testing.T) {
	_, err := Connect(Config{Queue: "insights_queue", DLQ: "insights_dlq"})
	if err == nil {
		t.Fatal("Connect() returned nil error")
	}
}

func TestConnectRejectsEmptyQueueName(t *testing.T) {
	_, err := Connect(Config{URL: "amqp://usuario:senha@rabbitmq.example.com:5672", DLQ: "insights_dlq"})
	if err == nil {
		t.Fatal("Connect() returned nil error")
	}
}

func TestConnectRejectsEmptyDLQ(t *testing.T) {
	_, err := Connect(Config{URL: "amqp://usuario:senha@rabbitmq.example.com:5672", Queue: "insights_queue"})
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

	dlqName := os.Getenv("RABBITMQ_INSIGHTS_DLQ")
	if dlqName == "" {
		dlqName = "insights_dlq"
	}

	client, err := Connect(Config{
		URL:      rabbitURL,
		Queue:    queueName,
		DLQ:      dlqName,
		Prefetch: 2,
	})
	if err != nil {
		t.Fatalf("Connect() returned error: %v", err)
	}

	if client.QueueName() != queueName {
		t.Fatalf("QueueName() = %q, want %q", client.QueueName(), queueName)
	}

	if client.PrefetchCount() != 2 {
		t.Fatalf("PrefetchCount() = %d, want %d", client.PrefetchCount(), 2)
	}

	if client.DLQName() != dlqName {
		t.Fatalf("DLQName() = %q, want %q", client.DLQName(), dlqName)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}
