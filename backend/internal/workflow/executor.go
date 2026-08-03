package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/artifact"
	"github.com/dduchieu793/go-dashboard/backend/internal/capability"
	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
	"github.com/dduchieu793/go-dashboard/backend/internal/modelrouter"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
)

type RunStore interface {
	SaveDefinition(ctx context.Context, definition Definition) error
	CreateRun(ctx context.Context, run Run) error
	UpdateRun(ctx context.Context, run Run) error
	UpdateStep(ctx context.Context, step StepRun) error
	CreateArtifact(ctx context.Context, value artifact.Artifact) error
	CompleteStep(ctx context.Context, step StepRun, value artifact.Artifact) error
	GetRun(ctx context.Context, id string) (Run, error)
	ListRuns(ctx context.Context, limit int) ([]Run, error)
	PrepareRecovery(ctx context.Context) ([]Run, error)
	FindRunByRequestID(ctx context.Context, requestID string) (Run, bool, error)
	ClaimNextPendingRun(ctx context.Context) (Run, bool, error)
	ResetRunForRetry(ctx context.Context, id string) error
	CancelRun(ctx context.Context, id string) error
}

type Executor struct {
	definition Definition
	registry   *capability.Registry
	store      RunStore
	logger     *slog.Logger
	wake       chan struct{}

	mu           sync.Mutex
	started      bool
	workerCancel context.CancelFunc
	workerDone   chan struct{}
	active       *runControl
}

type runControl struct {
	runID         string
	cancel        context.CancelFunc
	userCancelled atomic.Bool
}

func NewExecutor(definition Definition, registry *capability.Registry, store RunStore, logger *slog.Logger) (*Executor, error) {
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	for _, step := range definition.Steps {
		if _, err := registry.Resolve(step.Capability); err != nil {
			return nil, fmt.Errorf("validate workflow capability %q: %w", step.Capability, err)
		}
	}
	return &Executor{definition: definition, registry: registry, store: store, logger: logger, wake: make(chan struct{}, 1)}, nil
}

func (executor *Executor) Initialize(ctx context.Context) error {
	return executor.store.SaveDefinition(ctx, executor.definition)
}

func (executor *Executor) StartWorker(ctx context.Context) error {
	executor.mu.Lock()
	if executor.started {
		executor.mu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	executor.started = true
	executor.workerCancel = cancel
	executor.workerDone = make(chan struct{})
	executor.mu.Unlock()

	runs, err := executor.store.PrepareRecovery(ctx)
	if err != nil {
		cancel()
		executor.mu.Lock()
		executor.started = false
		executor.mu.Unlock()
		return err
	}
	for _, run := range runs {
		executor.logger.InfoContext(ctx, "queued recovered workflow run", "workflow_run_id", run.ID)
	}
	go executor.worker(workerCtx)
	executor.signal()
	return nil
}

func (executor *Executor) Shutdown(ctx context.Context) error {
	executor.mu.Lock()
	if !executor.started {
		executor.mu.Unlock()
		return nil
	}
	cancel, done := executor.workerCancel, executor.workerDone
	executor.mu.Unlock()
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (executor *Executor) Start(ctx context.Context, request trigger.Request) (Run, error) {
	if err := request.Validate(); err != nil {
		return Run{}, err
	}
	if existing, found, err := executor.store.FindRunByRequestID(ctx, request.ID); err != nil {
		return Run{}, err
	} else if found {
		return existing, nil
	}
	if !executor.isStarted() {
		return Run{}, ErrExecutorNotRunning
	}
	createdAt := time.Now()
	run := Run{
		ID: NewID("run"), WorkflowID: executor.definition.ID, WorkflowVersion: executor.definition.Version,
		Request: request, Status: RunPending, CreatedAt: createdAt, Artifacts: make([]artifact.Artifact, 0),
	}
	for _, definition := range executor.definition.Steps {
		run.Steps = append(run.Steps, StepRun{
			ID: NewID("step"), WorkflowRunID: run.ID, StepID: definition.ID, Capability: definition.Capability,
			ModelProfile: definition.ModelProfile, Status: StepPending, CreatedAt: createdAt,
		})
	}
	if err := executor.store.CreateRun(ctx, run); err != nil {
		return Run{}, err
	}
	executor.signal()
	return run, nil
}

func (executor *Executor) GetRun(ctx context.Context, id string) (Run, error) {
	return executor.store.GetRun(ctx, id)
}

func (executor *Executor) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	return executor.store.ListRuns(ctx, limit)
}

func (executor *Executor) Retry(ctx context.Context, id string) (Run, error) {
	if !executor.isStarted() {
		return Run{}, ErrExecutorNotRunning
	}
	if err := executor.store.ResetRunForRetry(ctx, id); err != nil {
		return Run{}, err
	}
	executor.signal()
	return executor.store.GetRun(ctx, id)
}

func (executor *Executor) Cancel(ctx context.Context, id string) (Run, error) {
	if err := executor.store.CancelRun(ctx, id); err != nil {
		return Run{}, err
	}
	executor.mu.Lock()
	active := executor.active
	if active != nil && active.runID == id {
		active.userCancelled.Store(true)
		active.cancel()
	}
	executor.mu.Unlock()
	return executor.store.GetRun(ctx, id)
}

func (executor *Executor) isStarted() bool {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return executor.started
}

func (executor *Executor) signal() {
	select {
	case executor.wake <- struct{}{}:
	default:
	}
}

func (executor *Executor) worker(ctx context.Context) {
	defer func() {
		executor.mu.Lock()
		executor.started = false
		executor.active = nil
		executor.mu.Unlock()
		close(executor.workerDone)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-executor.wake:
		}
		for ctx.Err() == nil {
			run, found, err := executor.store.ClaimNextPendingRun(ctx)
			if err != nil {
				executor.logger.ErrorContext(ctx, "claim workflow run", "error", err)
				select {
				case <-time.After(250 * time.Millisecond):
					executor.signal()
				case <-ctx.Done():
				}
				break
			}
			if !found {
				break
			}
			runCtx, cancel := context.WithCancel(ctx)
			control := &runControl{runID: run.ID, cancel: cancel}
			executor.mu.Lock()
			executor.active = control
			executor.mu.Unlock()

			// A cancellation can race with claiming the pending run. Reload after
			// publishing the active control so either path observes it.
			latest, loadErr := executor.store.GetRun(runCtx, run.ID)
			if loadErr == nil && latest.Status == RunRunning {
				_, err = executor.execute(runCtx, latest, control)
			} else {
				err = loadErr
			}
			cancel()
			executor.mu.Lock()
			if executor.active == control {
				executor.active = nil
			}
			executor.mu.Unlock()
			if err != nil && ctx.Err() == nil && !errors.Is(err, ErrRunCancelled) {
				executor.logger.ErrorContext(ctx, "execute workflow run", "workflow_run_id", run.ID, "error", err)
			}
		}
	}
}

