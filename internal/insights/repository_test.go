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

func TestRepositoryProcessWithFakeResult(t *testing.T) {
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
	email := "worker-test-" + uuid.NewString() + "@mentor.local"

	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM insights WHERE job_id = $1", jobID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM insight_jobs WHERE id = $1", jobID)
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

	repository := NewRepository(pool)
	if err := repository.ProcessWithFakeResult(ctx, Message{JobID: jobID, AlunoID: alunoID}); err != nil {
		t.Fatalf("ProcessWithFakeResult() returned error: %v", err)
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

	if insightContent != fakeInsightContent {
		t.Fatalf("conteudo = %q, want %q", insightContent, fakeInsightContent)
	}
}
