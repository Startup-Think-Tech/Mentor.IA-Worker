package insights

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fakeInsightContent = "Insight fake gerado pelo worker Go. A chamada real de IA sera integrada no proximo PR."

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ProcessWithFakeResult(ctx context.Context, message Message) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("falha ao iniciar transacao de insight: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var status string
	err = tx.QueryRow(ctx, `
		SELECT status::text
		FROM insight_jobs
		WHERE id = $1 AND aluno_id = $2
		FOR UPDATE
	`, message.JobID, message.AlunoID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("job de insight nao encontrado: %s", message.JobID)
		}

		return fmt.Errorf("falha ao buscar job de insight: %w", err)
	}

	if status == "concluido" {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("falha ao confirmar job ja concluido: %w", err)
		}

		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE insight_jobs
		SET status = 'processando',
		    tentativas = tentativas + 1,
		    erro_codigo = NULL,
		    erro_resumo = NULL,
		    atualizado_em = NOW()
		WHERE id = $1 AND aluno_id = $2
	`, message.JobID, message.AlunoID); err != nil {
		return fmt.Errorf("falha ao marcar job como processando: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO insights (id, aluno_id, job_id, conteudo)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (job_id) DO UPDATE
		SET conteudo = EXCLUDED.conteudo
	`, uuid.NewString(), message.AlunoID, message.JobID, fakeInsightContent); err != nil {
		return fmt.Errorf("falha ao salvar insight fake: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE insight_jobs
		SET status = 'concluido',
		    concluido_em = NOW(),
		    atualizado_em = NOW(),
		    erro_codigo = NULL,
		    erro_resumo = NULL
		WHERE id = $1 AND aluno_id = $2
	`, message.JobID, message.AlunoID); err != nil {
		return fmt.Errorf("falha ao marcar job como concluido: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("falha ao confirmar transacao de insight: %w", err)
	}

	return nil
}
