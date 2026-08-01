package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaClient(baseURL, model string, timeout time.Duration) *OllamaClient {
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *OllamaClient) Status(ctx context.Context) Status {
	status := Status{Model: c.model}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
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
