package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
)

type recordingSummaryGenerator struct {
	result  summary.Result
	err     error
	request summary.Request
}

func (generator *recordingSummaryGenerator) Generate(_ context.Context, request summary.Request) (summary.Result, error) {
	generator.request = request
	return generator.result, generator.err
}

func runSummaryRequest(generator summary.Generator, body string) *httptest.ResponseRecorder {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewSummaryHandler(logger, generator)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/summaries/generate", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.Generate(response, request)
	return response
}

func TestSummaryHandlerGenerate(t *testing.T) {
	generator := &recordingSummaryGenerator{result: summary.Result{
		Summary: "Generated summary", SummaryType: summary.TypeBrief, Model: "llama3.2:1b",
	}}
	response := runSummaryRequest(generator, `{"content":"source text","summary_type":"brief"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	if generator.request.Content != "source text" || generator.request.SummaryType != summary.TypeBrief {
		t.Errorf("service request = %+v", generator.request)
	}
	if !strings.Contains(response.Body.String(), `"summary":"Generated summary"`) {
		t.Errorf("body = %s", response.Body.String())
	}
}

func TestSummaryHandlerRejectsInvalidJSON(t *testing.T) {
	tests := []string{
		`{`,
		`{"content":"text","summary_type":"brief","unknown":true}`,
		`{"content":"text","summary_type":"brief"}{}`,
	}
	for _, body := range tests {
		response := runSummaryRequest(&recordingSummaryGenerator{}, body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, response.Code)
		}
	}
}

func TestSummaryHandlerMapsErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "empty", err: summary.ErrEmptyContent, status: http.StatusBadRequest, code: "content_required"},
		{name: "invalid type", err: summary.ErrInvalidSummaryType, status: http.StatusBadRequest, code: "invalid_summary_type"},
		{name: "content too long", err: summary.ErrContentTooLong, status: http.StatusRequestEntityTooLarge, code: "content_too_long"},
		{name: "model missing", err: llm.ErrModelNotFound, status: http.StatusServiceUnavailable, code: "model_unavailable"},
		{name: "ollama unavailable", err: llm.ErrUnavailable, status: http.StatusServiceUnavailable, code: "ollama_unavailable"},
		{name: "timeout", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: "generation_timeout"},
		{name: "invalid output", err: llm.ErrInvalidResponse, status: http.StatusBadGateway, code: "invalid_model_response"},
		{name: "unexpected", err: errors.New("unexpected"), status: http.StatusInternalServerError, code: "internal_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := runSummaryRequest(&recordingSummaryGenerator{err: test.err}, `{"content":"text","summary_type":"brief"}`)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Errorf("body = %s, want code %q", response.Body.String(), test.code)
			}
		})
	}
}

func TestSummaryHandlerRejectsOversizedBody(t *testing.T) {
	body := `{"content":"` + strings.Repeat("x", maxSummaryRequestBody) + `","summary_type":"brief"}`
	response := runSummaryRequest(&recordingSummaryGenerator{}, body)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.Code)
	}
}
