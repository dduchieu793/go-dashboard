package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/capability"
	"github.com/dduchieu793/go-dashboard/backend/internal/storage"
	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type staticCapability struct {
	name   string
	result capability.Result
	err    error
	calls  atomic.Int32
	mu     sync.RWMutex
}

func (item *staticCapability) Name() string { return item.name }
func (item *staticCapability) Metadata() capability.Metadata {
	return capability.Metadata{Name: item.name}
}
func (item *staticCapability) ValidateInput(json.RawMessage) error { return nil }
func (item *staticCapability) Execute(context.Context, json.RawMessage) (capability.Result, error) {
	item.calls.Add(1)
	item.mu.RLock()
	defer item.mu.RUnlock()
	return item.result, item.err
}

func (item *staticCapability) setError(err error) {
	item.mu.Lock()
	item.err = err
	item.mu.Unlock()
}

func setupExecutor(t *testing.T, summarize, actions capability.Capability) (*workflow.Executor, *storage.Store) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	registry := capability.NewRegistry()
	for _, item := range []capability.Capability{summarize, actions, capability.ComposeDashboardResult{}} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	executor, err := workflow.NewExecutor(workflow.ManualSummaryDefinition(time.Second), registry, store, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := executor.StartWorker(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := executor.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})
	return executor, store
}

