package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/daviPeter07/ai-worker/internal/insights/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) BeginProcessing(ctx context.Context, message domain.Message) (bool, error) {
	var claimedID string
	err := r.pool.QueryRow(ctx, `
		UPDATE insight_jobs
		SET status = 'processando',
		    tentativas = tentativas + 1,
		    lease_expira_em = NOW() + INTERVAL '5 minutes',
		    proxima_tentativa_em = NULL,
		    erro_codigo = NULL,
		    erro_resumo = NULL,
		    atualizado_em = NOW()
		WHERE id = $1
		  AND aluno_id = $2
		  AND (
		    status = 'pendente'
		    OR (status = 'aguardando_retentativa' AND COALESCE(proxima_tentativa_em <= NOW(), true))
		    OR (status = 'processando' AND lease_expira_em <= NOW())
		  )
		RETURNING id::text
	`, message.JobID, message.AlunoID).Scan(&claimedID)
	if err == nil {
		return false, nil
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if existsErr := r.pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM insight_jobs
					WHERE id = $1 AND aluno_id = $2
				)
			`, message.JobID, message.AlunoID).Scan(&exists); existsErr != nil {
				return false, fmt.Errorf("falha ao verificar job nao adquirido: %w", existsErr)
			}

			if !exists {
				return false, fmt.Errorf("job de insight nao encontrado: %s", message.JobID)
			}

			return true, nil
		}

		return false, fmt.Errorf("falha ao adquirir job de insight: %w", err)
	}

	return false, nil
}

func (r *Repository) FindLowestPerformanceDisciplines(ctx context.Context, alunoID string, limit int) ([]domain.DisciplinePerformance, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id::text, d.nome, AVG(rd.percentual)::float8 AS percentual_medio
		FROM registros_desempenho rd
		JOIN disciplinas d ON d.id = rd.disciplina_id
		WHERE rd.aluno_id = $1 AND d.ativo = true
		GROUP BY d.id, d.nome
		ORDER BY percentual_medio ASC, d.nome ASC
		LIMIT $2
	`, alunoID, limit)
	if err != nil {
		return nil, fmt.Errorf("falha ao buscar disciplinas de menor desempenho: %w", err)
	}
	defer rows.Close()

	disciplines := make([]domain.DisciplinePerformance, 0, limit)
	for rows.Next() {
		var discipline domain.DisciplinePerformance
		if err := rows.Scan(&discipline.ID, &discipline.Nome, &discipline.Percentual); err != nil {
			return nil, fmt.Errorf("falha ao ler disciplina de menor desempenho: %w", err)
		}

		disciplines = append(disciplines, discipline)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("falha ao iterar disciplinas de menor desempenho: %w", err)
	}

	return disciplines, nil
}

func (r *Repository) SaveInsightResult(ctx context.Context, message domain.Message, content string, disciplines []domain.DisciplinePerformance) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("falha ao iniciar transacao para salvar insight: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var insightID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO insights (id, aluno_id, job_id, conteudo)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (job_id) DO UPDATE
		SET conteudo = EXCLUDED.conteudo
		RETURNING id::text
	`, uuid.NewString(), message.AlunoID, message.JobID, content).Scan(&insightID); err != nil {
		return fmt.Errorf("falha ao salvar insight: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM insights_disciplinas WHERE insight_id = $1`, insightID); err != nil {
		return fmt.Errorf("falha ao limpar disciplinas do insight: %w", err)
	}

	for _, discipline := range disciplines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO insights_disciplinas (id, insight_id, disciplina_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (insight_id, disciplina_id) DO NOTHING
		`, uuid.NewString(), insightID, discipline.ID); err != nil {
			return fmt.Errorf("falha ao associar disciplina ao insight: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE insight_jobs
		SET status = 'concluido',
		    concluido_em = NOW(),
		    atualizado_em = NOW(),
		    lease_expira_em = NULL,
		    proxima_tentativa_em = NULL,
		    erro_codigo = NULL,
		    erro_resumo = NULL
		WHERE id = $1 AND aluno_id = $2
	`, message.JobID, message.AlunoID); err != nil {
		return fmt.Errorf("falha ao marcar job como concluido: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("falha ao confirmar insight: %w", err)
	}

	return nil
}

