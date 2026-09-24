package domain

import (
	"context"
	"encoding/json"
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

const OutboxTypeInsightRetryRequested = "insight.retry.requested"

type OutboxEvent struct {
	ID      string
	Type    string
	Payload json.RawMessage
}

type OutboxPublisher func(ctx context.Context, event OutboxEvent) error
