package llm

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
)

type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
	keepAlive  string
}

const statusTimeout = 3 * time.Second

type ollamaGenerateRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	Stream    bool   `json:"stream"`
	Format    string `json:"format"`
	Think     bool   `json:"think"`
	KeepAlive string `json:"keep_alive"`
}

type ollamaGenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Error    string `json:"error"`
}

func NewOllamaClient(baseURL, model string, timeout time.Duration, keepAlive ...string) *OllamaClient {
	configuredKeepAlive := "-1m"
	if len(keepAlive) > 0 && keepAlive[0] != "" {
		configuredKeepAlive = keepAlive[0]
	}
	return &OllamaClient{
		baseURL:   baseURL,
		model:     model,
		keepAlive: configuredKeepAlive,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *OllamaClient) Generate(ctx context.Context, prompt string) (Generation, error) {
	payload, err := json.Marshal(ollamaGenerateRequest{
		Model: c.model, Prompt: prompt, Stream: false, Format: "json", Think: false, KeepAlive: c.keepAlive,
	})
	if err != nil {
		return Generation{}, fmt.Errorf("encode Ollama request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return Generation{}, fmt.Errorf("create Ollama request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Generation{}, context.DeadlineExceeded
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return Generation{}, context.Canceled
		}
		return Generation{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()

	var result ollamaGenerateResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		if response.StatusCode == http.StatusNotFound {
			return Generation{}, ErrModelNotFound
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return Generation{}, fmt.Errorf("%w: Ollama returned status %d", ErrUnavailable, response.StatusCode)
		}
		return Generation{}, fmt.Errorf("%w: decode Ollama response", ErrInvalidResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode == http.StatusNotFound || strings.Contains(strings.ToLower(result.Error), "model") {
			return Generation{}, ErrModelNotFound
		}
		return Generation{}, fmt.Errorf("%w: Ollama returned status %d", ErrUnavailable, response.StatusCode)
	}
	if strings.TrimSpace(result.Response) == "" {
		return Generation{}, fmt.Errorf("%w: empty generated content", ErrInvalidResponse)
	}
	model := result.Model
	if model == "" {
		model = c.model
	}
	return Generation{Content: result.Response, Model: model}, nil
}

func (c *OllamaClient) Status(ctx context.Context) Status {
	status := Status{Model: c.model}
	statusCtx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(statusCtx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return status
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return status
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return status
	}
	status.Available = true

	var tags struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tags); err != nil {
		return status
	}

	for _, installed := range tags.Models {
		if installed.Name == c.model || installed.Model == c.model {
			status.ModelAvailable = true
			break
		}
	}
	return status
}
