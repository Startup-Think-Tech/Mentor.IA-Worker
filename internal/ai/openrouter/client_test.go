package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRejectsEmptyAPIKey(t *testing.T) {
	_, err := New(Config{Model: "openrouter/free"})
	if err == nil {
		t.Fatal("New() returned nil error")
	}
}

func TestComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}

		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("Authorization header was not set")
		}

		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if request.Model != "openrouter/free" {
			t.Fatalf("model = %q, want openrouter/free", request.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Insight gerado"}}]}`))
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "openrouter/free",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	content, err := client.Complete(context.Background(), "gere um insight")
	if err != nil {
		t.Fatalf("Complete() returned error: %v", err)
	}

	if content != "Insight gerado" {
		t.Fatalf("content = %q, want %q", content, "Insight gerado")
	}
}

func TestCompleteRejectsEmptyPrompt(t *testing.T) {
	client, err := New(Config{APIKey: "test-key", Model: "openrouter/free"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = client.Complete(context.Background(), " ")
	if err == nil {
		t.Fatal("Complete() returned nil error")
	}
}
