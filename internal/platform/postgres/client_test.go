package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/daviPeter07/ai-worker/internal/testsupport"
)

func TestConnectRejectsEmptyDatabaseURL(t *testing.T) {
	_, err := Connect(context.Background(), "")
	if err == nil {
		t.Fatal("Connect() returned nil error")
	}
}

func TestConnectWithEnv(t *testing.T) {
	testsupport.LoadDotEnv(t)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL nao configurada")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() returned error: %v", err)
	}
	defer client.Close()

	var result int
	if err := client.Pool().QueryRow(ctx, "SELECT 1").Scan(&result); err != nil {
		t.Fatalf("QueryRow() returned error: %v", err)
	}

	if result != 1 {
		t.Fatalf("result = %d, want 1", result)
	}
}
