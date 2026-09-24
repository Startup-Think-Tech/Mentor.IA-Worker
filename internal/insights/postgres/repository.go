package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
	generated "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/postgres/sqlc/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool          *pgxpool.Pool
	queries       *generated.Queries
	leaseDuration time.Duration
}

func NewRepository(pool *pgxpool.Pool, leaseDuration time.Duration) *Repository {
	return &Repository{pool: pool, queries: generated.New(pool), leaseDuration: leaseDuration}
}

func (r *Repository) BeginProcessing(ctx context.Context, message domain.Message) (domain.ProcessingLease, bool, error) {
	if r.leaseDuration <= 0 {
		return domain.ProcessingLease{}, false, fmt.Errorf("leaseDuration deve ser maior que zero")
	}

	token := uuid.NewString()
	leaseSeconds := int(r.leaseDuration.Seconds())
	if leaseSeconds <= 0 {
		return domain.ProcessingLease{}, false, fmt.Errorf("leaseDuration deve ter pelo menos um segundo")
	}

	var claimedToken string
	err := r.pool.QueryRow(ctx, `
		UPDATE insight_jobs
		SET status = 'processando',
		    tentativas = tentativas + 1,
		    processing_token = $3,
		    lease_expira_em = NOW() + ($4::int * INTERVAL '1 second'),
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
		RETURNING processing_token::text
	`, message.JobID, message.AlunoID, token, leaseSeconds).Scan(&claimedToken)
	if err == nil {
		return domain.ProcessingLease{Message: message, Token: claimedToken}, false, nil
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
				return domain.ProcessingLease{}, false, fmt.Errorf("falha ao verificar job nao adquirido: %w", existsErr)
			}

			if !exists {
				return domain.ProcessingLease{}, false, fmt.Errorf("job de insight nao encontrado: %s", message.JobID)
			}

			return domain.ProcessingLease{}, true, nil
		}

		return domain.ProcessingLease{}, false, fmt.Errorf("falha ao adquirir job de insight: %w", err)
	}

	return domain.ProcessingLease{}, false, nil
}

func (r *Repository) FindLowestPerformanceDisciplines(ctx context.Context, alunoID string, limit int) ([]domain.DisciplinePerformance, error) {
	parsedAlunoID, err := uuid.Parse(alunoID)
	if err != nil {
		return nil, fmt.Errorf("aluno_id invalido: %w", err)
	}

	rows, err := r.queries.FindLowestPerformanceDisciplines(ctx, generated.FindLowestPerformanceDisciplinesParams{
		AlunoID: parsedAlunoID,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("falha ao buscar disciplinas de menor desempenho: %w", err)
	}

	disciplines := make([]domain.DisciplinePerformance, 0, limit)
	for _, row := range rows {
		disciplines = append(disciplines, domain.DisciplinePerformance{
			ID:         row.ID.String(),
			Nome:       row.Nome,
			Percentual: row.PercentualMedio,
		})
	}

	return disciplines, nil
}

func (r *Repository) SaveInsightResult(ctx context.Context, lease domain.ProcessingLease, content string, disciplines []domain.DisciplinePerformance) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("falha ao iniciar transacao para salvar insight: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	commandTag, err := tx.Exec(ctx, `
		UPDATE insight_jobs
		SET status = 'concluido',
		    concluido_em = NOW(),
		    atualizado_em = NOW(),
		    processing_token = NULL,
		    lease_expira_em = NULL,
		    proxima_tentativa_em = NULL,
		    erro_codigo = NULL,
		    erro_resumo = NULL
		WHERE id = $1
		  AND aluno_id = $2
		  AND status = 'processando'
		  AND processing_token = $3
	`, lease.Message.JobID, lease.Message.AlunoID, lease.Token)
	if err != nil {
		return fmt.Errorf("falha ao marcar job como concluido: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return domain.ErrProcessingLeaseLost
	}

	var insightID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO insights (id, aluno_id, job_id, conteudo)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (job_id) DO UPDATE
		SET conteudo = EXCLUDED.conteudo
		RETURNING id::text
	`, uuid.NewString(), lease.Message.AlunoID, lease.Message.JobID, content).Scan(&insightID); err != nil {
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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("falha ao confirmar insight: %w", err)
	}

	return nil
}

