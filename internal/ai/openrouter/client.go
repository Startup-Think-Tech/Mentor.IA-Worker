package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func New(config Config) (*Client, error) {
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("AI_PROVIDER_API_KEY nao pode estar vazia")
	}

	model := strings.TrimSpace(config.Model)
	if model == "" {
		return nil, fmt.Errorf("AI_MODEL nao pode estar vazio")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("AI_PROVIDER_BASE_URL nao pode estar vazia")
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) Complete(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt nao pode estar vazio")
	}

	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("falha ao serializar request OpenRouter: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("falha ao criar request OpenRouter: %w", err)
	}

	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-OpenRouter-Title", "Mentor.ia Worker")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("falha ao chamar OpenRouter: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("falha ao ler resposta OpenRouter: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("OpenRouter retornou status %d: %s", response.StatusCode, string(responseBody))
	}

	var chatResponse chatResponse
	if err := json.Unmarshal(responseBody, &chatResponse); err != nil {
		return "", fmt.Errorf("falha ao decodificar resposta OpenRouter: %w", err)
	}

	if len(chatResponse.Choices) == 0 {
		return "", fmt.Errorf("OpenRouter nao retornou escolhas")
	}

	content := strings.TrimSpace(chatResponse.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("OpenRouter retornou conteudo vazio")
	}

	return content, nil
}
