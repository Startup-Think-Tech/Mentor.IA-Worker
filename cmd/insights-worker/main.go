package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/ai/openrouter"
	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/config"
	insightoutbox "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/outbox"
	insightpostgres "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/postgres"
	insightretry "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/retry"
	insightservice "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/service"
	insightworker "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/worker"
	platformlogger "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/platform/logger"
	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/platform/postgres"
	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/platform/rabbitmq"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		logger := platformlogger.New("development")
		logger.Error("falha ao carregar configuracao", "erro", err)
		return err
	}

	logger := platformlogger.New(cfg.AppEnv)
	logger.Info("worker de insights iniciando", cfg.LogAttrs()...)

	if cfg.AIProvider != "openrouter" {
		logger.Error("provedor de IA nao suportado", "provider", cfg.AIProvider)
		return fmt.Errorf("provedor de IA nao suportado: %s", cfg.AIProvider)
	}

	aiClient, err := openrouter.New(openrouter.Config{
		APIKey:  cfg.AIProviderAPIKey,
		BaseURL: cfg.AIProviderBaseURL,
		Model:   cfg.AIModel,
		Timeout: cfg.AIRequestTimeout,
	})
	if err != nil {
		logger.Error("falha ao preparar OpenRouter", "erro", err)
		return err
	}

	logger.Info("OpenRouter preparado com sucesso")

	postgresClient, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("falha ao preparar PostgreSQL", "erro", err)
		return err
	}
	defer postgresClient.Close()

	logger.Info("PostgreSQL preparado com sucesso")

	rabbitClient, err := rabbitmq.Connect(rabbitmq.Config{
		URL:            cfg.RabbitMQURL,
		Queue:          cfg.RabbitMQInsightsQueue,
		DLQ:            cfg.RabbitMQInsightsDLQ,
		Prefetch:       cfg.RabbitMQPrefetch,
		PublishTimeout: cfg.RabbitMQPublishTimeout,
	})
	if err != nil {
		logger.Error("falha ao preparar RabbitMQ", "erro", err)
		return err
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
		return err
	}

	logger.Info("worker aguardando mensagens de insights")
	insightsRepository := insightpostgres.NewRepository(postgresClient.Pool(), cfg.InsightLease)
	insightsService := insightservice.New(insightsRepository, aiClient, cfg.InsightMaxAttempts)
	retryScheduler := insightretry.NewScheduler(logger, insightsRepository, cfg.InsightRetryPoll, cfg.InsightRetryBatchSize)
	outboxDispatcher := insightoutbox.NewDispatcher(logger, insightsRepository, rabbitClient, cfg.OutboxPoll, cfg.OutboxBatchSize, cfg.OutboxLock, cfg.OutboxMaxAttempts)
	insightsConsumer := insightworker.NewConsumer(logger, insightsService, rabbitClient, cfg.WorkerConcurrency)

	components, componentCtx := errgroup.WithContext(ctx)
	components.Go(func() error {
		return insightsConsumer.Run(componentCtx, deliveries)
	})
	components.Go(func() error {
		return retryScheduler.Run(componentCtx)
	})
	components.Go(func() error {
		return outboxDispatcher.Run(componentCtx)
	})
	components.Go(func() error {
		select {
		case <-componentCtx.Done():
			return nil
		case closeErr, ok := <-rabbitClient.Errors():
			if !ok || closeErr == nil {
				return fmt.Errorf("conexao RabbitMQ fechada inesperadamente")
			}

			return fmt.Errorf("conexao RabbitMQ encerrada: %w", closeErr)
		}
	})

	if err := components.Wait(); err != nil {
		logger.Error("worker de insights finalizado com erro", "erro", err)
		return err
	}

	logger.Info("worker de insights finalizado")
	return nil
}
