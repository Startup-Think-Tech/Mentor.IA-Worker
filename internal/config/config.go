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
	defaultAppEnv                = "development"
	defaultDatabaseURL           = "postgresql://mentor_ia:mentor_ia@localhost:5432/mentor_ia?schema=public"
	defaultInsightRetryBatchSize = 10
	defaultInsightRetryPollMS    = 30000
	defaultOutboxBatchSize       = 50
	defaultOutboxPollMS          = 5000
	defaultOutboxLockSeconds     = 30
	defaultOutboxMaxAttempts     = 5
	defaultRabbitMQURL           = "amqp://mentor_ia:mentor_ia@localhost:5672"
	defaultRabbitMQInsightsDLQ   = "insights_dlq"
	defaultRabbitMQInsightsQueue = "insights_queue"
	defaultAIProvider            = "openrouter"
	defaultAIModel               = "openrouter/free"
	defaultAIRequestTimeoutMS    = 60000
	defaultInsightLeaseSeconds   = 120
	defaultInsightMaxAttempts    = 3
	defaultWorkerConcurrency     = 1
)

type Config struct {
	AppEnv                string
	DatabaseURL           string
	InsightRetryBatchSize int
	InsightRetryPoll      time.Duration
	OutboxBatchSize       int
	OutboxPoll            time.Duration
	OutboxLock            time.Duration
	OutboxMaxAttempts     int
	RabbitMQURL           string
	RabbitMQInsightsDLQ   string
	RabbitMQInsightsQueue string
	RabbitMQPrefetch      int
	WorkerConcurrency     int
	AIProvider            string
	AIProviderAPIKey      string
	AIProviderBaseURL     string
	AIModel               string
	AIRequestTimeout      time.Duration
	InsightLease          time.Duration
	InsightMaxAttempts    int
}

