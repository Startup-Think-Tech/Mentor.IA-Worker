# Mentor.ia AI Worker

Worker Go responsável por processar tarefas assíncronas de IA do Mentor.ia.

O worker consome mensagens de insights via RabbitMQ, conecta no PostgreSQL e processa jobs de insight com resultado fake. O cliente OpenRouter já existe, mas a geração real de conteúdo com IA fica para o próximo PR.

## Estrutura

```text
cmd/insights-worker/
  main.go
internal/
  ai/
    openrouter/
      client.go
  config/
    config.go
  insights/
    consumer.go
    message.go
    repository.go
    service.go
  platform/
    logger/
      logger.go
    postgres/
      client.go
    rabbitmq/
      client.go
  testsupport/
    env.go
```

## Responsabilidades

- `cmd/insights-worker`: ponto de entrada do processo.
- `internal/ai/openrouter`: cliente HTTP para OpenRouter Chat Completions.
- `internal/config`: leitura e validação de variáveis de ambiente.
- `internal/insights`: validação da mensagem, consumer, service e repository do processamento de insights.
- `internal/platform/logger`: configuração de logs estruturados.
- `internal/platform/postgres`: conexão e pool PostgreSQL com `pgxpool`.
- `internal/platform/rabbitmq`: conexão, canal, fila, prefetch e consumo técnico do RabbitMQ.
- `internal/testsupport`: helpers reutilizáveis para testes, incluindo carregamento do `.env` local.

## Mensagem Esperada

O worker espera receber mensagens JSON na fila configurada por `RABBITMQ_INSIGHTS_QUEUE`:

```json
{
  "job_id": "uuid-do-job",
  "aluno_id": "uuid-do-aluno"
}
```

Mensagens válidas são processadas no PostgreSQL e recebem `Ack`. Mensagens inválidas recebem `Nack` sem requeue, para evitar loop infinito com payload malformado. Falhas de processamento recebem `Nack` com requeue até o PR de retry/DLQ definir a política final.

## Processamento Atual

Ao receber uma mensagem válida, o worker:

- Busca o `InsightJob` por `job_id` e `aluno_id`.
- Marca o job como `processando`.
- Incrementa `tentativas`.
- Salva um registro em `insights` com conteúdo fake.
- Marca o job como `concluido`.

O conteúdo fake atual é temporário. O próximo PR deve substituir isso por geração real com OpenRouter e política de falhas/retry.

## Variáveis De Ambiente

```env
NODE_ENV="development"
DATABASE_URL="postgresql://mentor_ia:mentor_ia@localhost:5432/mentor_ia?schema=public"
RABBITMQ_URL="amqp://mentor_ia:mentor_ia@localhost:5672"
RABBITMQ_INSIGHTS_QUEUE="insights_queue"
AI_PROVIDER="openrouter"
AI_PROVIDER_API_KEY=""
AI_MODEL="openrouter/free"
AI_REQUEST_TIMEOUT_MS=60000
INSIGHT_MAX_ATTEMPTS=3
```

## Setup Local

```bash
cp .env.example .env
go mod tidy
go run ./cmd/insights-worker
```

Para o worker iniciar completamente, o RabbitMQ precisa estar acessível em `RABBITMQ_URL`.

## Testes

```bash
go test ./cmd/insights-worker ./internal/config ./internal/insights ./internal/platform/logger ./internal/platform/rabbitmq
```

Teste com PostgreSQL e RabbitMQ reais usando `.env`:

```bash
go test ./internal/platform/postgres -run TestConnectWithEnv -v
go test ./internal/platform/rabbitmq -run TestConnectWithEnv -v
go test ./internal/insights -run TestRepositoryProcessWithFakeResult -v
```

Para validar conexão real com RabbitMQ usando `RABBITMQ_URL` e `RABBITMQ_INSIGHTS_QUEUE` do `.env`:

```bash
go test ./internal/platform/rabbitmq -run TestConnectWithEnv -v
```

Esse teste conecta no broker, declara a fila configurada, valida `prefetch = 1` e fecha a conexão.

## Segurança

A variável `AI_PROVIDER_API_KEY` deve existir apenas no ambiente local ou no provedor de deploy. Ela não deve ser commitada e nunca deve aparecer em logs.
