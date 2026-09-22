# feature: add RabbitMQ foundation to Go AI worker

## Resumo

Adiciona a fundação de RabbitMQ no worker Go de IA, permitindo conectar no broker, declarar a fila de insights, configurar prefetch e iniciar um consumer com confirmação manual de mensagens.

Este PR ainda não conecta no PostgreSQL e não chama o OpenRouter. O objetivo é validar a ponte assíncrona entre fila e worker antes de processar jobs reais.

## O Que Foi Feito

- Adiciona a dependência `github.com/rabbitmq/amqp091-go`.
- Cria `internal/platform/rabbitmq` para encapsular conexão, canal, fila, prefetch e consumo técnico.
- Conecta no RabbitMQ usando `RABBITMQ_URL`.
- Declara a fila configurada em `RABBITMQ_INSIGHTS_QUEUE` como durável.
- Configura `prefetch = 1` para processar uma mensagem por vez no início.
- Expõe consumo com `autoAck=false`.
- Cria `internal/insights/message.go` para validar payloads `{ job_id, aluno_id }`.
- Cria `internal/insights/consumer.go` para receber mensagens, logar dados básicos e aplicar `Ack`/`Nack`.
- Cria `internal/testsupport/env.go` para carregar o `.env` local em testes de integração sem duplicar lógica.
- Adiciona teste de conexão real com RabbitMQ usando `RABBITMQ_URL` e `RABBITMQ_INSIGHTS_QUEUE`.
- Adiciona graceful shutdown com `Ctrl+C`/SIGTERM no entrypoint.
- Atualiza o README com estrutura, envs, formato da mensagem e comando de teste.

## Comportamento Do Consumer

- Mensagem válida: faz parse, loga `job_id` e `aluno_id`, depois executa `Ack(false)`.
- Mensagem inválida: loga erro e executa `Nack(false, false)` para rejeitar sem requeue.
- O worker permanece aguardando mensagens até receber sinal de parada.

## Fora Do Escopo

- Buscar `InsightJob` no PostgreSQL.
- Alterar status do job.
- Chamar OpenRouter ou qualquer provedor de IA.
- Implementar retentativas reais.
- Criar DLQ.
- Publicar mensagens a partir da API NestJS.
- Adicionar Docker/Compose.

## Como Validar

```bash
go test ./cmd/insights-worker ./internal/config ./internal/insights ./internal/platform/logger ./internal/platform/rabbitmq ./internal/testsupport
```

Teste de conexão real com RabbitMQ usando `.env`:

```bash
go test ./internal/platform/rabbitmq -run TestConnectWithEnv -v
```

Com RabbitMQ rodando, para iniciar o worker:

```bash
go run ./cmd/insights-worker
```

## Observações

- `go test ./internal/...` pode falhar se existirem pacotes experimentais locais fora do escopo.
- `cmd/teste-get/` não faz parte deste PR.
- O próximo PR recomendado é `feature: add PostgreSQL connection to Go AI worker`.
