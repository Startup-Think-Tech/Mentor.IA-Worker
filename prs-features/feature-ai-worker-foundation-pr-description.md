# feature: add Go AI worker foundation

## Resumo

Cria a fundação do worker Go de IA do Mentor.ia, preparando a base para evoluir o processamento assíncrono por PRs pequenos e seguros.

Este PR não implementa RabbitMQ, PostgreSQL ou chamada real de IA. Ele apenas organiza o entrypoint, configuração, logger, documentação e validações iniciais.

## O Que Foi Feito

- Move o entrypoint do worker para `cmd/insights-worker/main.go`.
- Remove o servidor HTTP experimental da raiz.
- Adiciona carregamento centralizado de configuração em `internal/config`.
- Adiciona logger estruturado em JSON usando `log/slog`.
- Configura OpenRouter como provedor padrão.
- Define `openrouter/free` como modelo padrão gratuito.
- Adiciona `.env.example` com as variáveis planejadas do worker.
- Adiciona README com setup local, estrutura e cuidados com segredo.
- Adiciona testes para defaults, validação e proteção contra vazamento de segredos em logs.
- Mantém `AI_PROVIDER_API_KEY` fora dos logs.

## Fora Do Escopo

- Consumir mensagens do RabbitMQ.
- Conectar no PostgreSQL.
- Chamar o OpenRouter.
- Implementar retentativas.
- Adicionar Docker.
- Alterar API NestJS ou schema Prisma.

## Como Validar

```bash
go test ./cmd/insights-worker ./internal/...
go run ./cmd/insights-worker
```

## Observações

- O arquivo `.env` local é ignorado pelo Git.
- A chave real da IA deve ficar apenas no ambiente local ou no provedor de deploy.
- O próximo PR planejado é `feature: add RabbitMQ consumer to Go AI worker`.
