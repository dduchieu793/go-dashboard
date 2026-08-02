package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestOllamaClientGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/generate" {
			t.Errorf("request = %s %s, want POST /api/generate", request.Method, request.URL.Path)
		}
		if contentType := request.Header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", contentType)
		}
		body, _ := io.ReadAll(request.Body)
		var payload ollamaGenerateRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "llama3.2:1b" || payload.Prompt != "prompt" || payload.Stream || payload.Format != "json" || payload.Think || payload.KeepAlive != "-1m" {
			t.Errorf("request payload = %+v", payload)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"model":"llama3.2:1b","response":"{\"summary\":\"result\"}"}`))
	}))
	defer server.Close()

	result, err := NewOllamaClient(server.URL, "llama3.2:1b", time.Second).Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Model != "llama3.2:1b" || result.Content != `{"summary":"result"}` {
		t.Errorf("Generate() = %+v", result)
	}
}

func TestOllamaClientGenerateErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
	}{
		{name: "model missing", statusCode: http.StatusNotFound, body: `{"error":"model not found"}`, wantErr: ErrModelNotFound},
		{name: "provider unavailable", statusCode: http.StatusServiceUnavailable, body: `{"error":"busy"}`, wantErr: ErrUnavailable},
		{name: "malformed response", statusCode: http.StatusOK, body: `{`, wantErr: ErrInvalidResponse},
		{name: "empty response", statusCode: http.StatusOK, body: `{"response":""}`, wantErr: ErrInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.statusCode)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()

			_, err := NewOllamaClient(server.URL, "llama3.2:1b", time.Second).Generate(context.Background(), "prompt")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Generate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestOllamaClientGenerateTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = response.Write([]byte(`{"response":"late"}`))
	}))
	defer server.Close()

	_, err := NewOllamaClient(server.URL, "llama3.2:1b", 10*time.Millisecond).Generate(context.Background(), "prompt")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Generate() error = %v, want deadline exceeded", err)
	}
}
