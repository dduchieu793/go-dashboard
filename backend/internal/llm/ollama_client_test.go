package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOllamaClientStatus(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		body           string
		available      bool
		modelAvailable bool
	}{
		{
			name:           "configured model is installed",
			statusCode:     http.StatusOK,
			body:           `{"models":[{"name":"llama3.2:1b"}]}`,
			available:      true,
			modelAvailable: true,
		},
		{
			name:           "ollama is available without configured model",
			statusCode:     http.StatusOK,
			body:           `{"models":[{"name":"another-model:latest"}]}`,
			available:      true,
			modelAvailable: false,
		},
		{
			name:           "model field also identifies installed model",
			statusCode:     http.StatusOK,
			body:           `{"models":[{"model":"llama3.2:1b"}]}`,
			available:      true,
			modelAvailable: true,
		},
		{
			name:           "malformed response still proves connectivity",
			statusCode:     http.StatusOK,
			body:           `{`,
			available:      true,
			modelAvailable: false,
		},
		{
			name:           "ollama error response",
			statusCode:     http.StatusServiceUnavailable,
			body:           `{"error":"unavailable"}`,
			available:      false,
			modelAvailable: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/tags" {
					t.Errorf("request path = %q, want /api/tags", request.URL.Path)
				}
				response.WriteHeader(test.statusCode)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()

			client := NewOllamaClient(server.URL, "llama3.2:1b", time.Second)
			status := client.Status(context.Background())

			if status.Available != test.available {
				t.Errorf("Available = %v, want %v", status.Available, test.available)
			}
			if status.Model != "llama3.2:1b" {
				t.Errorf("Model = %q, want llama3.2:1b", status.Model)
			}
			if status.ModelAvailable != test.modelAvailable {
				t.Errorf("ModelAvailable = %v, want %v", status.ModelAvailable, test.modelAvailable)
			}
		})
	}
}

func TestOllamaClientStatusWhenServerIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	client := NewOllamaClient(baseURL, "llama3.2:1b", 50*time.Millisecond)
	status := client.Status(context.Background())

	if status.Available {
		t.Error("Available = true, want false")
	}
	if status.ModelAvailable {
		t.Error("ModelAvailable = true, want false")
	}
	if status.Model != "llama3.2:1b" {
		t.Errorf("Model = %q, want llama3.2:1b", status.Model)
	}
}
