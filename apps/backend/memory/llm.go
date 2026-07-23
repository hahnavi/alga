package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"alga/strutil"
)

type LLM interface {
	Generate(ctx context.Context, messages []Message) (string, error)
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiLLM struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

type openAIChatRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func NewOpenAILLM(baseURL, apiKey, model string) LLM {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &openaiLLM{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *openaiLLM) Generate(ctx context.Context, messages []Message) (string, error) {
	reqBody := openAIChatRequest{
		Model:    l.model,
		Messages: messages,
		ResponseFormat: &struct {
			Type string `json:"type"`
		}{Type: "json_object"},
		Temperature: 0.1,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal llm request: %w", err)
	}

	base := strings.TrimRight(l.baseURL, "/")
	endpoint := base
	if !strings.HasSuffix(base, "/chat/completions") {
		endpoint = base + "/chat/completions"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if l.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+l.apiKey)
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("read llm response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm API error (%d): %s", resp.StatusCode, strutil.Truncate(string(body), 300))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal llm response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.New("llm returned no choices")
	}
	return chatResp.Choices[0].Message.Content, nil
}