func (r *Repository) RegisterFailure(ctx context.Context, message domain.Message, maxAttempts int, code string, failure error) (domain.FailureAction, error) {
	summary := strings.TrimSpace(failure.Error())
	if len(summary) > 500 {
		summary = summary[:500]
	}

	var status string
	err := r.pool.QueryRow(ctx, `
		UPDATE insight_jobs
		SET status = CASE
		      WHEN tentativas >= $3 THEN 'falhou'::"InsightJobStatus"
		      ELSE 'aguardando_retentativa'::"InsightJobStatus"
		    END,
		    proxima_tentativa_em = CASE
		      WHEN tentativas >= $3 THEN NULL
		      ELSE NOW() + (LEAST(300, GREATEST(5, tentativas * 15)) * INTERVAL '1 second')
		    END,
		    erro_codigo = $4,
		    erro_resumo = $5,
		    lease_expira_em = NULL,
		    atualizado_em = NOW()
		WHERE id = $1 AND aluno_id = $2
		RETURNING status::text
	`, message.JobID, message.AlunoID, maxAttempts, code, summary).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("falha ao registrar erro do job de insight: %w", err)
	}

	if status == "falhou" {
		return domain.FailureActionFailed, nil
	}

	return domain.FailureActionRetry, nil
}

func (r *Repository) ScheduleDueRetryJobs(ctx context.Context, limit int) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("falha ao iniciar transacao de retry: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
		SELECT id::text, aluno_id::text
		FROM insight_jobs
		WHERE status = 'aguardando_retentativa'
		  AND proxima_tentativa_em <= NOW()
		ORDER BY proxima_tentativa_em ASC, atualizado_em ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("falha ao buscar jobs para retry: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.Message, 0, limit)
	for rows.Next() {
		var message domain.Message
		if err := rows.Scan(&message.JobID, &message.AlunoID); err != nil {
			return 0, fmt.Errorf("falha ao ler job para retry: %w", err)
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("falha ao iterar jobs para retry: %w", err)
	}
	rows.Close()

	for _, message := range messages {
		payload, err := json.Marshal(message)
		if err != nil {
			return 0, fmt.Errorf("falha ao serializar evento de retry: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE insight_jobs
			SET status = 'pendente',
			    proxima_tentativa_em = NULL,
			    atualizado_em = NOW()
			WHERE id = $1 AND aluno_id = $2
		`, message.JobID, message.AlunoID); err != nil {
			return 0, fmt.Errorf("falha ao marcar job para retry: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_eventos (id, job_id, tipo, payload)
			VALUES ($1, $2, $3, $4)
		`, uuid.NewString(), message.JobID, domain.OutboxTypeInsightRetryRequested, payload); err != nil {
			return 0, fmt.Errorf("falha ao criar evento de outbox para retry: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("falha ao confirmar jobs de retry: %w", err)
	}

	return len(messages), nil
}

func (r *Repository) DispatchPendingOutboxEvents(ctx context.Context, limit int, publish domain.OutboxPublisher) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("falha ao iniciar transacao de outbox: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
		SELECT id::text, tipo, payload
		FROM outbox_eventos
		WHERE publicado_em IS NULL
		ORDER BY criado_em ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("falha ao buscar eventos de outbox: %w", err)
	}
	defer rows.Close()

	events := make([]domain.OutboxEvent, 0, limit)
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.Payload); err != nil {
			return 0, fmt.Errorf("falha ao ler evento de outbox: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("falha ao iterar eventos de outbox: %w", err)
	}

	for _, event := range events {
		if err := publish(ctx, event); err != nil {
			return 0, fmt.Errorf("falha ao publicar evento de outbox %s: %w", event.ID, err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE outbox_eventos
			SET publicado_em = NOW()
			WHERE id = $1
		`, event.ID); err != nil {
			return 0, fmt.Errorf("falha ao marcar evento de outbox como publicado: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("falha ao confirmar eventos de outbox: %w", err)
	}

	return len(events), nil
}
