package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/daviPeter07/ai-worker/internal/config"
	"github.com/daviPeter07/ai-worker/internal/insights"
	platformlogger "github.com/daviPeter07/ai-worker/internal/platform/logger"
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

	rabbitClient, err := rabbitmq.Connect(rabbitmq.Config{
		URL:       cfg.RabbitMQURL,
		QueueName: cfg.RabbitMQInsightsQueue,
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
		"prefetch", rabbitClient.PrefetchCount(),
	)

	deliveries, err := rabbitClient.Consume("insights-worker")
	if err != nil {
		logger.Error("falha ao iniciar consumo de mensagens", "erro", err)
		os.Exit(1)
	}

	logger.Info("worker aguardando mensagens de insights")
	insightsConsumer := insights.NewConsumer(logger)
	insightsConsumer.Run(ctx, deliveries)
	logger.Info("worker de insights finalizado")
}
