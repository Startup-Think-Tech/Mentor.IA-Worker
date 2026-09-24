//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/domain"
	insightpostgres "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/insights/postgres"
	platformpostgres "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/platform/postgres"
	platformrabbitmq "github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/platform/rabbitmq"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

func TestDependenciesWithContainers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	postgresContainer, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mentor"),
		postgres.WithUsername("mentor"),
		postgres.WithPassword("mentor"),
	)
	if err != nil {
		t.Fatalf("postgres.Run() returned error: %v", err)
	}
	defer func() {
		if err := postgresContainer.Terminate(context.Background()); err != nil {
			t.Errorf("postgres.Terminate() returned error: %v", err)
		}
	}()

	databaseURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("ConnectionString() returned error: %v", err)
	}

	postgresClient, err := connectPostgresEventually(ctx, databaseURL)
	if err != nil {
		t.Fatalf("postgres.Connect() returned error: %v", err)
	}
	defer postgresClient.Close()

	var value int
	if err := postgresClient.Pool().QueryRow(ctx, "SELECT 1").Scan(&value); err != nil {
		t.Fatalf("SELECT 1 returned error: %v", err)
	}
	if value != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", value)
	}

	rabbitContainer, err := rabbitmq.Run(
		ctx,
		"rabbitmq:4-alpine",
		rabbitmq.WithAdminUsername("mentor"),
		rabbitmq.WithAdminPassword("mentor"),
	)
	if err != nil {
		t.Fatalf("rabbitmq.Run() returned error: %v", err)
	}
	defer func() {
		if err := rabbitContainer.Terminate(context.Background()); err != nil {
			t.Errorf("rabbitmq.Terminate() returned error: %v", err)
		}
	}()

	rabbitURL, err := rabbitContainer.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("AmqpURL() returned error: %v", err)
	}

	rabbitClient, err := platformrabbitmq.Connect(platformrabbitmq.Config{
		URL:      rabbitURL,
		Queue:    "insights_queue",
		DLQ:      "insights_dlq",
		Prefetch: 2,
	})
	if err != nil {
		t.Fatalf("rabbitmq.Connect() returned error: %v", err)
	}
	defer func() {
		if err := rabbitClient.Close(); err != nil {
			t.Errorf("rabbitmq.Close() returned error: %v", err)
		}
	}()

	deliveries, err := rabbitClient.Consume("integration-test")
	if err != nil {
		t.Fatalf("Consume() returned error: %v", err)
	}

	if err := rabbitClient.PublishInsightMessage(ctx, map[string]string{"job_id": "job-1", "aluno_id": "aluno-1"}); err != nil {
		t.Fatalf("PublishInsightMessage() returned error: %v", err)
	}

	select {
	case delivery := <-deliveries:
		if err := delivery.Ack(false); err != nil {
			t.Fatalf("Ack() returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RabbitMQ delivery")
	}
}

func TestRepositoryReliabilityWithPostgresContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	container, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mentor"),
		postgres.WithUsername("mentor"),
		postgres.WithPassword("mentor"),
	)
	if err != nil {
		t.Fatalf("postgres.Run() returned error: %v", err)
	}
	defer func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("postgres.Terminate() returned error: %v", err)
		}
	}()

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("ConnectionString() returned error: %v", err)
	}

	client, err := connectPostgresEventually(ctx, databaseURL)
	if err != nil {
		t.Fatalf("postgres.Connect() returned error: %v", err)
	}
	defer client.Close()

	for _, statement := range []string{
		`CREATE TYPE "InsightJobStatus" AS ENUM ('pendente', 'processando', 'aguardando_retentativa', 'falhou', 'concluido')`,
		`CREATE TABLE alunos (id UUID PRIMARY KEY)`,
		`CREATE TABLE insight_jobs (
			id UUID PRIMARY KEY,
			aluno_id UUID NOT NULL,
			status "InsightJobStatus" NOT NULL DEFAULT 'pendente',
			idempotency_key TEXT NOT NULL,
			payload_hash TEXT NOT NULL,
			tentativas INTEGER NOT NULL DEFAULT 0,
			proxima_tentativa_em TIMESTAMP(3),
			processing_token UUID,
			lease_expira_em TIMESTAMP(3),
			erro_codigo TEXT,
			erro_resumo TEXT,
			atualizado_em TIMESTAMP(3) NOT NULL DEFAULT NOW(),
			concluido_em TIMESTAMP(3)
		)`,
		`CREATE TABLE outbox_eventos (
			id UUID PRIMARY KEY,
			job_id UUID NOT NULL,
			tipo TEXT NOT NULL,
			payload JSONB NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			tentativas INTEGER NOT NULL DEFAULT 0,
			erro_resumo TEXT,
			proxima_tentativa_em TIMESTAMP(3) NOT NULL DEFAULT NOW(),
			processing_token UUID,
			lock_expira_em TIMESTAMP(3),
			criado_em TIMESTAMP(3) NOT NULL DEFAULT NOW(),
			publicado_em TIMESTAMP(3)
		)`,
	} {
		if _, err := client.Pool().Exec(ctx, statement); err != nil {
			t.Fatalf("schema setup returned error: %v", err)
		}
	}

	alunoID := uuid.New()
	jobID := uuid.New()
	if _, err := client.Pool().Exec(ctx, `INSERT INTO alunos (id) VALUES ($1)`, alunoID); err != nil {
		t.Fatalf("insert aluno returned error: %v", err)
	}
	if _, err := client.Pool().Exec(ctx, `
		INSERT INTO insight_jobs (id, aluno_id, idempotency_key, payload_hash)
		VALUES ($1, $2, $3, $4)
	`, jobID, alunoID, "key-1", "hash-1"); err != nil {
		t.Fatalf("insert insight job returned error: %v", err)
	}

	repository := insightpostgres.NewRepository(client.Pool(), time.Minute)
	message := domain.Message{JobID: jobID.String(), AlunoID: alunoID.String()}
	firstLease, alreadyHandled, err := repository.BeginProcessing(ctx, message)
	if err != nil {
		t.Fatalf("first BeginProcessing() returned error: %v", err)
	}
	if alreadyHandled {
		t.Fatal("first claim should not be already handled")
	}

	if _, err := client.Pool().Exec(ctx, `UPDATE insight_jobs SET lease_expira_em = NOW() - INTERVAL '1 second' WHERE id = $1`, jobID); err != nil {
		t.Fatalf("expire lease returned error: %v", err)
	}

	secondLease, alreadyHandled, err := repository.BeginProcessing(ctx, message)
	if err != nil {
		t.Fatalf("second BeginProcessing() returned error: %v", err)
	}
	if alreadyHandled || firstLease.Token == secondLease.Token {
		t.Fatal("expired job should be claimed with a new token")
	}

	if _, err := repository.RegisterFailure(ctx, firstLease, 3, "AI_COMPLETION_FAILED", errors.New("stale worker")); !errors.Is(err, domain.ErrProcessingLeaseLost) {
		t.Fatalf("RegisterFailure() error = %v, want ErrProcessingLeaseLost", err)
	}

	action, err := repository.RegisterFailure(ctx, secondLease, 1, "AI_COMPLETION_FAILED", errors.New("invalid API key"))
	if err != nil {
		t.Fatalf("RegisterFailure() returned error: %v", err)
	}
	if action != domain.FailureActionFailed {
		t.Fatalf("action = %q, want %q", action, domain.FailureActionFailed)
	}

	var jobStatus string
	if err := client.Pool().QueryRow(ctx, `SELECT status::text FROM insight_jobs WHERE id = $1`, jobID).Scan(&jobStatus); err != nil {
		t.Fatalf("select job status returned error: %v", err)
	}
	if jobStatus != "falhou" {
		t.Fatalf("job status = %q, want falhou", jobStatus)
	}

	var failedPayload []byte
	if err := client.Pool().QueryRow(ctx, `
		SELECT payload
		FROM outbox_eventos
		WHERE job_id = $1 AND tipo = $2
	`, jobID, domain.OutboxTypeInsightFailed).Scan(&failedPayload); err != nil {
		t.Fatalf("select failure outbox event returned error: %v", err)
	}

	var failedEvent domain.InsightFailedEvent
	if err := json.Unmarshal(failedPayload, &failedEvent); err != nil {
		t.Fatalf("decode failure outbox payload returned error: %v", err)
	}
	if failedEvent.JobID != jobID.String() || failedEvent.AlunoID != alunoID.String() || failedEvent.ErrorCode != "AI_COMPLETION_FAILED" {
		t.Fatalf("failure payload = %#v", failedEvent)
	}

	eventID := uuid.New()
	if _, err := client.Pool().Exec(ctx, `
		INSERT INTO outbox_eventos (id, job_id, tipo, payload)
		VALUES ($1, $2, $3, $4)
	`, eventID, jobID, domain.OutboxTypeInsightRetryRequested, []byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`)); err != nil {
		t.Fatalf("insert outbox event returned error: %v", err)
	}

	events, err := repository.ClaimPendingOutboxEvents(ctx, 1, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingOutboxEvents() returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	failed, err := repository.MarkOutboxEventFailed(ctx, events[0], 1, errors.New("poison event"))
	if err != nil {
		t.Fatalf("MarkOutboxEventFailed() returned error: %v", err)
	}
	if !failed {
		t.Fatal("poison event should reach failed status after its configured limit")
	}
}

func connectPostgresEventually(ctx context.Context, databaseURL string) (*platformpostgres.Client, error) {
	var lastErr error
	for {
		client, err := platformpostgres.Connect(ctx, databaseURL)
		if err == nil {
			return client, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return nil, lastErr
		case <-time.After(250 * time.Millisecond):
		}
	}
}
