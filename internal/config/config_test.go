package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_INSIGHTS_DLQ", "")
	t.Setenv("RABBITMQ_INSIGHTS_QUEUE", "")
	t.Setenv("RABBITMQ_PREFETCH", "")
	t.Setenv("RABBITMQ_PUBLISH_TIMEOUT_MS", "")
	t.Setenv("WORKER_CONCURRENCY", "")
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("AI_PROVIDER_API_KEY", "")
	t.Setenv("AI_MODEL", "")
	t.Setenv("AI_REQUEST_TIMEOUT_MS", "")
	t.Setenv("INSIGHT_PROCESSING_LEASE_SECONDS", "")
	t.Setenv("INSIGHT_MAX_ATTEMPTS", "")
	t.Setenv("INSIGHT_RETRY_BATCH_SIZE", "")
	t.Setenv("INSIGHT_RETRY_POLL_INTERVAL_MS", "")
	t.Setenv("OUTBOX_LOCK_SECONDS", "")
	t.Setenv("OUTBOX_BATCH_SIZE", "")
	t.Setenv("OUTBOX_MAX_ATTEMPTS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.AppEnv != defaultAppEnv {
		t.Fatalf("AppEnv = %q, want %q", cfg.AppEnv, defaultAppEnv)
	}

	if cfg.AIRequestTimeout != defaultAIRequestTimeoutMS*time.Millisecond {
		t.Fatalf("AIRequestTimeout = %s, want %s", cfg.AIRequestTimeout, defaultAIRequestTimeoutMS*time.Millisecond)
	}

	if cfg.InsightMaxAttempts != defaultInsightMaxAttempts {
		t.Fatalf("InsightMaxAttempts = %d, want %d", cfg.InsightMaxAttempts, defaultInsightMaxAttempts)
	}

	if cfg.InsightLease != defaultInsightLeaseSeconds*time.Second {
		t.Fatalf("InsightLease = %s, want %s", cfg.InsightLease, defaultInsightLeaseSeconds*time.Second)
	}

	if cfg.RabbitMQPublishTimeout != defaultRabbitMQPublishTimeoutMS*time.Millisecond {
		t.Fatalf("RabbitMQPublishTimeout = %s, want %s", cfg.RabbitMQPublishTimeout, defaultRabbitMQPublishTimeoutMS*time.Millisecond)
	}

	if cfg.RabbitMQInsightsDLQ != defaultRabbitMQInsightsDLQ {
		t.Fatalf("RabbitMQInsightsDLQ = %q, want %q", cfg.RabbitMQInsightsDLQ, defaultRabbitMQInsightsDLQ)
	}

	if cfg.InsightRetryBatchSize != defaultInsightRetryBatchSize {
		t.Fatalf("InsightRetryBatchSize = %d, want %d", cfg.InsightRetryBatchSize, defaultInsightRetryBatchSize)
	}
}

func TestLoadRejectsInvalidInteger(t *testing.T) {
	t.Setenv("AI_REQUEST_TIMEOUT_MS", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLoadRejectsMissingProductionConfiguration(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("AI_PROVIDER_API_KEY", "")
	t.Setenv("AI_PROVIDER_BASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLoadRejectsPrefetchBelowConcurrency(t *testing.T) {
	t.Setenv("WORKER_CONCURRENCY", "2")
	t.Setenv("RABBITMQ_PREFETCH", "1")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLoadRejectsInvalidRabbitMQPublishTimeout(t *testing.T) {
	t.Setenv("RABBITMQ_PUBLISH_TIMEOUT_MS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLoadRejectsRabbitMQPublishTimeoutAtOrAboveOutboxLock(t *testing.T) {
	t.Setenv("RABBITMQ_PUBLISH_TIMEOUT_MS", "30000")
	t.Setenv("OUTBOX_LOCK_SECONDS", "30")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLoadRejectsOutboxBatchWithoutPublishMargin(t *testing.T) {
	t.Setenv("OUTBOX_BATCH_SIZE", "5")
	t.Setenv("RABBITMQ_PUBLISH_TIMEOUT_MS", "5000")
	t.Setenv("OUTBOX_LOCK_SECONDS", "30")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLoadAcceptsInsightLeaseWithMinimumMargin(t *testing.T) {
	t.Setenv("AI_REQUEST_TIMEOUT_MS", "60000")
	t.Setenv("INSIGHT_PROCESSING_LEASE_SECONDS", "90")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
}

func TestLoadRejectsInsightLeaseBelowMinimumMargin(t *testing.T) {
	t.Setenv("AI_REQUEST_TIMEOUT_MS", "60000")
	t.Setenv("INSIGHT_PROCESSING_LEASE_SECONDS", "89")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLogAttrsDoNotExposeSecrets(t *testing.T) {
	const apiKey = "secret-api-key"
	const baseURL = "https://model-provider.example/v1"
	cfg := Config{
		AppEnv:                "development",
		DatabaseURL:           "postgresql://user:pass@localhost:5432/db",
		InsightRetryBatchSize: 10,
		InsightRetryPoll:      time.Minute,
		RabbitMQURL:           "amqp://user:pass@localhost:5672",
		RabbitMQInsightsDLQ:   "insights_dlq",
		RabbitMQInsightsQueue: "insights_queue",
		AIProvider:            "openai",
		AIProviderAPIKey:      apiKey,
		AIProviderBaseURL:     baseURL,
		AIModel:               "gpt-4o-mini",
		AIRequestTimeout:      time.Minute,
		InsightLease:          2 * time.Minute,
		InsightMaxAttempts:    3,
	}

	attrs := fmt.Sprint(cfg.LogAttrs())
	if strings.Contains(attrs, apiKey) {
		t.Fatal("LogAttrs() exposed AI_PROVIDER_API_KEY")
	}

	if strings.Contains(attrs, baseURL) {
		t.Fatal("LogAttrs() exposed AI_PROVIDER_BASE_URL")
	}

	if strings.Contains(attrs, "user:pass") {
		t.Fatal("LogAttrs() exposed URL credentials")
	}
}
