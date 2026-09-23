# feature: add insight retry scheduler and DLQ

## Resumo

Melhora o prompt de geração de insights e adiciona scheduler de retry com DLQ no worker Go.

## O Que Foi Feito

- Renomeia o escopo da branch para retry/DLQ.
- Melhora o prompt enviado ao OpenRouter com regras claras de tom, limites e uso dos dados.
- Adiciona testes unitários para o prompt.
- Adiciona `RABBITMQ_INSIGHTS_DLQ` com padrão `insights_dlq`.
- Adiciona `INSIGHT_RETRY_POLL_INTERVAL_MS` e `INSIGHT_RETRY_BATCH_SIZE`.
- Declara a fila principal e a DLQ no bootstrap RabbitMQ.
- Adiciona publicação persistente na fila principal para retries.
- Adiciona publicação persistente na DLQ.
- Cria scheduler interno para buscar jobs `aguardando_retentativa` vencidos e republicar mensagens.
- Mensagens inválidas são enviadas para DLQ e confirmadas.
- Jobs com falha definitiva são enviados para DLQ e confirmados.

## Fora Do Escopo

- Integração com API NestJS.
- Endpoints HTTP no worker.
- Endpoints HTTP na API NestJS.
- DLX automático via argumentos RabbitMQ na fila existente, para evitar conflito com filas já declaradas sem argumentos.

## Como Validar

```bash
go test ./cmd/insights-worker ./internal/config ./internal/insights ./internal/platform/logger ./internal/platform/rabbitmq ./internal/platform/postgres ./internal/testsupport ./internal/ai/openrouter
```

Testes de integração com `.env`:

```bash
go test ./internal/platform/rabbitmq -run TestConnectWithEnv -v
go test ./internal/insights -run TestRepositoryClaimDueRetryJobs -v
```
