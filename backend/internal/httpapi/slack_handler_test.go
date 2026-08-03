package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	slacksource "github.com/dduchieu793/go-dashboard/backend/internal/slack"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type slackApplicationStub struct{ handled bool }

func (application *slackApplicationStub) HandleEvent(context.Context, slacksource.EventEnvelope) (workflow.Run, bool, error) {
	application.handled = true
	return workflow.Run{ID: "run_1"}, true, nil
}
func (*slackApplicationStub) ListThreads(context.Context, int) ([]slacksource.Thread, error) {
	return nil, nil
}
func (*slackApplicationStub) GetThread(context.Context, string) (slacksource.Thread, error) {
	return slacksource.Thread{}, nil
}
func (*slackApplicationStub) ListMessages(context.Context, string) ([]slacksource.Message, error) {
	return nil, nil
}
func (*slackApplicationStub) ListAttachments(context.Context, string) ([]slacksource.Attachment, error) {
	return nil, nil
}
func (*slackApplicationStub) Refresh(context.Context, string) (slacksource.Thread, workflow.Run, error) {
	return slacksource.Thread{}, workflow.Run{}, nil
}
func (*slackApplicationStub) Analyze(context.Context, string) (workflow.Run, error) {
	return workflow.Run{}, nil
}

func slackSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestSlackEventsVerifiesSignatureAndChallenge(t *testing.T) {
	now := time.Unix(1710000000, 0)
	handler := NewSlackHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), "secret", &slackApplicationStub{})
	handler.now = func() time.Time { return now }
	body := []byte(`{"type":"url_verification","challenge":"answer"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(string(body)))
	timestamp := "1710000000"
	request.Header.Set("X-Slack-Request-Timestamp", timestamp)
	request.Header.Set("X-Slack-Signature", slackSignature("secret", timestamp, body))
	response := httptest.NewRecorder()
	handler.Events(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "answer") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(`{}`))
	request.Header.Set("X-Slack-Request-Timestamp", timestamp)
	request.Header.Set("X-Slack-Signature", "v0=wrong")
	response = httptest.NewRecorder()
	handler.Events(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid signature status = %d", response.Code)
	}
}

func TestSlackEventsStartsWorkflow(t *testing.T) {
	now := time.Unix(1710000000, 0)
	application := &slackApplicationStub{}
	handler := NewSlackHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), "secret", application)
	handler.now = func() time.Time { return now }
	body := []byte(`{"type":"event_callback","team_id":"T1","event_id":"Ev1","event":{"type":"message"}}`)
	timestamp := "1710000000"
	request := httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(string(body)))
	request.Header.Set("X-Slack-Request-Timestamp", timestamp)
	request.Header.Set("X-Slack-Signature", slackSignature("secret", timestamp, body))
	response := httptest.NewRecorder()
	handler.Events(response, request)
	if response.Code != http.StatusOK || !application.handled || !strings.Contains(response.Body.String(), "run_1") {
		t.Fatalf("response = %d %s handled=%t", response.Code, response.Body.String(), application.handled)
	}
}

func TestVerifySlackSignatureRejectsReplay(t *testing.T) {
	now := time.Unix(1710001000, 0)
	body := []byte(`{}`)
	timestamp := "1710000000"
	if VerifySlackSignature("secret", timestamp, slackSignature("secret", timestamp, body), body, now) {
		t.Fatal("stale Slack request must be rejected")
	}
}
