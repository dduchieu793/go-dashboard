package llm

import (
	"context"
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return Status{Available: false}
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Status{Available: false}
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Status{Available: false}
	}
	return Status{Available: true, Model: c.model}
}
