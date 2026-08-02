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

	"github.com/go-chi/chi/v5"

	"github.com/dduchieu793/go-dashboard/backend/internal/storage"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type recordingWorkflowApplication struct {
	run       workflow.Run
	runs      []workflow.Run
	err       error
	request   trigger.Request
	operation string
}

func (application *recordingWorkflowApplication) Start(_ context.Context, request trigger.Request) (workflow.Run, error) {
	application.request = request
	return application.run, application.err
}

func (application *recordingWorkflowApplication) GetRun(context.Context, string) (workflow.Run, error) {
	return application.run, application.err
}

func (application *recordingWorkflowApplication) ListRuns(context.Context, int) ([]workflow.Run, error) {
	return application.runs, application.err
}

func (application *recordingWorkflowApplication) Retry(context.Context, string) (workflow.Run, error) {
	application.operation = "retry"
	return application.run, application.err
}

func (application *recordingWorkflowApplication) Cancel(context.Context, string) (workflow.Run, error) {
	application.operation = "cancel"
	return application.run, application.err
}

func workflowTestRouter(application WorkflowApplication) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewWorkflowHandler(logger, application)
	router := chi.NewRouter()
	router.Post("/runs", handler.StartManualSummary)
	router.Get("/runs", handler.List)
	router.Get("/runs/{id}", handler.Get)
	router.Post("/runs/{id}/retry", handler.Retry)
	router.Post("/runs/{id}/cancel", handler.Cancel)
	return router
}

func TestWorkflowHandlerStartsManualRun(t *testing.T) {
	application := &recordingWorkflowApplication{run: workflow.Run{ID: "run_1", Status: workflow.RunCompleted}}
	request := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"content":" source "}`))
	response := httptest.NewRecorder()
	workflowTestRouter(application).ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if application.request.Content != "source" || application.request.Source != "ui" || application.request.ID == "" {
		t.Errorf("normalized request = %+v", application.request)
	}
}

func TestWorkflowHandlerRetriesAndCancelsRuns(t *testing.T) {
	for _, operation := range []string{"retry", "cancel"} {
		application := &recordingWorkflowApplication{run: workflow.Run{ID: "run_1", Status: workflow.RunPending}}
		request := httptest.NewRequest(http.MethodPost, "/runs/run_1/"+operation, nil)
		response := httptest.NewRecorder()
		workflowTestRouter(application).ServeHTTP(response, request)
		if response.Code != http.StatusAccepted || application.operation != operation {
			t.Errorf("%s: status=%d operation=%q body=%s", operation, response.Code, application.operation, response.Body.String())
		}
	}
}

func TestWorkflowHandlerRejectsInvalidLifecycleOperations(t *testing.T) {
	for _, test := range []struct {
		err  error
		want int
	}{
		{err: storage.ErrNotFound, want: http.StatusNotFound},
		{err: workflow.ErrRunNotRetryable, want: http.StatusConflict},
		{err: errors.New("boom"), want: http.StatusInternalServerError},
	} {
		request := httptest.NewRequest(http.MethodPost, "/runs/run_1/retry", nil)
		response := httptest.NewRecorder()
		workflowTestRouter(&recordingWorkflowApplication{err: test.err}).ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("error %v: status=%d want=%d", test.err, response.Code, test.want)
		}
	}
}

func TestWorkflowHandlerListsAndGetsRuns(t *testing.T) {
	application := &recordingWorkflowApplication{run: workflow.Run{ID: "run_1"}, runs: []workflow.Run{{ID: "run_1"}}}
	for _, path := range []string{"/runs", "/runs/run_1"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		workflowTestRouter(application).ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "run_1") {
			t.Errorf("GET %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestWorkflowHandlerValidationAndNotFound(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"content":" "}`))
	response := httptest.NewRecorder()
	workflowTestRouter(&recordingWorkflowApplication{}).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Errorf("blank content status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/runs/missing", nil)
	response = httptest.NewRecorder()
	workflowTestRouter(&recordingWorkflowApplication{err: storage.ErrNotFound}).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("missing status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"content":"text"}`))
	response = httptest.NewRecorder()
	workflowTestRouter(&recordingWorkflowApplication{err: errors.New("boom")}).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Errorf("start error status = %d", response.Code)
	}
}
