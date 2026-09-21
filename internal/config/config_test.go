package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("NODE_ENV", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_INSIGHTS_QUEUE", "")
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("AI_PROVIDER_API_KEY", "")
	t.Setenv("AI_MODEL", "")
	t.Setenv("AI_REQUEST_TIMEOUT_MS", "")
	t.Setenv("INSIGHT_MAX_ATTEMPTS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.NodeEnv != defaultNodeEnv {
		t.Fatalf("NodeEnv = %q, want %q", cfg.NodeEnv, defaultNodeEnv)
	}

	if cfg.AIRequestTimeout != defaultAIRequestTimeoutMS*time.Millisecond {
		t.Fatalf("AIRequestTimeout = %s, want %s", cfg.AIRequestTimeout, defaultAIRequestTimeoutMS*time.Millisecond)
	}

	if cfg.InsightMaxAttempts != defaultInsightMaxAttempts {
		t.Fatalf("InsightMaxAttempts = %d, want %d", cfg.InsightMaxAttempts, defaultInsightMaxAttempts)
	}
}

func TestLoadRejectsInvalidInteger(t *testing.T) {
	t.Setenv("AI_REQUEST_TIMEOUT_MS", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
}

func TestLogAttrsDoNotExposeSecrets(t *testing.T) {
	const apiKey = "secret-api-key"
	cfg := Config{
		NodeEnv:               "development",
		DatabaseURL:           "postgresql://user:pass@localhost:5432/db",
		RabbitMQURL:           "amqp://user:pass@localhost:5672",
		RabbitMQInsightsQueue: "insights_queue",
		AIProvider:            "openai",
		AIProviderAPIKey:      apiKey,
		AIModel:               "gpt-4o-mini",
		AIRequestTimeout:      time.Minute,
		InsightMaxAttempts:    3,
	}

	attrs := fmt.Sprint(cfg.LogAttrs())
	if strings.Contains(attrs, apiKey) {
		t.Fatal("LogAttrs() exposed AI_PROVIDER_API_KEY")
	}

	if strings.Contains(attrs, "user:pass") {
		t.Fatal("LogAttrs() exposed URL credentials")
	}
}
