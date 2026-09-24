# Mentor.ia AI Worker

Worker Go responsável por processar tarefas assíncronas de IA do Mentor.ia.

Ele consome jobs de insights via RabbitMQ, usa PostgreSQL como fonte de verdade, chama o provedor de IA configurado e persiste o insight gerado.

## Estrutura

```text
cmd/insights-worker/
  main.go
internal/
  ai/openrouter/
    client.go
  config/
    config.go
  insights/
    domain/
    outbox/
    postgres/
    prompt/
    retry/
    service/
    worker/
  platform/
    logger/
    postgres/
    rabbitmq/
  testsupport/
```

## Responsabilidades

- `cmd/insights-worker`: composition root do processo.
- `internal/ai/openrouter`: cliente HTTP para Chat Completions e classificação de erros retryáveis.
- `internal/config`: leitura e validação de variáveis de ambiente.
- `internal/insights/domain`: tipos compartilhados do fluxo de insights.
- `internal/insights/service`: caso de uso de geração e persistência do insight.
- `internal/insights/worker`: consumer RabbitMQ, worker pool limitado e ACK/NACK/DLQ.
- `internal/insights/postgres`: transações PostgreSQL, transactional outbox e query tipada gerada por `sqlc`.
- `internal/insights/retry`: scheduler que agenda retries vencidos.
- `internal/insights/outbox`: dispatcher que publica eventos pendentes da outbox.
- `internal/insights/prompt`: montagem do prompt educacional.
- `internal/platform/rabbitmq`: conexão compartilhada com canais separados para consumo e publicação.
- `internal/testsupport`: helpers reutilizáveis para testes.

## Makefile

Use `make help` para listar os comandos disponíveis.

Comandos principais:

```bash
make setup
make fmt
make vet
make test
make test-v
make test-race
make test-integration
make sqlc-generate
make test-cover
make coverage-html
make check
make build
make run
make clean
```

Comandos Docker:

```bash
make docker-build
make docker-up
make docker-up-d
make docker-down
make docker-down-volumes
make docker-logs
make docker-ps
make docker-restart
```

Não há alvo para `docker compose config` porque esse comando imprime variáveis interpoladas e pode expor secrets no terminal.

## Mensagem Esperada

O worker espera receber mensagens JSON na fila configurada por `RABBITMQ_INSIGHTS_QUEUE`:

```json
{
  "job_id": "uuid-do-job",
  "aluno_id": "uuid-do-aluno"
}
```

Mensagens válidas são processadas no PostgreSQL e recebem `Ack`. Mensagens inválidas são publicadas na DLQ e confirmadas com `Ack`. Falhas persistidas no banco como retry ou falha definitiva também recebem `Ack`. Falhas inesperadas de infraestrutura recebem `Nack` com requeue.

O consumer usa worker pool limitado por `WORKER_CONCURRENCY`. `RABBITMQ_PREFETCH` deve ser maior ou igual à concorrência para manter backpressure coerente.

## Processamento

Ao receber uma mensagem válida, o worker:

- Faz claim atômico do `InsightJob` com `UPDATE ... RETURNING`.
- Marca o job como `processando`, incrementa `tentativas`, gera `processing_token` e define lease temporário.
- Busca até três disciplinas de menor desempenho.
- Monta um prompt educacional curto.
- Chama o provedor de IA com `AI_MODEL`.
- Salva o conteúdo gerado em `insights`.
- Associa as disciplinas usadas em `insights_disciplinas`.
- Marca o job como `concluido` validando `processing_token` e limpa o lease.

O `processing_token` protege contra stale workers. Se um worker perder o lease e outro worker readquirir o job, o worker antigo não consegue salvar resultado nem registrar falha usando o token anterior.

Erros de rede, timeout, `429` e `5xx` do provedor de IA geram retry. Erros permanentes, como `400`, `401` e `403`, falham definitivamente sem gastar tentativas adicionais.

Se a geração ou persistência falhar de forma retryável, o job é atualizado para `aguardando_retentativa` enquanto houver tentativas disponíveis. Ao atingir `INSIGHT_MAX_ATTEMPTS`, o job é marcado como `falhou` e enviado para `RABBITMQ_INSIGHTS_DLQ`.

## Retry, Outbox E DLQ

O retry usa transactional outbox para evitar dual-write entre PostgreSQL e RabbitMQ.

Fluxo atual:

- O scheduler busca jobs com `status = aguardando_retentativa` e `proxima_tentativa_em <= NOW()`.
- Na mesma transação, marca o job como `pendente` e cria um evento em `outbox_eventos`.
- O dispatcher faz claim curto de eventos `pending`, usando token e lock temporário, e confirma a transação antes de publicar.
- Fora da transação PostgreSQL, ele publica no RabbitMQ com `mandatory=true`, publisher confirms e tratamento de retornos não roteáveis.
- Após confirmação, marca o evento como `published`; em falhas, agenda novo retry com backoff. Eventos que excedem `OUTBOX_MAX_ATTEMPTS` ficam em `failed` e não bloqueiam os demais.

O RabbitMQ usa uma conexão compartilhada com dois channels: um para `Consume`/`Ack`/`Nack` e outro para publicação/DLQ/publisher confirms. A política é fail-fast: queda inesperada da conexão encerra o processo com erro para reinício pelo orquestrador.

A DLQ recebe:

- Payloads inválidos que não seguem o contrato `{ job_id, aluno_id }`.
- Jobs que atingem falha definitiva após `INSIGHT_MAX_ATTEMPTS`.

## Variáveis De Ambiente

```env
APP_ENV="development"
DATABASE_URL="postgresql://mentor_ia:mentor_ia@localhost:5432/mentor_ia?schema=public"
RABBITMQ_URL="amqp://mentor_ia:mentor_ia@localhost:5672"
RABBITMQ_INSIGHTS_QUEUE="insights_queue"
RABBITMQ_INSIGHTS_DLQ="insights_dlq"
WORKER_CONCURRENCY=1
RABBITMQ_PREFETCH=1
AI_PROVIDER="openrouter"
AI_PROVIDER_API_KEY=""
AI_PROVIDER_BASE_URL=""
AI_MODEL="openrouter/free"
AI_REQUEST_TIMEOUT_MS=60000
INSIGHT_PROCESSING_LEASE_SECONDS=120
INSIGHT_MAX_ATTEMPTS=3
INSIGHT_RETRY_POLL_INTERVAL_MS=30000
INSIGHT_RETRY_BATCH_SIZE=10
OUTBOX_POLL_INTERVAL_MS=5000
OUTBOX_BATCH_SIZE=50
OUTBOX_LOCK_SECONDS=30
OUTBOX_MAX_ATTEMPTS=5
```

`AI_PROVIDER_BASE_URL` é obrigatório em runtime e não possui fallback hardcoded no binário.

`INSIGHT_PROCESSING_LEASE_SECONDS` precisa ser maior que `AI_REQUEST_TIMEOUT_MS`, para evitar que uma chamada de IA válida perca o lease antes de terminar.

Em `APP_ENV=production`, `DATABASE_URL`, `RABBITMQ_URL`, `AI_PROVIDER_API_KEY` e `AI_PROVIDER_BASE_URL` são obrigatórias. Não há fallback de desenvolvimento para essas variáveis.

## Setup Local

```bash
cp .env.example .env
make setup
make run
```

Para o worker iniciar completamente, PostgreSQL e RabbitMQ precisam estar acessíveis por `DATABASE_URL` e `RABBITMQ_URL`.

O provedor de IA também precisa estar configurado com `AI_PROVIDER_API_KEY`, `AI_PROVIDER_BASE_URL` e `AI_MODEL`. O worker falha no bootstrap se essas configurações obrigatórias estiverem ausentes.

## Docker

Subir em foreground:

```bash
make docker-up
```

Subir em background:

```bash
make docker-up-d
```

Parar:

```bash
make docker-down
```

Remover volumes/orphans:

```bash
make docker-down-volumes
```

## Testes

Rodar a suíte principal:

```bash
make test
```

Rodar com race detector:

```bash
make test-race
```

Gerar cobertura:

```bash
make test-cover
make coverage-html
```

Rodar integrações sem `.env`, usando containers efêmeros de PostgreSQL e RabbitMQ:

```bash
make test-integration
```

O GitHub Actions executa build, `go vet`, testes, race detector e a suíte Testcontainers em pushes e pull requests.

Regenerar a query tipada do `sqlc`:

```bash
make sqlc-generate
```

Testes específicos com integrações reais usando `.env`:

```bash
go test ./internal/platform/postgres -run TestConnectWithEnv -v
go test ./internal/platform/rabbitmq -run TestConnectWithEnv -v
go test ./internal/insights/postgres -run TestRepositoryProcessesRealInsightResult -v
go test ./internal/insights/postgres -run TestRepositoryScheduleDueRetryJobs -v
```

## Segurança

`AI_PROVIDER_API_KEY`, `AI_PROVIDER_BASE_URL`, `DATABASE_URL` e `RABBITMQ_URL` devem existir apenas no ambiente local ou no provedor de deploy. Não commite `.env` e não compartilhe saídas de comandos que interpolam variáveis sensíveis.

Logs do worker mascaram credenciais de URLs e não imprimem API key nem base URL do provedor de IA.