func Load() (Config, error) {
	_ = godotenv.Load()
	appEnv := getEnvString("APP_ENV", defaultAppEnv)

	requestTimeoutMS, err := getEnvInt("AI_REQUEST_TIMEOUT_MS", defaultAIRequestTimeoutMS)
	if err != nil {
		return Config{}, err
	}

	maxAttempts, err := getEnvInt("INSIGHT_MAX_ATTEMPTS", defaultInsightMaxAttempts)
	if err != nil {
		return Config{}, err
	}

	leaseSeconds, err := getEnvInt("INSIGHT_PROCESSING_LEASE_SECONDS", defaultInsightLeaseSeconds)
	if err != nil {
		return Config{}, err
	}

	workerConcurrency, err := getEnvInt("WORKER_CONCURRENCY", defaultWorkerConcurrency)
	if err != nil {
		return Config{}, err
	}

	rabbitMQPrefetch, err := getEnvInt("RABBITMQ_PREFETCH", workerConcurrency)
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

	outboxPollMS, err := getEnvInt("OUTBOX_POLL_INTERVAL_MS", defaultOutboxPollMS)
	if err != nil {
		return Config{}, err
	}

	outboxBatchSize, err := getEnvInt("OUTBOX_BATCH_SIZE", defaultOutboxBatchSize)
	if err != nil {
		return Config{}, err
	}

	outboxLockSeconds, err := getEnvInt("OUTBOX_LOCK_SECONDS", defaultOutboxLockSeconds)
	if err != nil {
		return Config{}, err
	}

	outboxMaxAttempts, err := getEnvInt("OUTBOX_MAX_ATTEMPTS", defaultOutboxMaxAttempts)
	if err != nil {
		return Config{}, err
	}

	if requestTimeoutMS <= 0 {
		return Config{}, fmt.Errorf("AI_REQUEST_TIMEOUT_MS must be greater than zero")
	}

	if maxAttempts <= 0 {
		return Config{}, fmt.Errorf("INSIGHT_MAX_ATTEMPTS must be greater than zero")
	}

	if leaseSeconds <= 0 {
		return Config{}, fmt.Errorf("INSIGHT_PROCESSING_LEASE_SECONDS must be greater than zero")
	}

	if workerConcurrency <= 0 {
		return Config{}, fmt.Errorf("WORKER_CONCURRENCY must be greater than zero")
	}

	if rabbitMQPrefetch < workerConcurrency {
		return Config{}, fmt.Errorf("RABBITMQ_PREFETCH must be greater than or equal to WORKER_CONCURRENCY")
	}

	leaseDuration := time.Duration(leaseSeconds) * time.Second
	requestTimeout := time.Duration(requestTimeoutMS) * time.Millisecond
	if leaseDuration <= requestTimeout {
		return Config{}, fmt.Errorf("INSIGHT_PROCESSING_LEASE_SECONDS must be greater than AI_REQUEST_TIMEOUT_MS")
	}

	if retryPollMS <= 0 {
		return Config{}, fmt.Errorf("INSIGHT_RETRY_POLL_INTERVAL_MS must be greater than zero")
	}

	if retryBatchSize <= 0 {
		return Config{}, fmt.Errorf("INSIGHT_RETRY_BATCH_SIZE must be greater than zero")
	}

	if outboxPollMS <= 0 {
		return Config{}, fmt.Errorf("OUTBOX_POLL_INTERVAL_MS must be greater than zero")
	}

	if outboxBatchSize <= 0 {
		return Config{}, fmt.Errorf("OUTBOX_BATCH_SIZE must be greater than zero")
	}

	if outboxLockSeconds <= 0 {
		return Config{}, fmt.Errorf("OUTBOX_LOCK_SECONDS must be greater than zero")
	}

	if outboxMaxAttempts <= 0 {
		return Config{}, fmt.Errorf("OUTBOX_MAX_ATTEMPTS must be greater than zero")
	}

	if appEnv == "production" {
		for _, key := range []string{"DATABASE_URL", "RABBITMQ_URL", "AI_PROVIDER_API_KEY", "AI_PROVIDER_BASE_URL"} {
			if strings.TrimSpace(os.Getenv(key)) == "" {
				return Config{}, fmt.Errorf("%s nao pode estar vazia em producao", key)
			}
		}
	}

	return Config{
		AppEnv:                appEnv,
		DatabaseURL:           getEnvString("DATABASE_URL", defaultDatabaseURL),
		InsightRetryBatchSize: retryBatchSize,
		InsightRetryPoll:      time.Duration(retryPollMS) * time.Millisecond,
		OutboxBatchSize:       outboxBatchSize,
		OutboxPoll:            time.Duration(outboxPollMS) * time.Millisecond,
		OutboxLock:            time.Duration(outboxLockSeconds) * time.Second,
		OutboxMaxAttempts:     outboxMaxAttempts,
		RabbitMQURL:           getEnvString("RABBITMQ_URL", defaultRabbitMQURL),
		RabbitMQInsightsDLQ:   getEnvString("RABBITMQ_INSIGHTS_DLQ", defaultRabbitMQInsightsDLQ),
		RabbitMQInsightsQueue: getEnvString("RABBITMQ_INSIGHTS_QUEUE", defaultRabbitMQInsightsQueue),
		RabbitMQPrefetch:      rabbitMQPrefetch,
		WorkerConcurrency:     workerConcurrency,
		AIProvider:            getEnvString("AI_PROVIDER", defaultAIProvider),
		AIProviderAPIKey:      strings.TrimSpace(os.Getenv("AI_PROVIDER_API_KEY")),
		AIProviderBaseURL:     strings.TrimSpace(os.Getenv("AI_PROVIDER_BASE_URL")),
		AIModel:               getEnvString("AI_MODEL", defaultAIModel),
		AIRequestTimeout:      requestTimeout,
		InsightLease:          leaseDuration,
		InsightMaxAttempts:    maxAttempts,
	}, nil
}

func (c Config) LogAttrs() []any {
	return []any{
		slog.String("app_env", c.AppEnv),
		slog.String("database_url", maskURL(c.DatabaseURL)),
		slog.Int("insight_retry_batch_size", c.InsightRetryBatchSize),
		slog.Duration("insight_retry_poll", c.InsightRetryPoll),
		slog.Int("outbox_batch_size", c.OutboxBatchSize),
		slog.Duration("outbox_poll", c.OutboxPoll),
		slog.Duration("outbox_lock", c.OutboxLock),
		slog.Int("outbox_max_attempts", c.OutboxMaxAttempts),
		slog.String("rabbitmq_url", maskURL(c.RabbitMQURL)),
		slog.String("rabbitmq_insights_dlq", c.RabbitMQInsightsDLQ),
		slog.String("rabbitmq_insights_queue", c.RabbitMQInsightsQueue),
		slog.Int("rabbitmq_prefetch", c.RabbitMQPrefetch),
		slog.Int("worker_concurrency", c.WorkerConcurrency),
		slog.String("ai_provider", c.AIProvider),
		slog.String("ai_model", c.AIModel),
		slog.Duration("ai_request_timeout", c.AIRequestTimeout),
		slog.Duration("insight_lease", c.InsightLease),
		slog.Int("insight_max_attempts", c.InsightMaxAttempts),
		slog.Bool("ai_provider_api_key_configured", c.AIProviderAPIKey != ""),
		slog.Bool("ai_provider_base_url_configured", c.AIProviderBaseURL != ""),
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
