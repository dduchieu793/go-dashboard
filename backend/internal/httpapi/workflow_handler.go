package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dduchieu793/go-dashboard/backend/internal/storage"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type WorkflowApplication interface {
	Start(ctx context.Context, request trigger.Request) (workflow.Run, error)
	GetRun(ctx context.Context, id string) (workflow.Run, error)
	ListRuns(ctx context.Context, limit int) ([]workflow.Run, error)
	Retry(ctx context.Context, id string) (workflow.Run, error)
	Cancel(ctx context.Context, id string) (workflow.Run, error)
}

type WorkflowHandler struct {
	logger      *slog.Logger
	application WorkflowApplication
}

func NewWorkflowHandler(logger *slog.Logger, application WorkflowApplication) *WorkflowHandler {
	return &WorkflowHandler{logger: logger, application: application}
}

func (handler *WorkflowHandler) StartManualSummary(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, maxSummaryRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input struct {
		Content string `json:"content"`
	}
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "The request body must be valid JSON.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "The request body must contain one JSON object.")
		return
	}
	if strings.TrimSpace(input.Content) == "" {
		writeAPIError(response, http.StatusBadRequest, "content_required", "Content is required.")
		return
	}
	normalized := trigger.Request{
		ID: workflow.NewID("request"), Source: "ui", Type: "manual_text", Content: strings.TrimSpace(input.Content),
		Metadata: map[string]string{}, ReceivedAt: time.Now(),
	}
	run, err := handler.application.Start(request.Context(), normalized)
	if err != nil {
		handler.logger.ErrorContext(request.Context(), "start workflow", "request_id", normalized.ID, "error", err)
		writeAPIError(response, http.StatusInternalServerError, "workflow_start_failed", "Unable to start the workflow.")
		return
	}
	writeJSON(response, http.StatusAccepted, run)
}

func (handler *WorkflowHandler) List(response http.ResponseWriter, request *http.Request) {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	runs, err := handler.application.ListRuns(request.Context(), limit)
	if err != nil {
		handler.logger.ErrorContext(request.Context(), "list workflow runs", "error", err)
		writeAPIError(response, http.StatusInternalServerError, "workflow_list_failed", "Unable to load workflow runs.")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"runs": runs})
}

func (handler *WorkflowHandler) Get(response http.ResponseWriter, request *http.Request) {
	run, err := handler.application.GetRun(request.Context(), chi.URLParam(request, "id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeAPIError(response, http.StatusNotFound, "workflow_run_not_found", "Workflow run not found.")
		return
	}
	if err != nil {
		handler.logger.ErrorContext(request.Context(), "get workflow run", "error", err)
		writeAPIError(response, http.StatusInternalServerError, "workflow_get_failed", "Unable to load the workflow run.")
		return
	}
	writeJSON(response, http.StatusOK, run)
}

func (handler *WorkflowHandler) Retry(response http.ResponseWriter, request *http.Request) {
	run, err := handler.application.Retry(request.Context(), chi.URLParam(request, "id"))
	handler.writeLifecycleResult(response, request, run, err, "retry")
}

func (handler *WorkflowHandler) Cancel(response http.ResponseWriter, request *http.Request) {
	run, err := handler.application.Cancel(request.Context(), chi.URLParam(request, "id"))
	handler.writeLifecycleResult(response, request, run, err, "cancel")
}

func (handler *WorkflowHandler) writeLifecycleResult(response http.ResponseWriter, request *http.Request, run workflow.Run, err error, operation string) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeAPIError(response, http.StatusNotFound, "workflow_run_not_found", "Workflow run not found.")
	case errors.Is(err, workflow.ErrRunNotRetryable), errors.Is(err, workflow.ErrRunNotCancellable):
		writeAPIError(response, http.StatusConflict, "workflow_state_conflict", "The workflow run cannot perform that operation in its current state.")
	case err != nil:
		handler.logger.ErrorContext(request.Context(), operation+" workflow run", "workflow_run_id", chi.URLParam(request, "id"), "error", err)
		writeAPIError(response, http.StatusInternalServerError, "workflow_"+operation+"_failed", "Unable to "+operation+" the workflow run.")
	default:
		writeJSON(response, http.StatusAccepted, run)
	}
}