func (r *Repository) RegisterFailure(ctx context.Context, lease domain.ProcessingLease, maxAttempts int, code string, failure error) (domain.FailureAction, error) {
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
		    processing_token = NULL,
		    lease_expira_em = NULL,
		    atualizado_em = NOW()
		WHERE id = $1
		  AND aluno_id = $2
		  AND status = 'processando'
		  AND processing_token = $6
		RETURNING status::text
	`, lease.Message.JobID, lease.Message.AlunoID, maxAttempts, code, summary, lease.Token).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrProcessingLeaseLost
		}

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
			    processing_token = NULL,
			    lease_expira_em = NULL,
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

func (r *Repository) ClaimPendingOutboxEvents(ctx context.Context, limit int, lockDuration time.Duration) ([]domain.OutboxEvent, error) {
	if lockDuration <= 0 {
		return nil, fmt.Errorf("lockDuration deve ser maior que zero")
	}

	lockSeconds := int(lockDuration.Seconds())
	if lockSeconds <= 0 {
		return nil, fmt.Errorf("lockDuration deve ter pelo menos um segundo")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar transacao de claim da outbox: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	token := uuid.NewString()
	rows, err := tx.Query(ctx, `
		WITH eventos_disponiveis AS (
			SELECT id
			FROM outbox_eventos
			WHERE (status = 'pending' AND proxima_tentativa_em <= NOW())
			   OR (status = 'processing' AND lock_expira_em <= NOW())
			ORDER BY proxima_tentativa_em ASC, criado_em ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_eventos oe
		SET status = 'processing',
		    processing_token = $2,
		    lock_expira_em = NOW() + ($3::int * INTERVAL '1 second'),
		    tentativas = oe.tentativas + 1
		FROM eventos_disponiveis ed
		WHERE oe.id = ed.id
		RETURNING oe.id::text, oe.tipo, oe.payload, oe.processing_token::text, oe.tentativas
	`, limit, token, lockSeconds)
	if err != nil {
		return nil, fmt.Errorf("falha ao buscar eventos de outbox: %w", err)
	}
	defer rows.Close()

	events := make([]domain.OutboxEvent, 0, limit)
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.Payload, &event.ProcessingToken, &event.Attempts); err != nil {
			return nil, fmt.Errorf("falha ao ler evento de outbox: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("falha ao iterar eventos de outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("falha ao confirmar claim de eventos da outbox: %w", err)
	}

	return events, nil
}

func (r *Repository) MarkOutboxEventPublished(ctx context.Context, event domain.OutboxEvent) error {
	commandTag, err := r.pool.Exec(ctx, `
		UPDATE outbox_eventos
		SET status = 'published',
		    publicado_em = NOW(),
		    processing_token = NULL,
		    lock_expira_em = NULL,
		    erro_resumo = NULL
		WHERE id = $1
		  AND status = 'processing'
		  AND processing_token = $2
	`, event.ID, event.ProcessingToken)
	if err != nil {
		return fmt.Errorf("falha ao marcar evento de outbox como publicado: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return domain.ErrOutboxDispatchLost
	}

	return nil
}

func (r *Repository) MarkOutboxEventFailed(ctx context.Context, event domain.OutboxEvent, maxAttempts int, failure error) (bool, error) {
	if maxAttempts <= 0 {
		return false, fmt.Errorf("maxAttempts deve ser maior que zero")
	}

	summary := strings.TrimSpace(failure.Error())
	if len(summary) > 500 {
		summary = summary[:500]
	}

	retrySeconds := min(300, max(5, event.Attempts*15))
	var status string
	err := r.pool.QueryRow(ctx, `
		UPDATE outbox_eventos
		SET status = CASE
		      WHEN tentativas >= $3 THEN 'failed'
		      ELSE 'pending'
		    END,
		    erro_resumo = $4,
		    proxima_tentativa_em = CASE
		      WHEN tentativas >= $3 THEN proxima_tentativa_em
		      ELSE NOW() + ($5::int * INTERVAL '1 second')
		    END,
		    processing_token = NULL,
		    lock_expira_em = NULL
		WHERE id = $1
		  AND status = 'processing'
		  AND processing_token = $2
		RETURNING status
	`, event.ID, event.ProcessingToken, maxAttempts, summary, retrySeconds).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrOutboxDispatchLost
		}

		return false, fmt.Errorf("falha ao registrar falha de evento da outbox: %w", err)
	}

	return status == "failed", nil
}
