# Mentor.ia AI Worker

Worker Go responsável por processar tarefas assíncronas de IA do Mentor.ia.

Neste primeiro PR, o worker ainda não consome RabbitMQ, não conecta no PostgreSQL e não chama provedor de IA. A fundação inicial apenas organiza o entrypoint, configuração e logger para as próximas features.

## Estrutura

```text
cmd/insights-worker/
  main.go
internal/
  config/
    config.go
  platform/
    logger/
      logger.go
```

## Responsabilidades

- `cmd/insights-worker`: ponto de entrada do processo.
- `internal/config`: leitura e validação de variáveis de ambiente.
- `internal/platform/logger`: configuração de logs estruturados.

## Setup Local

```bash
cp .env.example .env
go mod tidy
go run ./cmd/insights-worker
```

## Testes

```bash
go test ./...
```

## Segurança

A variável `AI_PROVIDER_API_KEY` deve existir apenas no ambiente local ou no provedor de deploy. Ela não deve ser commitada e nunca deve aparecer em logs.
