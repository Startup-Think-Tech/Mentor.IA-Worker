# feature: estrutura base do worker Go de IA

## Objetivo

Criar a fundação do worker Go de IA de forma incremental, organizada e escalável, sem implementar ainda o consumo do RabbitMQ, acesso ao PostgreSQL ou chamada real ao provedor de IA.

Este PR deve preparar a arquitetura para que as próximas features sejam adicionadas em PRs menores e seguros.

## Escopo Deste PR

- Organizar a estrutura inicial de pastas do worker Go.
- Criar um entrypoint claro em `cmd/insights-worker/main.go`.
- Criar carregamento centralizado de configuração via variáveis de ambiente.
- Criar logger básico e estruturado usando biblioteca padrão do Go.
- Criar `.env.example` com as variáveis necessárias para evolução do worker.
- Atualizar `README.md` do worker explicando como rodar localmente.
- Garantir que o worker inicialize, leia configurações e finalize corretamente.

## Fora Do Escopo

- Não consumir mensagens do RabbitMQ neste PR.
- Não conectar no PostgreSQL neste PR.
- Não chamar API de IA neste PR.
- Não implementar retentativas neste PR.
- Não alterar a API NestJS neste PR.
- Não alterar schema Prisma neste PR.
- Não adicionar Docker neste PR.

## Estrutura Esperada

```text
ai-worker/
  cmd/
    insights-worker/
      main.go
  internal/
    config/
      config.go
    platform/
      logger/
        logger.go
  .env.example
  README.md
  go.mod
  go.sum
```

## Responsabilidades Das Pastas

`cmd/insights-worker/`

Ponto de entrada do worker. Deve apenas carregar dependências, montar a aplicação e iniciar/parar o processo.

`internal/config/`

Responsável por ler e validar variáveis de ambiente do worker.

`internal/platform/logger/`

Responsável por configurar logs estruturados para serem reutilizados pelas próximas features.

## Variáveis De Ambiente Planejadas

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

Neste PR, as variáveis podem existir no `.env.example`, mas somente as necessárias para o bootstrap básico precisam ser lidas de fato.

## Critérios De Aceite

- `go test ./...` executa sem falhas.
- `go run ./cmd/insights-worker` inicia sem panic.
- O worker registra log de inicialização.
- O worker registra quais configurações não sensíveis foram carregadas.
- A chave `AI_PROVIDER_API_KEY` nunca é impressa em log.
- A estrutura final não contém regra de negócio no `main.go`.

## Prefixo Semântico

Nome sugerido do branch:

```text
feature/ai-worker-foundation
```

Título sugerido do PR:

```text
feature: add Go AI worker foundation
```

Commit sugerido:

```text
feature: add Go AI worker foundation
```

## Próximo PR Planejado

```text
feature: add RabbitMQ consumer to Go AI worker
```

Esse próximo PR deve adicionar conexão com RabbitMQ, declaração da fila `insights_queue`, consumo com ack manual e logs das mensagens recebidas, ainda sem acessar banco ou IA.
