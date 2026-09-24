package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/daviPeter07/ai-worker/internal/ai/openrouter"
	"github.com/daviPeter07/ai-worker/internal/config"
	insightoutbox "github.com/daviPeter07/ai-worker/internal/insights/outbox"
	insightpostgres "github.com/daviPeter07/ai-worker/internal/insights/postgres"
	insightretry "github.com/daviPeter07/ai-worker/internal/insights/retry"
	insightservice "github.com/daviPeter07/ai-worker/internal/insights/service"
	insightworker "github.com/daviPeter07/ai-worker/internal/insights/worker"
	platformlogger "github.com/daviPeter07/ai-worker/internal/platform/logger"
	"github.com/daviPeter07/ai-worker/internal/platform/postgres"
	"github.com/daviPeter07/ai-worker/internal/platform/rabbitmq"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	//carrega as configurações do pacote que le as envs
	cfg, err := config.Load()
	if err != nil {
		logger := platformlogger.New("development")
		logger.Error("falha ao carregar configuracao", "erro", err)
		os.Exit(1)
	}

	logger := platformlogger.New(cfg.NodeEnv)
	logger.Info("worker de insights iniciando", cfg.LogAttrs()...)

	if cfg.AIProvider != "openrouter" {
		logger.Error("provedor de IA nao suportado", "provider", cfg.AIProvider)
		os.Exit(1)
	}

	aiClient, err := openrouter.New(openrouter.Config{
		APIKey:  cfg.AIProviderAPIKey,
		BaseURL: cfg.AIProviderBaseURL,
		Model:   cfg.AIModel,
		Timeout: cfg.AIRequestTimeout,
	})
	if err != nil {
		logger.Error("falha ao preparar OpenRouter", "erro", err)
		os.Exit(1)
	}

	logger.Info("OpenRouter preparado com sucesso")

	postgresClient, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("falha ao preparar PostgreSQL", "erro", err)
		os.Exit(1)
	}
	defer postgresClient.Close()

	logger.Info("PostgreSQL preparado com sucesso")

	rabbitClient, err := rabbitmq.Connect(rabbitmq.Config{
		URL:   cfg.RabbitMQURL,
		Queue: cfg.RabbitMQInsightsQueue,
		DLQ:   cfg.RabbitMQInsightsDLQ,
	})
	if err != nil {
		logger.Error("falha ao preparar RabbitMQ", "erro", err)
		os.Exit(1)
	}
	defer func() {
		if err := rabbitClient.Close(); err != nil {
			logger.Error("falha ao fechar conexao com RabbitMQ", "erro", err)
		}
	}()

	logger.Info(
		"RabbitMQ preparado com sucesso",
		"fila", rabbitClient.QueueName(),
		"dlq", rabbitClient.DLQName(),
		"prefetch", rabbitClient.PrefetchCount(),
	)

	deliveries, err := rabbitClient.Consume("insights-worker")
	if err != nil {
		logger.Error("falha ao iniciar consumo de mensagens", "erro", err)
		os.Exit(1)
	}

	logger.Info("worker aguardando mensagens de insights")
	insightsRepository := insightpostgres.NewRepository(postgresClient.Pool())
	insightsService := insightservice.New(insightsRepository, aiClient, cfg.InsightMaxAttempts)
	retryScheduler := insightretry.NewScheduler(logger, insightsRepository, cfg.InsightRetryPoll, cfg.InsightRetryBatchSize)
	go retryScheduler.Run(ctx)
	outboxDispatcher := insightoutbox.NewDispatcher(logger, insightsRepository, rabbitClient, cfg.OutboxPoll, cfg.OutboxBatchSize)
	go outboxDispatcher.Run(ctx)

	insightsConsumer := insightworker.NewConsumer(logger, insightsService, rabbitClient)
	insightsConsumer.Run(ctx, deliveries)
	logger.Info("worker de insights finalizado")
}