func (executor *Executor) execute(ctx context.Context, run Run, control *runControl) (Run, error) {
	runCtx, cancel := context.WithTimeout(ctx, executor.definition.Timeout)
	defer cancel()

	outputs := make(map[string]json.RawMessage, len(run.Steps))
	for index, stepDefinition := range executor.definition.Steps {
		step := &run.Steps[index]
		if step.Status == StepCompleted {
			outputs[step.StepID] = step.Output
			for _, value := range run.Artifacts {
				if value.StepRunID == step.ID {
					run.FinalArtifactID = value.ID
				}
			}
			continue
		}
		run.CurrentStepID = step.StepID
		if err := executor.store.UpdateRun(runCtx, run); err != nil {
			return Run{}, err
		}
		input, err := mapInput(run.Request, outputs, stepDefinition.InputMapping)
		if err != nil {
			return executor.failRun(runCtx, run, index, "input_mapping_failed", err)
		}
		step.Input = input
		resolved, _ := executor.registry.Resolve(step.Capability)
		if err := resolved.ValidateInput(input); err != nil {
			return executor.failRun(runCtx, run, index, "invalid_capability_input", err)
		}

		var result capability.Result
		var executeErr error
		for localAttempt := 1; localAttempt <= stepDefinition.RetryPolicy.MaxAttempts; localAttempt++ {
			attempt := step.Attempt + 1
			started := time.Now()
			step.Status, step.Attempt, step.StartedAt = StepRunning, attempt, &started
			step.ErrorCode, step.ErrorMessage = "", ""
			if err := executor.store.UpdateStep(runCtx, *step); err != nil {
				return Run{}, err
			}
			stepCtx, stepCancel := context.WithTimeout(runCtx, stepDefinition.Timeout)
			result, executeErr = resolved.Execute(modelrouter.WithProfile(stepCtx, step.ModelProfile), input)
			stepCancel()
			if runCtx.Err() != nil {
				if control.userCancelled.Load() {
					return executor.store.GetRun(context.WithoutCancel(runCtx), run.ID)
				}
				// Service shutdown leaves the run recoverable instead of turning an
				// interrupted local-model call into a terminal failure.
				if errors.Is(ctx.Err(), context.Canceled) {
					return Run{}, ctx.Err()
				}
			}
			if executeErr == nil {
				break
			}
			failedAt := time.Now()
			step.Status, step.CompletedAt = StepFailed, &failedAt
			step.ErrorCode, step.ErrorMessage = errorCategory(executeErr), safeErrorMessage(executeErr)
			if err := executor.store.UpdateStep(runCtx, *step); err != nil {
				return Run{}, err
			}
			if localAttempt < stepDefinition.RetryPolicy.MaxAttempts {
				select {
				case <-time.After(stepDefinition.RetryPolicy.Backoff):
				case <-runCtx.Done():
					executeErr = runCtx.Err()
				}
			}
		}
		if runCtx.Err() != nil {
			if control.userCancelled.Load() {
				return executor.store.GetRun(context.WithoutCancel(runCtx), run.ID)
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return Run{}, ctx.Err()
			}
		}
		if executeErr != nil {
			return executor.failRun(runCtx, run, index, errorCategory(executeErr), executeErr)
		}
		if control.userCancelled.Load() {
			return executor.store.GetRun(context.WithoutCancel(runCtx), run.ID)
		}

		completed := time.Now()
		step.Status, step.CompletedAt, step.Output, step.Model = StepCompleted, &completed, result.Content, result.Model
		value := artifact.Artifact{
			ID: NewID("artifact"), WorkflowRunID: run.ID, StepRunID: step.ID, Type: result.ArtifactType,
			Content: result.Content, Model: result.Model, PromptVersion: result.PromptVersion, CreatedAt: completed,
		}
		if err := executor.store.CompleteStep(runCtx, *step, value); err != nil {
			if errors.Is(err, ErrRunCancelled) {
				return executor.store.GetRun(context.WithoutCancel(runCtx), run.ID)
			}
			return Run{}, err
		}
		outputs[step.StepID] = result.Content
		run.FinalArtifactID = value.ID
		executor.logger.InfoContext(runCtx, "workflow step completed", "workflow_run_id", run.ID, "step_run_id", step.ID,
			"capability", step.Capability, "model", step.Model, "duration", completed.Sub(*step.StartedAt), "status", step.Status)
	}

	completed := time.Now()
	run.Status, run.CompletedAt, run.CurrentStepID = RunCompleted, &completed, ""
	if err := executor.store.UpdateRun(runCtx, run); err != nil {
		if errors.Is(err, ErrRunCancelled) {
			return executor.store.GetRun(context.WithoutCancel(runCtx), run.ID)
		}
		return Run{}, err
	}
	return executor.store.GetRun(ctx, run.ID)
}

