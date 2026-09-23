package insights

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/daviPeter07/ai-worker/internal/platform/postgres"
	"github.com/daviPeter07/ai-worker/internal/testsupport"
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

	postgresClient, err := postgres.Connect(ctx, databaseURL)
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

	repository := NewRepository(pool)
	alreadyHandled, err := repository.BeginProcessing(ctx, Message{JobID: jobID, AlunoID: alunoID})
	if err != nil {
		t.Fatalf("BeginProcessing() returned error: %v", err)
	}

	if alreadyHandled {
		t.Fatal("job should not be already handled")
	}

	disciplines, err := repository.FindLowestPerformanceDisciplines(ctx, alunoID, 3)
	if err != nil {
		t.Fatalf("FindLowestPerformanceDisciplines() returned error: %v", err)
	}

	if len(disciplines) != 1 {
		t.Fatalf("len(disciplines) = %d, want 1", len(disciplines))
	}

	if err := repository.SaveInsightResult(ctx, Message{JobID: jobID, AlunoID: alunoID}, "Insight real", disciplines); err != nil {
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

func TestRepositoryClaimDueRetryJobs(t *testing.T) {
	testsupport.LoadDotEnv(t)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL nao configurada")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	postgresClient, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("postgres.Connect() returned error: %v", err)
	}
	defer postgresClient.Close()

	pool := postgresClient.Pool()
	alunoID := uuid.NewString()
	jobID := uuid.NewString()
	email := "worker-retry-test-" + uuid.NewString() + "@mentor.local"

	defer func() {
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

	repository := NewRepository(pool)
	messages, err := repository.ClaimDueRetryJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueRetryJobs() returned error: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}

	if messages[0].JobID != jobID || messages[0].AlunoID != alunoID {
		t.Fatalf("message = %#v, want job %s aluno %s", messages[0], jobID, alunoID)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM insight_jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("failed to select job status: %v", err)
	}

	if status != "pendente" {
		t.Fatalf("status = %q, want pendente", status)
	}
}
