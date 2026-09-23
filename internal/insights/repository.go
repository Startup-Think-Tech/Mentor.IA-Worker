package insights

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DisciplinePerformance struct {
	ID         string
	Nome       string
	Percentual float64
}

type FailureAction string

const (
	FailureActionFailed FailureAction = "failed"
	FailureActionRetry  FailureAction = "retry"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) BeginProcessing(ctx context.Context, message Message) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("falha ao iniciar transacao do job de insight: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var status string
	var shouldWait bool
	err = tx.QueryRow(ctx, `
		SELECT status::text, COALESCE(proxima_tentativa_em > NOW(), false)
		FROM insight_jobs
		WHERE id = $1 AND aluno_id = $2
		FOR UPDATE
	`, message.JobID, message.AlunoID).Scan(&status, &shouldWait)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("job de insight nao encontrado: %s", message.JobID)
		}

		return false, fmt.Errorf("falha ao buscar job de insight: %w", err)
	}

	if status == "concluido" || status == "falhou" || shouldWait {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("falha ao confirmar job sem processamento: %w", err)
		}

		return true, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE insight_jobs
		SET status = 'processando',
		    tentativas = tentativas + 1,
		    proxima_tentativa_em = NULL,
		    erro_codigo = NULL,
		    erro_resumo = NULL,
		    atualizado_em = NOW()
		WHERE id = $1 AND aluno_id = $2
	`, message.JobID, message.AlunoID); err != nil {
		return false, fmt.Errorf("falha ao marcar job como processando: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("falha ao confirmar inicio do processamento: %w", err)
	}

	return false, nil
}

func (r *Repository) FindLowestPerformanceDisciplines(ctx context.Context, alunoID string, limit int) ([]DisciplinePerformance, error) {
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

	disciplines := make([]DisciplinePerformance, 0, limit)
	for rows.Next() {
		var discipline DisciplinePerformance
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

func (r *Repository) SaveInsightResult(ctx context.Context, message Message, content string, disciplines []DisciplinePerformance) error {
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

func (r *Repository) RegisterFailure(ctx context.Context, message Message, maxAttempts int, code string, failure error) (FailureAction, error) {
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
		    atualizado_em = NOW()
		WHERE id = $1 AND aluno_id = $2
		RETURNING status::text
	`, message.JobID, message.AlunoID, maxAttempts, code, summary).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("falha ao registrar erro do job de insight: %w", err)
	}

	if status == "falhou" {
		return FailureActionFailed, nil
	}

	return FailureActionRetry, nil
}

func (r *Repository) ClaimDueRetryJobs(ctx context.Context, limit int) ([]Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar transacao de retry: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
		WITH due_jobs AS (
			SELECT id, aluno_id
			FROM insight_jobs
			WHERE status = 'aguardando_retentativa'
			  AND proxima_tentativa_em <= NOW()
			ORDER BY proxima_tentativa_em ASC, atualizado_em ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE insight_jobs ij
		SET status = 'pendente',
		    proxima_tentativa_em = NULL,
		    atualizado_em = NOW()
		FROM due_jobs
		WHERE ij.id = due_jobs.id
		RETURNING ij.id::text, ij.aluno_id::text
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("falha ao buscar jobs para retry: %w", err)
	}
	defer rows.Close()

	messages := make([]Message, 0, limit)
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.JobID, &message.AlunoID); err != nil {
			return nil, fmt.Errorf("falha ao ler job para retry: %w", err)
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("falha ao iterar jobs para retry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("falha ao confirmar jobs de retry: %w", err)
	}

	return messages, nil
}
