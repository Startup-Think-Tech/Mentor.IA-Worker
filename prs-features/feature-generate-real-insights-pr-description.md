# feature: generate real insights and handle retries

## Resumo

Substitui o processamento fake por geração real de insights com OpenRouter e adiciona política inicial de retry/falha persistida no PostgreSQL.

## O Que Foi Feito

- Integra o `internal/ai/openrouter` ao service de insights.
- Faz o worker falhar no bootstrap se `AI_PROVIDER_API_KEY` estiver ausente.
- Busca até três disciplinas de menor desempenho do aluno usando `registros_desempenho`.
- Monta prompt educacional em português com foco ENEM.
- Chama OpenRouter usando `AI_MODEL`, hoje `openrouter/free`.
- Salva o conteúdo gerado em `insights`.
- Associa as disciplinas usadas em `insights_disciplinas`.
- Marca jobs bem-sucedidos como `concluido`.
- Registra falhas como `aguardando_retentativa` enquanto houver tentativas disponíveis.
- Marca jobs como `falhou` ao atingir `INSIGHT_MAX_ATTEMPTS`.
- Atualiza o consumer para dar `Ack` quando retry/falha já foi persistido, evitando loop imediato de requeue.

## Comportamento De Falha

- Payload inválido: `Nack(false, false)`.
- Erro persistido como retry/falha: `Ack(false)`.
- Erro inesperado antes de persistir status: `Nack(false, true)`.

## Fora Do Escopo

- Criar scheduler para republicar jobs em `aguardando_retentativa` quando `proxima_tentativa_em` chegar.
- Criar DLQ no RabbitMQ.
- Publicar mensagens pela API NestJS.
- Refinar prompt com dados de conteúdo, cronograma ou revisões.

## Como Validar

```bash
go test ./cmd/insights-worker ./internal/config ./internal/insights ./internal/platform/logger ./internal/platform/rabbitmq ./internal/platform/postgres ./internal/testsupport ./internal/ai/openrouter
```

Testes de integração com `.env`:

```bash
go test ./internal/platform/postgres -run TestConnectWithEnv -v
go test ./internal/platform/rabbitmq -run TestConnectWithEnv -v
go test ./internal/insights -run TestRepositoryProcessesRealInsightResult -v
```

Bootstrap real:

```bash
go run ./cmd/insights-worker
```

Observação: `AI_PROVIDER_API_KEY` precisa estar configurada para o bootstrap passar.
