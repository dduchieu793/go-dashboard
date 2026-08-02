package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/artifact"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

func testDefinition() workflow.Definition {
	return workflow.Definition{
		ID: "manual-summary", Name: "Manual Summary", Version: 1, Timeout: time.Minute, Enabled: true,
		Steps: []workflow.StepDefinition{{
			ID: "summarize", Capability: "summarize_text", Timeout: time.Second,
			RetryPolicy: workflow.RetryPolicy{MaxAttempts: 1},
		}},
	}
}

func TestStorePersistsWorkflowRun(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	definition := testDefinition()
	if err := store.SaveDefinition(ctx, definition); err != nil {
		t.Fatalf("SaveDefinition() error = %v", err)
	}
	now := time.Now().Truncate(time.Millisecond)
	run := workflow.Run{
		ID: "run_1", WorkflowID: definition.ID, WorkflowVersion: definition.Version, Status: workflow.RunPending,
		Request:   trigger.Request{ID: "req_1", Source: "ui", Type: "manual_text", Content: "source", ReceivedAt: now},
		CreatedAt: now,
		Steps:     []workflow.StepRun{{ID: "step_1", WorkflowRunID: "run_1", StepID: "summarize", Capability: "summarize_text", Status: workflow.StepPending, CreatedAt: now}},
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	started := now.Add(time.Second)
	run.Status, run.StartedAt, run.CurrentStepID = workflow.RunRunning, &started, "summarize"
	if err := store.UpdateRun(ctx, run); err != nil {
		t.Fatalf("UpdateRun() error = %v", err)
	}
	step := run.Steps[0]
	step.Status, step.Attempt, step.StartedAt = workflow.StepRunning, 1, &started
	step.Input = json.RawMessage(`{"content":"source"}`)
	if err := store.UpdateStep(ctx, step); err != nil {
		t.Fatalf("UpdateStep() error = %v", err)
	}
	value := artifact.Artifact{ID: "artifact_1", WorkflowRunID: run.ID, StepRunID: step.ID, Type: "summary", Content: json.RawMessage(`{"summary":"result"}`), Model: "qwen3:4b", CreatedAt: now}
	if err := store.CreateArtifact(ctx, value); err != nil {
		t.Fatalf("CreateArtifact() error = %v", err)
	}

	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != workflow.RunRunning || len(got.Steps) != 1 || len(got.Artifacts) != 1 || got.Request.Content != "source" {
		t.Errorf("GetRun() = %+v", got)
	}
	byRequest, found, err := store.FindRunByRequestID(ctx, run.Request.ID)
	if err != nil || !found || byRequest.ID != run.ID {
		t.Fatalf("FindRunByRequestID() = %q, %t, %v", byRequest.ID, found, err)
	}
	if _, found, err := store.FindRunByRequestID(ctx, "missing"); err != nil || found {
		t.Fatalf("FindRunByRequestID(missing) found = %t, error = %v", found, err)
	}
	runs, err := store.ListRuns(ctx, 25)
	if err != nil || len(runs) != 1 {
		t.Fatalf("ListRuns() = %d, %v", len(runs), err)
	}
	recoverable, err := store.PrepareRecovery(ctx)
	if err != nil || len(recoverable) != 1 {
		t.Fatalf("PrepareRecovery() = %d, %v", len(recoverable), err)
	}
	if recoverable[0].Status != workflow.RunPending || recoverable[0].Steps[0].Status != workflow.StepPending || len(recoverable[0].Artifacts) != 0 {
		t.Errorf("recovered run = %+v", recoverable[0])
	}
	claimed, found, err := store.ClaimNextPendingRun(ctx)
	if err != nil || !found || claimed.Status != workflow.RunRunning {
		t.Fatalf("ClaimNextPendingRun() = %s, %t, %v", claimed.Status, found, err)
	}
	failedAt := now.Add(2 * time.Second)
	failedStep := claimed.Steps[0]
	failedStep.Status, failedStep.Attempt, failedStep.CompletedAt = workflow.StepFailed, 2, &failedAt
	failedStep.ErrorCode, failedStep.ErrorMessage = "timeout", "model timed out"
	if err := store.UpdateStep(ctx, failedStep); err != nil {
		t.Fatal(err)
	}
	claimed.Status, claimed.CompletedAt = workflow.RunFailed, &failedAt
	if err := store.UpdateRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetRunForRetry(ctx, claimed.ID); err != nil {
		t.Fatal(err)
	}
	retrying, err := store.GetRun(ctx, claimed.ID)
	if err != nil || retrying.Status != workflow.RunPending || retrying.Steps[0].Status != workflow.StepPending || retrying.Steps[0].Attempt != 2 {
		t.Fatalf("retrying run = %+v, %v", retrying, err)
	}
	claimed, found, err = store.ClaimNextPendingRun(ctx)
	if err != nil || !found {
		t.Fatalf("second ClaimNextPendingRun() found=%t error=%v", found, err)
	}
	if err := store.CancelRun(ctx, claimed.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.GetRun(ctx, claimed.ID)
	if err != nil || cancelled.Status != workflow.RunCancelled || cancelled.Steps[0].Status != workflow.StepCancelled {
		t.Fatalf("cancelled run = %+v, %v", cancelled, err)
	}
}

func TestStoreReturnsNotFound(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.GetRun(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRun() error = %v, want ErrNotFound", err)
	}
	if _, found, err := store.ClaimNextPendingRun(context.Background()); err != nil || found {
		t.Fatalf("ClaimNextPendingRun() found=%t error=%v", found, err)
	}
	if err := store.ResetRunForRetry(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResetRunForRetry() error = %v, want ErrNotFound", err)
	}
	if err := store.CancelRun(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CancelRun() error = %v, want ErrNotFound", err)
	}
}
