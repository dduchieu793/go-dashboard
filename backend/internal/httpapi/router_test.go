package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type stubLLMClient struct {
	status llm.Status
}

func (client stubLLMClient) Status(context.Context) llm.Status {
	return client.status
}

func (client stubLLMClient) Generate(context.Context, string) (llm.Generation, error) {
	return llm.Generation{}, nil
}

type stubSummaryGenerator struct{}

func (stubSummaryGenerator) Generate(context.Context, summary.Request) (summary.Result, error) {
	return summary.Result{}, nil
}

type stubWorkflowApplication struct{}

func (stubWorkflowApplication) Start(context.Context, trigger.Request) (workflow.Run, error) {
	return workflow.Run{}, nil
}

func (stubWorkflowApplication) GetRun(context.Context, string) (workflow.Run, error) {
	return workflow.Run{}, nil
}

func (stubWorkflowApplication) ListRuns(context.Context, int) ([]workflow.Run, error) {
	return nil, nil
}

func (stubWorkflowApplication) Retry(context.Context, string) (workflow.Run, error) {
	return workflow.Run{}, nil
}

func (stubWorkflowApplication) Cancel(context.Context, string) (workflow.Run, error) {
	return workflow.Run{}, nil
}

func testRouter(status llm.Status) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(logger, "http://localhost:5173", stubLLMClient{status: status}, stubSummaryGenerator{}, stubWorkflowApplication{})
}

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	testRouter(llm.Status{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("response status = %q, want ok", body["status"])
	}
}

func TestLLMStatus(t *testing.T) {
	want := llm.Status{
		Available:      true,
		Model:          "llama3.2:1b",
		ModelAvailable: false,
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/llm-status", nil)
	response := httptest.NewRecorder()

	testRouter(want).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got llm.Status
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got != want {
		t.Errorf("response = %+v, want %+v", got, want)
	}
}

func TestCORS(t *testing.T) {
	t.Run("allowed origin", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		request.Header.Set("Origin", "http://localhost:5173")
		response := httptest.NewRecorder()

		testRouter(llm.Status{}).ServeHTTP(response, request)

		if origin := response.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:5173" {
			t.Errorf("Access-Control-Allow-Origin = %q, want configured origin", origin)
		}
		if vary := response.Header().Get("Vary"); !strings.Contains(vary, "Origin") {
			t.Errorf("Vary = %q, want Origin", vary)
		}
	})

	t.Run("disallowed origin", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		request.Header.Set("Origin", "https://example.com")
		response := httptest.NewRecorder()

		testRouter(llm.Status{}).ServeHTTP(response, request)

		if origin := response.Header().Get("Access-Control-Allow-Origin"); origin != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty", origin)
		}
	})

	t.Run("preflight", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodOptions, "/api/v1/summaries/generate", nil)
		request.Header.Set("Origin", "http://localhost:5173")
		request.Header.Set("Access-Control-Request-Method", http.MethodPost)
		response := httptest.NewRecorder()

		testRouter(llm.Status{}).ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Errorf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if methods := response.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, http.MethodPost) {
			t.Errorf("Access-Control-Allow-Methods = %q, want POST", methods)
		}
		if headers := response.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(headers, "Content-Type") {
			t.Errorf("Access-Control-Allow-Headers = %q, want Content-Type", headers)
		}
	})
}