func waitForRun(t *testing.T, executor *workflow.Executor, id string, statuses ...workflow.RunStatus) workflow.Run {
	t.Helper()
	wanted := make(map[workflow.RunStatus]bool, len(statuses))
	for _, status := range statuses {
		wanted[status] = true
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := executor.GetRun(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if wanted[run.Status] {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach statuses %v", id, statuses)
	return workflow.Run{}
}

func summaryArtifact(t *testing.T, summaryType summary.Type, text string) capability.Result {
	t.Helper()
	content, err := json.Marshal(summary.Result{Summary: text, SummaryType: summaryType, Model: "qwen3:4b"})
	if err != nil {
		t.Fatal(err)
	}
	return capability.Result{Content: content, ArtifactType: string(summaryType), Model: "qwen3:4b", PromptVersion: "v1"}
}

func TestExecutorCompletesAndPersistsWorkflow(t *testing.T) {
	summarize := &staticCapability{name: "summarize_text", result: summaryArtifact(t, summary.TypeBrief, "Summary")}
	actions := &staticCapability{name: "extract_action_items", result: summaryArtifact(t, summary.TypeActionItems, "Actions")}
	executor, _ := setupExecutor(t, summarize, actions)
	request := trigger.Request{
		ID: "request_1", Source: "ui", Type: "manual_text", Content: "source", ReceivedAt: time.Now(),
	}
	run, err := executor.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if run.Status != workflow.RunPending {
		t.Fatalf("Start() status = %s, want pending", run.Status)
	}
	if run.Artifacts == nil {
		t.Fatal("Start() artifacts must be an empty collection, not nil")
	}
	run = waitForRun(t, executor, run.ID, workflow.RunCompleted)
	if len(run.Steps) != 3 || len(run.Artifacts) != 3 || run.FinalArtifactID == "" {
		t.Fatalf("run = %+v", run)
	}
	for _, step := range run.Steps {
		if step.Status != workflow.StepCompleted {
			t.Errorf("step %s status = %s", step.StepID, step.Status)
		}
	}
	var final capability.DashboardResult
	if err := json.Unmarshal(run.Artifacts[2].Content, &final); err != nil {
		t.Fatal(err)
	}
	if final.Summary != "Summary" || final.ActionItems != "Actions" {
		t.Errorf("final = %+v", final)
	}

	repeated, err := executor.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("repeated Start() error = %v", err)
	}
	if repeated.ID != run.ID {
		t.Errorf("repeated run ID = %q, want %q", repeated.ID, run.ID)
	}
	if summarize.calls.Load() != 1 || actions.calls.Load() != 1 {
		t.Errorf("duplicate request executed capabilities again: summarize=%d actions=%d", summarize.calls.Load(), actions.calls.Load())
	}
}

func TestExecutorPersistsFailureAndSkipsRemainingSteps(t *testing.T) {
	failing := &staticCapability{name: "summarize_text", err: errors.New("generation failed")}
	executor, _ := setupExecutor(t, failing,
		&staticCapability{name: "extract_action_items", result: summaryArtifact(t, summary.TypeActionItems, "Actions")},
	)
	run, err := executor.Start(context.Background(), trigger.Request{
		ID: "request_1", Source: "ui", Type: "manual_text", Content: "source", ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	run = waitForRun(t, executor, run.ID, workflow.RunFailed)
	if run.Steps[0].Status != workflow.StepFailed ||
		run.Steps[1].Status != workflow.StepSkipped || run.Steps[2].Status != workflow.StepSkipped {
		t.Fatalf("failed run = %+v", run)
	}
	if failing.calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 attempts", failing.calls.Load())
	}
}

func TestExecutorRetriesOnlyFailedAndDownstreamSteps(t *testing.T) {
	summarize := &staticCapability{name: "summarize_text", result: summaryArtifact(t, summary.TypeBrief, "Summary")}
	actions := &staticCapability{name: "extract_action_items", result: summaryArtifact(t, summary.TypeActionItems, "Actions"), err: errors.New("generation failed")}
	executor, _ := setupExecutor(t, summarize, actions)
	run, err := executor.Start(context.Background(), trigger.Request{
		ID: "request_retry", Source: "ui", Type: "manual_text", Content: "source", ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run = waitForRun(t, executor, run.ID, workflow.RunFailed)
	if len(run.Artifacts) != 1 {
		t.Fatalf("failed run artifacts = %d, want completed upstream artifact", len(run.Artifacts))
	}
	actions.setError(nil)
	retrying, err := executor.Retry(context.Background(), run.ID)
	if err != nil || retrying.Status != workflow.RunPending {
		t.Fatalf("Retry() = %s, %v", retrying.Status, err)
	}
	completed := waitForRun(t, executor, run.ID, workflow.RunCompleted)
	if summarize.calls.Load() != 1 || actions.calls.Load() != 3 {
		t.Errorf("calls after retry: summarize=%d actions=%d", summarize.calls.Load(), actions.calls.Load())
	}
	if len(completed.Artifacts) != 3 {
		t.Errorf("artifacts after retry = %d, want 3 without duplicates", len(completed.Artifacts))
	}
	if completed.Steps[1].Attempt != 3 {
		t.Errorf("action attempts = %d, want cumulative attempt 3", completed.Steps[1].Attempt)
	}
}

type blockingCapability struct {
	name    string
	started chan struct{}
}

func (item *blockingCapability) Name() string { return item.name }
func (item *blockingCapability) Metadata() capability.Metadata {
	return capability.Metadata{Name: item.name}
}
func (item *blockingCapability) ValidateInput(json.RawMessage) error { return nil }
func (item *blockingCapability) Execute(ctx context.Context, _ json.RawMessage) (capability.Result, error) {
	select {
	case <-item.started:
	default:
		close(item.started)
	}
	<-ctx.Done()
	return capability.Result{}, ctx.Err()
}

func TestExecutorCancelsActiveRun(t *testing.T) {
	blocking := &blockingCapability{name: "summarize_text", started: make(chan struct{})}
	executor, _ := setupExecutor(t, blocking,
		&staticCapability{name: "extract_action_items", result: summaryArtifact(t, summary.TypeActionItems, "Actions")},
	)
	run, err := executor.Start(context.Background(), trigger.Request{
		ID: "request_cancel", Source: "ui", Type: "manual_text", Content: "source", ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("capability did not start")
	}
	cancelled, err := executor.Cancel(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != workflow.RunCancelled || cancelled.Steps[0].Status != workflow.StepCancelled {
		t.Fatalf("cancelled run = %+v", cancelled)
	}
	time.Sleep(20 * time.Millisecond)
	persisted, err := executor.GetRun(context.Background(), run.ID)
	if err != nil || persisted.Status != workflow.RunCancelled || len(persisted.Artifacts) != 0 {
		t.Fatalf("persisted cancellation = %+v, %v", persisted, err)
	}
}

type interruptOnceCapability struct {
	name    string
	started chan struct{}
	result  capability.Result
	calls   atomic.Int32
}

func (item *interruptOnceCapability) Name() string { return item.name }
func (item *interruptOnceCapability) Metadata() capability.Metadata {
	return capability.Metadata{Name: item.name}
}
func (item *interruptOnceCapability) ValidateInput(json.RawMessage) error { return nil }
func (item *interruptOnceCapability) Execute(ctx context.Context, _ json.RawMessage) (capability.Result, error) {
	if item.calls.Add(1) == 1 {
		close(item.started)
		<-ctx.Done()
		return capability.Result{}, ctx.Err()
	}
	return item.result, nil
}

func TestExecutorRecoversInterruptedActiveRun(t *testing.T) {
	interruptOnce := &interruptOnceCapability{
		name: "summarize_text", started: make(chan struct{}), result: summaryArtifact(t, summary.TypeBrief, "Summary"),
	}
	executor, _ := setupExecutor(t, interruptOnce,
		&staticCapability{name: "extract_action_items", result: summaryArtifact(t, summary.TypeActionItems, "Actions")},
	)
	run, err := executor.Start(context.Background(), trigger.Request{
		ID: "request_recovery", Source: "ui", Type: "manual_text", Content: "source", ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-interruptOnce.started:
	case <-time.After(time.Second):
		t.Fatal("capability did not start")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := executor.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	interrupted, err := executor.GetRun(context.Background(), run.ID)
	if err != nil || interrupted.Status != workflow.RunRunning || interrupted.Steps[0].Status != workflow.StepRunning {
		t.Fatalf("interrupted run = %+v, %v", interrupted, err)
	}
	if err := executor.StartWorker(context.Background()); err != nil {
		t.Fatal(err)
	}
	completed := waitForRun(t, executor, run.ID, workflow.RunCompleted)
	if completed.Steps[0].Attempt != 2 || len(completed.Artifacts) != 3 {
		t.Fatalf("recovered run = %+v", completed)
	}
}
