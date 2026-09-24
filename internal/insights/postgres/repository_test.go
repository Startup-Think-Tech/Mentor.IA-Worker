package postgres

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
	platformpostgres "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/platform/postgres"
	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/testsupport"
	"github.com/google/uuid"
)

func TestRepositoryProcessesRealInsightResult(t *testing.T) {
	testsupport.LoadDotEnv(t)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL nao configurada")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	postgresClient, err := platformpostgres.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("postgres.Connect() returned error: %v", err)
	}
	defer postgresClient.Close()

	pool := postgresClient.Pool()
	alunoID := uuid.NewString()
	jobID := uuid.NewString()
	disciplinaID := uuid.NewString()
	conteudoID := uuid.NewString()
	email := "worker-test-" + uuid.NewString() + "@mentor.local"
	disciplinaCode := "worker-" + uuid.NewString()

	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM insights_disciplinas WHERE insight_id IN (SELECT id FROM insights WHERE job_id = $1)", jobID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM insights WHERE job_id = $1", jobID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM insight_jobs WHERE id = $1", jobID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM registros_desempenho WHERE aluno_id = $1", alunoID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM conteudos WHERE id = $1", conteudoID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM disciplinas WHERE id = $1", disciplinaID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM alunos WHERE id = $1", alunoID)
	}()

	if _, err := pool.Exec(ctx, `
		INSERT INTO alunos (id, nome, email, senha_hash, atualizado_em)
		VALUES ($1, $2, $3, $4, NOW())
	`, alunoID, "Worker Test", email, "hash"); err != nil {
		t.Fatalf("failed to insert aluno: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO insight_jobs (id, aluno_id, idempotency_key, payload_hash, atualizado_em)
		VALUES ($1, $2, $3, $4, NOW())
	`, jobID, alunoID, "test-key-"+jobID, "payload-hash"); err != nil {
		t.Fatalf("failed to insert insight job: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO disciplinas (id, nome, codigo)
		VALUES ($1, $2, $3)
	`, disciplinaID, "Worker Disciplina "+disciplinaID, disciplinaCode); err != nil {
		t.Fatalf("failed to insert disciplina: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO conteudos (id, disciplina_id, nome)
		VALUES ($1, $2, $3)
	`, conteudoID, disciplinaID, "Worker Conteudo"); err != nil {
		t.Fatalf("failed to insert conteudo: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO registros_desempenho (id, aluno_id, disciplina_id, conteudo_id, tipo_origem, total_questoes, acertos, percentual)
		VALUES ($1, $2, $3, $4, 'manual', 10, 4, 40.00)
	`, uuid.NewString(), alunoID, disciplinaID, conteudoID); err != nil {
		t.Fatalf("failed to insert registro desempenho: %v", err)
	}

	repository := NewRepository(pool, 2*time.Minute)
	lease, alreadyHandled, err := repository.BeginProcessing(ctx, domain.Message{JobID: jobID, AlunoID: alunoID})
	if err != nil {
		t.Fatalf("BeginProcessing() returned error: %v", err)
	}

	if alreadyHandled {
		t.Fatal("job should not be already handled")
	}

	if lease.Token == "" {
		t.Fatal("lease token should not be empty")
	}

	disciplines, err := repository.FindLowestPerformanceDisciplines(ctx, alunoID, 3)
	if err != nil {
		t.Fatalf("FindLowestPerformanceDisciplines() returned error: %v", err)
	}

	if len(disciplines) != 1 {
		t.Fatalf("len(disciplines) = %d, want 1", len(disciplines))
	}

	if err := repository.SaveInsightResult(ctx, lease, "Insight real", disciplines); err != nil {
		t.Fatalf("SaveInsightResult() returned error: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM insight_jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("failed to select job status: %v", err)
	}

	if status != "concluido" {
		t.Fatalf("status = %q, want concluido", status)
	}

	var insightContent string
	if err := pool.QueryRow(ctx, `SELECT conteudo FROM insights WHERE job_id = $1`, jobID).Scan(&insightContent); err != nil {
		t.Fatalf("failed to select insight: %v", err)
	}

	if insightContent != "Insight real" {
		t.Fatalf("conteudo = %q, want Insight real", insightContent)
	}

	var associationCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM insights_disciplinas id
		JOIN insights i ON i.id = id.insight_id
		WHERE i.job_id = $1
	`, jobID).Scan(&associationCount); err != nil {
		t.Fatalf("failed to count insight disciplines: %v", err)
	}

	if associationCount != 1 {
		t.Fatalf("associationCount = %d, want 1", associationCount)
	}
}

func TestRepositoryScheduleDueRetryJobs(t *testing.T) {
	testsupport.LoadDotEnv(t)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL nao configurada")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	postgresClient, err := platformpostgres.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("postgres.Connect() returned error: %v", err)
	}
	defer postgresClient.Close()

	pool := postgresClient.Pool()
	alunoID := uuid.NewString()
	jobID := uuid.NewString()
	email := "worker-retry-test-" + uuid.NewString() + "@mentor.local"

	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM outbox_eventos WHERE job_id = $1", jobID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM insight_jobs WHERE id = $1", jobID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM alunos WHERE id = $1", alunoID)
	}()

	if _, err := pool.Exec(ctx, `
		INSERT INTO alunos (id, nome, email, senha_hash, atualizado_em)
		VALUES ($1, $2, $3, $4, NOW())
	`, alunoID, "Worker Retry Test", email, "hash"); err != nil {
		t.Fatalf("failed to insert aluno: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO insight_jobs (id, aluno_id, status, idempotency_key, payload_hash, proxima_tentativa_em, atualizado_em)
		VALUES ($1, $2, 'aguardando_retentativa', $3, $4, NOW() - INTERVAL '1 minute', NOW())
	`, jobID, alunoID, "retry-key-"+jobID, "payload-hash"); err != nil {
		t.Fatalf("failed to insert retry job: %v", err)
	}

	repository := NewRepository(pool, 2*time.Minute)
	count, err := repository.ScheduleDueRetryJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ScheduleDueRetryJobs() returned error: %v", err)
	}

	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM insight_jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("failed to select job status: %v", err)
	}

	if status != "pendente" {
		t.Fatalf("status = %q, want pendente", status)
	}

	var eventType string
	var payloadBytes []byte
	var payload domain.Message
	if err := pool.QueryRow(ctx, `
		SELECT tipo, payload
		FROM outbox_eventos
		WHERE job_id = $1
	`, jobID).Scan(&eventType, &payloadBytes); err != nil {
		t.Fatalf("failed to select outbox event: %v", err)
	}

	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("failed to decode outbox payload: %v", err)
	}

	if eventType != domain.OutboxTypeInsightRetryRequested {
		t.Fatalf("eventType = %q, want %q", eventType, domain.OutboxTypeInsightRetryRequested)
	}

	if payload.JobID != jobID || payload.AlunoID != alunoID {
		t.Fatalf("payload = %#v, want job %s aluno %s", payload, jobID, alunoID)
	}

	events, err := repository.ClaimPendingOutboxEvents(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingOutboxEvents() returned error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	if events[0].ProcessingToken == "" {
		t.Fatal("claimed outbox event should have a processing token")
	}

	if err := repository.MarkOutboxEventPublished(ctx, events[0]); err != nil {
		t.Fatalf("MarkOutboxEventPublished() returned error: %v", err)
	}

	var outboxStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM outbox_eventos WHERE id = $1`, events[0].ID).Scan(&outboxStatus); err != nil {
		t.Fatalf("failed to select outbox status: %v", err)
	}

	if outboxStatus != "published" {
		t.Fatalf("outbox status = %q, want published", outboxStatus)
	}
}
