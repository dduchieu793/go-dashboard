package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	slacksource "github.com/dduchieu793/go-dashboard/backend/internal/slack"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

const maxSlackEventBody = 1 << 20

type SlackApplication interface {
	HandleEvent(context.Context, slacksource.EventEnvelope) (workflow.Run, bool, error)
	ListThreads(context.Context, int) ([]slacksource.Thread, error)
	GetThread(context.Context, string) (slacksource.Thread, error)
	ListMessages(context.Context, string) ([]slacksource.Message, error)
	ListAttachments(context.Context, string) ([]slacksource.Attachment, error)
	Refresh(context.Context, string) (slacksource.Thread, workflow.Run, error)
	Analyze(context.Context, string) (workflow.Run, error)
}

type SlackHandler struct {
	logger        *slog.Logger
	signingSecret string
	application   SlackApplication
	now           func() time.Time
}

func NewSlackHandler(logger *slog.Logger, signingSecret string, application SlackApplication) *SlackHandler {
	return &SlackHandler{logger: logger, signingSecret: signingSecret, application: application, now: time.Now}
}

func (handler *SlackHandler) Events(response http.ResponseWriter, request *http.Request) {
	if handler.signingSecret == "" || handler.application == nil {
		writeAPIError(response, http.StatusServiceUnavailable, "slack_not_configured", "Slack ingestion is not configured.")
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxSlackEventBody)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_slack_event", "Unable to read the Slack event.")
		return
	}
	if !VerifySlackSignature(handler.signingSecret, request.Header.Get("X-Slack-Request-Timestamp"),
		request.Header.Get("X-Slack-Signature"), body, handler.now()) {
		writeAPIError(response, http.StatusUnauthorized, "invalid_slack_signature", "Slack request signature is invalid.")
		return
	}
	var envelope slacksource.EventEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_slack_event", "The Slack event must be valid JSON.")
		return
	}
	if envelope.Type == "url_verification" {
		writeJSON(response, http.StatusOK, map[string]string{"challenge": envelope.Challenge})
		return
	}
	run, started, err := handler.application.HandleEvent(request.Context(), envelope)
	if err != nil {
		handler.logger.ErrorContext(request.Context(), "process Slack event", "event_id", envelope.EventID, "error", err)
		writeAPIError(response, http.StatusInternalServerError, "slack_event_failed", "Unable to process the Slack event.")
		return
	}
	result := map[string]any{"ok": true, "workflow_started": started}
	if started {
		result["workflow_run_id"] = run.ID
	}
	writeJSON(response, http.StatusOK, result)
}

func (handler *SlackHandler) ListThreads(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	threads, err := handler.application.ListThreads(request.Context(), limit)
	if err != nil {
		handler.writeError(response, request, err, "list Slack threads")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"threads": threads})
}

func (handler *SlackHandler) GetThread(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	thread, err := handler.application.GetThread(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		handler.writeError(response, request, err, "get Slack thread")
		return
	}
	writeJSON(response, http.StatusOK, thread)
}

func (handler *SlackHandler) ListMessages(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	messages, err := handler.application.ListMessages(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		handler.writeError(response, request, err, "list Slack messages")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"messages": messages})
}

func (handler *SlackHandler) ListAttachments(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	attachments, err := handler.application.ListAttachments(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		handler.writeError(response, request, err, "list Slack attachments")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"attachments": attachments})
}

func (handler *SlackHandler) Refresh(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	thread, run, err := handler.application.Refresh(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		handler.writeError(response, request, err, "refresh Slack thread")
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"thread": thread, "workflow_run": nullableRun(run)})
}

func (handler *SlackHandler) Analyze(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	run, err := handler.application.Analyze(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		handler.writeError(response, request, err, "analyze Slack thread")
		return
	}
	writeJSON(response, http.StatusAccepted, run)
}

func (handler *SlackHandler) available(response http.ResponseWriter) bool {
	if handler.application != nil {
		return true
	}
	writeAPIError(response, http.StatusServiceUnavailable, "slack_not_configured", "Slack ingestion is not configured.")
	return false
}

func (handler *SlackHandler) writeError(response http.ResponseWriter, request *http.Request, err error, operation string) {
	if errors.Is(err, slacksource.ErrNotFound) {
		writeAPIError(response, http.StatusNotFound, "slack_thread_not_found", "Slack thread not found.")
		return
	}
	handler.logger.ErrorContext(request.Context(), operation, "error", err)
	writeAPIError(response, http.StatusInternalServerError, "slack_operation_failed", "Unable to complete the Slack operation.")
}

func VerifySlackSignature(secret, timestamp, signature string, body []byte, now time.Time) bool {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || strings.TrimSpace(secret) == "" || !strings.HasPrefix(signature, "v0=") {
		return false
	}
	requestTime := time.Unix(seconds, 0)
	if now.Sub(requestTime) > 5*time.Minute || requestTime.Sub(now) > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func nullableRun(run workflow.Run) any {
	if run.ID == "" {
		return nil
	}
	return run
}
