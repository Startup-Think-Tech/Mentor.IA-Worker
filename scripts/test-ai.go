//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Startup-Think-Tech/Mentor.IA-Worker/internal/ai/openrouter"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	apiKey := strings.TrimSpace(os.Getenv("AI_PROVIDER_API_KEY"))
	baseURL := strings.TrimSpace(os.Getenv("AI_PROVIDER_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("AI_MODEL"))

	if apiKey == "" {
		fail("AI_PROVIDER_API_KEY nao configurada")
	}

	if baseURL == "" {
		fail("AI_PROVIDER_BASE_URL nao configurada")
	}

	if model == "" {
		fail("AI_MODEL nao configurado")
	}

	client, err := openrouter.New(openrouter.Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		fail("falha ao criar client de IA: %v", err)
	}

	prompt := strings.TrimSpace(strings.Join(os.Args[1:], " "))
	if prompt == "" {
		prompt = "Responda em portugues do Brasil, em uma frase curta: teste de conectividade da IA do Mentor.ia."
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	startedAt := time.Now()
	content, err := client.Complete(ctx, prompt)
	if err != nil {
		fail("falha ao chamar IA: %v", err)
	}

	fmt.Println("IA respondeu com sucesso")
	fmt.Printf("modelo: %s\n", model)
	fmt.Printf("duracao: %s\n", time.Since(startedAt).Round(time.Millisecond))
	fmt.Println("resposta:")
	fmt.Println(content)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