func (executor *Executor) failRun(ctx context.Context, run Run, failedIndex int, code string, cause error) (Run, error) {
	completed := time.Now()
	run.Status, run.CompletedAt, run.ErrorCode, run.ErrorMessage = RunFailed, &completed, code, safeErrorMessage(cause)
	for index := failedIndex + 1; index < len(run.Steps); index++ {
		step := &run.Steps[index]
		step.Status, step.CompletedAt = StepSkipped, &completed
		_ = executor.store.UpdateStep(context.WithoutCancel(ctx), *step)
	}
	if err := executor.store.UpdateRun(context.WithoutCancel(ctx), run); err != nil {
		return Run{}, err
	}
	persisted, err := executor.store.GetRun(context.WithoutCancel(ctx), run.ID)
	if err != nil {
		return Run{}, err
	}
	return persisted, nil
}

func mapInput(request trigger.Request, outputs map[string]json.RawMessage, mapping map[string]string) (json.RawMessage, error) {
	result := make(map[string]any, len(mapping))
	for target, source := range mapping {
		switch {
		case source == "request.content":
			result[target] = request.Content
		case strings.HasPrefix(source, "steps."):
			stepID := strings.TrimPrefix(source, "steps.")
			content, exists := outputs[stepID]
			if !exists {
				return nil, fmt.Errorf("step output %q is unavailable", stepID)
			}
			var decoded any
			if err := json.Unmarshal(content, &decoded); err != nil {
				return nil, fmt.Errorf("decode step output %q: %w", stepID, err)
			}
			result[target] = decoded
		default:
			return nil, fmt.Errorf("unsupported input source %q", source)
		}
	}
	return json.Marshal(result)
}

func errorCategory(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, llm.ErrModelNotFound):
		return "model_unavailable"
	case errors.Is(err, llm.ErrUnavailable):
		return "provider_unavailable"
	case errors.Is(err, llm.ErrInvalidResponse):
		return "invalid_model_response"
	default:
		return "capability_failed"
	}
}

func safeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
