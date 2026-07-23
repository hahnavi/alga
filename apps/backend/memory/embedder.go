package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"alga/strutil"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

type openaiEmbedder struct {
	baseURL    string
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func NewOpenAIEmbedder(baseURL, apiKey, model string) Embedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	dim := 1536
	if strings.Contains(model, "large") {
		dim = 3072
	}
	return &openaiEmbedder{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		dimension:  dim,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *openaiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := openAIEmbeddingRequest{
		Model: e.model,
		Input: texts,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	base := strings.TrimRight(e.baseURL, "/")
	endpoint := base
	if !strings.HasSuffix(base, "/embeddings") {
		endpoint = base + "/embeddings"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API error (%d): %s", resp.StatusCode, strutil.Truncate(string(body), 200))
	}

	var embedResp openAIEmbeddingResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}

	results := make([][]float32, len(texts))
	for _, d := range embedResp.Data {
		if d.Index >= 0 && d.Index < len(results) {
			results[d.Index] = d.Embedding
		}
	}
	return results, nil
}

func (e *openaiEmbedder) Dimension() int {
	return e.dimension
}

type noopEmbedder struct {
	dim int
}

func NewNoopEmbedder() Embedder {
	return &noopEmbedder{dim: 1536}
}

func (e *noopEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, e.dim)
	}
	return out, nil
}

func (e *noopEmbedder) Dimension() int {
	return e.dim
}
