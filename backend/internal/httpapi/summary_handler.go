package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
)

const maxSummaryRequestBody = 256 << 10

type SummaryHandler struct {
	logger  *slog.Logger
	service summary.Generator
}

func NewSummaryHandler(logger *slog.Logger, service summary.Generator) *SummaryHandler {
	return &SummaryHandler{logger: logger, service: service}
}

func (handler *SummaryHandler) Generate(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, maxSummaryRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	var input summary.Request
	if err := decoder.Decode(&input); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeAPIError(response, http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large.")
			return
		}
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "The request body must be valid JSON.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "The request body must contain one JSON object.")
		return
	}

	result, err := handler.service.Generate(request.Context(), input)
	if err == nil {
		writeJSON(response, http.StatusOK, result)
		return
	}

	switch {
	case errors.Is(err, summary.ErrEmptyContent):
		writeAPIError(response, http.StatusBadRequest, "content_required", "Content is required.")
	case errors.Is(err, summary.ErrInvalidSummaryType):
		writeAPIError(response, http.StatusBadRequest, "invalid_summary_type", "Choose a supported summary type.")
	case errors.Is(err, summary.ErrContentTooLong):
		writeAPIError(response, http.StatusRequestEntityTooLarge, "content_too_long", "Content exceeds the 50,000 character limit.")
	case errors.Is(err, llm.ErrModelNotFound):
		writeAPIError(response, http.StatusServiceUnavailable, "model_unavailable", "The configured model is not installed.")
	case errors.Is(err, llm.ErrUnavailable):
		writeAPIError(response, http.StatusServiceUnavailable, "ollama_unavailable", "Ollama is unavailable.")
	case errors.Is(err, context.DeadlineExceeded):
		writeAPIError(response, http.StatusGatewayTimeout, "generation_timeout", "Summary generation timed out.")
	case errors.Is(err, context.Canceled):
		return
	case errors.Is(err, llm.ErrInvalidResponse):
		writeAPIError(response, http.StatusBadGateway, "invalid_model_response", "The model returned an invalid response.")
	default:
		handler.logger.ErrorContext(request.Context(), "generate summary", "error", err)
		writeAPIError(response, http.StatusInternalServerError, "internal_error", "Unable to generate a summary.")
	}
}

func writeAPIError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
