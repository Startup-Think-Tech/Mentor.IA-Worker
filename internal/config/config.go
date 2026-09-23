package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultNodeEnv               = "development"
	defaultDatabaseURL           = "postgresql://mentor_ia:mentor_ia@localhost:5432/mentor_ia?schema=public"
	defaultInsightRetryBatchSize = 10
	defaultInsightRetryPollMS    = 30000
	defaultRabbitMQURL           = "amqp://mentor_ia:mentor_ia@localhost:5672"
	defaultRabbitMQInsightsDLQ   = "insights_dlq"
	defaultRabbitMQInsightsQueue = "insights_queue"
	defaultAIProvider            = "openrouter"
	defaultAIModel               = "openrouter/free"
	defaultAIRequestTimeoutMS    = 60000
	defaultInsightMaxAttempts    = 3
)

type Config struct {
	NodeEnv               string
	DatabaseURL           string
	InsightRetryBatchSize int
	InsightRetryPoll      time.Duration
	RabbitMQURL           string
	RabbitMQInsightsDLQ   string
	RabbitMQInsightsQueue string
	AIProvider            string
	AIProviderAPIKey      string
	AIModel               string
	AIRequestTimeout      time.Duration
	InsightMaxAttempts    int
}

func Load() (Config, error) {
	_ = godotenv.Load()

	requestTimeoutMS, err := getEnvInt("AI_REQUEST_TIMEOUT_MS", defaultAIRequestTimeoutMS)
	if err != nil {
		return Config{}, err
	}

	maxAttempts, err := getEnvInt("INSIGHT_MAX_ATTEMPTS", defaultInsightMaxAttempts)
	if err != nil {
		return Config{}, err
	}

	retryPollMS, err := getEnvInt("INSIGHT_RETRY_POLL_INTERVAL_MS", defaultInsightRetryPollMS)
	if err != nil {
		return Config{}, err
	}

	retryBatchSize, err := getEnvInt("INSIGHT_RETRY_BATCH_SIZE", defaultInsightRetryBatchSize)
	if err != nil {
		return Config{}, err
	}

	if requestTimeoutMS <= 0 {
		return Config{}, fmt.Errorf("AI_REQUEST_TIMEOUT_MS must be greater than zero")
	}

	if maxAttempts <= 0 {
		return Config{}, fmt.Errorf("INSIGHT_MAX_ATTEMPTS must be greater than zero")
	}

	if retryPollMS <= 0 {
		return Config{}, fmt.Errorf("INSIGHT_RETRY_POLL_INTERVAL_MS must be greater than zero")
	}

	if retryBatchSize <= 0 {
		return Config{}, fmt.Errorf("INSIGHT_RETRY_BATCH_SIZE must be greater than zero")
	}

	return Config{
		NodeEnv:               getEnvString("NODE_ENV", defaultNodeEnv),
		DatabaseURL:           getEnvString("DATABASE_URL", defaultDatabaseURL),
		InsightRetryBatchSize: retryBatchSize,
		InsightRetryPoll:      time.Duration(retryPollMS) * time.Millisecond,
		RabbitMQURL:           getEnvString("RABBITMQ_URL", defaultRabbitMQURL),
		RabbitMQInsightsDLQ:   getEnvString("RABBITMQ_INSIGHTS_DLQ", defaultRabbitMQInsightsDLQ),
		RabbitMQInsightsQueue: getEnvString("RABBITMQ_INSIGHTS_QUEUE", defaultRabbitMQInsightsQueue),
		AIProvider:            getEnvString("AI_PROVIDER", defaultAIProvider),
		AIProviderAPIKey:      strings.TrimSpace(os.Getenv("AI_PROVIDER_API_KEY")),
		AIModel:               getEnvString("AI_MODEL", defaultAIModel),
		AIRequestTimeout:      time.Duration(requestTimeoutMS) * time.Millisecond,
		InsightMaxAttempts:    maxAttempts,
	}, nil
}

func (c Config) LogAttrs() []any {
	return []any{
		slog.String("node_env", c.NodeEnv),
		slog.String("database_url", maskURL(c.DatabaseURL)),
		slog.Int("insight_retry_batch_size", c.InsightRetryBatchSize),
		slog.Duration("insight_retry_poll", c.InsightRetryPoll),
		slog.String("rabbitmq_url", maskURL(c.RabbitMQURL)),
		slog.String("rabbitmq_insights_dlq", c.RabbitMQInsightsDLQ),
		slog.String("rabbitmq_insights_queue", c.RabbitMQInsightsQueue),
		slog.String("ai_provider", c.AIProvider),
		slog.String("ai_model", c.AIModel),
		slog.Duration("ai_request_timeout", c.AIRequestTimeout),
		slog.Int("insight_max_attempts", c.InsightMaxAttempts),
		slog.Bool("ai_provider_api_key_configured", c.AIProviderAPIKey != ""),
	}
}

func getEnvString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer", key)
	}

	return parsed, nil
}

func maskURL(value string) string {
	if value == "" {
		return ""
	}

	if !strings.Contains(value, "@") || !strings.Contains(value, "://") {
		return value
	}

	parts := strings.SplitN(value, "://", 2)
	credentialsAndHost := parts[1]
	atIndex := strings.LastIndex(credentialsAndHost, "@")
	if atIndex == -1 {
		return value
	}

	return parts[0] + "://***:***@" + credentialsAndHost[atIndex+1:]
}
