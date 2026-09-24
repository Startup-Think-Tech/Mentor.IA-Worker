package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

type APIError struct {
	StatusCode int
	Code       string
	Retry      bool
	Err        error
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("OpenRouter retornou %s (status %d): %v", e.Code, e.StatusCode, e.Err)
	}

	return fmt.Sprintf("OpenRouter retornou %s: %v", e.Code, e.Err)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

func (e *APIError) Retryable() bool {
	return e.Retry
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
		return "", classifyTransportError(err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("falha ao ler resposta OpenRouter: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", newAPIError(response.StatusCode, string(responseBody))
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

func classifyTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &APIError{Code: "timeout", Retry: true, Err: err}
	}

	if errors.Is(err, context.Canceled) {
		return &APIError{Code: "request_canceled", Retry: true, Err: err}
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		return &APIError{Code: "network_error", Retry: true, Err: err}
	}

	return &APIError{Code: "request_error", Retry: true, Err: err}
}

func newAPIError(statusCode int, responseBody string) error {
	code := "http_error"
	retryable := false

	switch {
	case statusCode == http.StatusBadRequest:
		code = "bad_request"
	case statusCode == http.StatusUnauthorized:
		code = "unauthorized"
	case statusCode == http.StatusForbidden:
		code = "forbidden"
	case statusCode == http.StatusTooManyRequests:
		code = "rate_limited"
		retryable = true
	case statusCode >= http.StatusInternalServerError:
		code = "server_error"
		retryable = true
	}

	responseBody = strings.TrimSpace(responseBody)
	if len(responseBody) > 1000 {
		responseBody = responseBody[:1000]
	}

	return &APIError{
		StatusCode: statusCode,
		Code:       code,
		Retry:      retryable,
		Err:        errors.New(responseBody),
	}
}
