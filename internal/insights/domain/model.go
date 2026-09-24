package domain

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrOutboxDispatchLost  = errors.New("ownership do dispatch da outbox foi perdido")
	ErrProcessingLeaseLost = errors.New("ownership do processamento do job foi perdido")
)

type DisciplinePerformance struct {
	ID         string
	Nome       string
	Percentual float64
}

type ProcessingLease struct {
	Message Message
	Token   string
}

type FailureAction string

const (
	FailureActionFailed FailureAction = "failed"
	FailureActionRetry  FailureAction = "retry"
)

const (
	OutboxTypeInsightFailed         = "insight.failed"
	OutboxTypeInsightRetryRequested = "insight.retry.requested"
)

type InsightFailedEvent struct {
	JobID        string `json:"job_id"`
	AlunoID      string `json:"aluno_id"`
	ErrorCode    string `json:"error_code"`
	ErrorSummary string `json:"error_summary"`
}

type OutboxEvent struct {
	ID              string
	Type            string
	Payload         json.RawMessage
	ProcessingToken string
	Attempts        int
}

type OutboxPublisher func(ctx context.Context, event OutboxEvent) error
