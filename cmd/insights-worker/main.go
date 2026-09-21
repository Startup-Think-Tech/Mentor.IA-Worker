package main

import (
	"os"

	"github.com/daviPeter07/ai-worker/internal/config"
	platformlogger "github.com/daviPeter07/ai-worker/internal/platform/logger"
)

func main() {
	//carrega as configurações do pacote que le as envs
	cfg, err := config.Load()
	if err != nil {
		logger := platformlogger.New("development")
		logger.Error("falha ao carregar configuracao", "erro", err)
		os.Exit(1)
	}

	logger := platformlogger.New(cfg.NodeEnv)
	logger.Info("worker de insights iniciando", cfg.LogAttrs()...)
	logger.Info("nenhum consumidor configurado ainda; bootstrap da fundacao concluido")
	logger.Info("worker de insights finalizado")
}
