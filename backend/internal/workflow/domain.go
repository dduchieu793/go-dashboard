package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/artifact"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
)

var (
	ErrInvalidDefinition  = errors.New("invalid workflow definition")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrExecutorNotRunning = errors.New("workflow executor is not running")
	ErrRunNotRetryable    = errors.New("workflow run is not retryable")
	ErrRunNotCancellable  = errors.New("workflow run is not cancellable")
	ErrRunCancelled       = errors.New("workflow run was cancelled")
)

type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	Backoff     time.Duration `json:"backoff"`
}

type StepDefinition struct {
	ID               string            `json:"id"`
	Capability       string            `json:"capability"`
	InputMapping     map[string]string `json:"input_mapping"`
	DependsOn        []string          `json:"depends_on"`
	ModelProfile     string            `json:"model_profile,omitempty"`
	Timeout          time.Duration     `json:"timeout"`
	RetryPolicy      RetryPolicy       `json:"retry_policy"`
	RequiresApproval bool              `json:"requires_approval"`
}

type Definition struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Version      int              `json:"version"`
	InputSchema  json.RawMessage  `json:"input_schema"`
	Steps        []StepDefinition `json:"steps"`
	OutputSchema json.RawMessage  `json:"output_schema"`
	Timeout      time.Duration    `json:"timeout"`
	RetryPolicy  RetryPolicy      `json:"retry_policy"`
	Enabled      bool             `json:"enabled"`
}

func (definition Definition) Validate() error {
	if strings.TrimSpace(definition.ID) == "" || strings.TrimSpace(definition.Name) == "" ||
		definition.Version < 1 || definition.Timeout <= 0 || len(definition.Steps) == 0 {
		return fmt.Errorf("%w: missing required definition fields", ErrInvalidDefinition)
	}
	stepIndexes := make(map[string]int, len(definition.Steps))
	for index, step := range definition.Steps {
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.Capability) == "" || step.Timeout <= 0 {
			return fmt.Errorf("%w: invalid step at index %d", ErrInvalidDefinition, index)
		}
		if step.RetryPolicy.MaxAttempts < 1 || step.RetryPolicy.Backoff < 0 {
			return fmt.Errorf("%w: invalid retry policy for step %q", ErrInvalidDefinition, step.ID)
		}
		if _, exists := stepIndexes[step.ID]; exists {
			return fmt.Errorf("%w: duplicate step ID %q", ErrInvalidDefinition, step.ID)
		}
		stepIndexes[step.ID] = index
	}
	for index, step := range definition.Steps {
		seenDependencies := map[string]bool{}
		for _, dependency := range step.DependsOn {
			dependencyIndex, exists := stepIndexes[dependency]
			if !exists {
				return fmt.Errorf("%w: step %q depends on unknown step %q", ErrInvalidDefinition, step.ID, dependency)
			}
			if dependencyIndex >= index {
				return fmt.Errorf("%w: step %q has cyclic or forward dependency %q", ErrInvalidDefinition, step.ID, dependency)
			}
			if seenDependencies[dependency] {
				return fmt.Errorf("%w: step %q repeats dependency %q", ErrInvalidDefinition, step.ID, dependency)
			}
			seenDependencies[dependency] = true
		}
	}
	return nil
}

type RunStatus string

const (
	RunPending         RunStatus = "pending"
	RunRunning         RunStatus = "running"
	RunWaitingApproval RunStatus = "waiting_approval"
	RunCompleted       RunStatus = "completed"
	RunFailed          RunStatus = "failed"
	RunCancelled       RunStatus = "cancelled"
)

type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
	StepCancelled StepStatus = "cancelled"
)

type Run struct {
	ID              string              `json:"id"`
	WorkflowID      string              `json:"workflow_id"`
	WorkflowVersion int                 `json:"workflow_version"`
	Request         trigger.Request     `json:"request"`
	Status          RunStatus           `json:"status"`
	CurrentStepID   string              `json:"current_step_id,omitempty"`
	FinalArtifactID string              `json:"final_artifact_id,omitempty"`
	ErrorCode       string              `json:"error_code,omitempty"`
	ErrorMessage    string              `json:"error_message,omitempty"`
	StartedAt       *time.Time          `json:"started_at,omitempty"`
	CompletedAt     *time.Time          `json:"completed_at,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	Steps           []StepRun           `json:"steps"`
	Artifacts       []artifact.Artifact `json:"artifacts"`
}

type StepRun struct {
	ID            string          `json:"id"`
	WorkflowRunID string          `json:"workflow_run_id"`
	StepID        string          `json:"step_id"`
	Capability    string          `json:"capability"`
	ModelProfile  string          `json:"model_profile,omitempty"`
	Model         string          `json:"model,omitempty"`
	Status        StepStatus      `json:"status"`
	Attempt       int             `json:"attempt"`
	Input         json.RawMessage `json:"input,omitempty"`
	Output        json.RawMessage `json:"output,omitempty"`
	ErrorCode     string          `json:"error_code,omitempty"`
	ErrorMessage  string          `json:"error_message,omitempty"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

func (status RunStatus) CanTransitionTo(next RunStatus) bool {
	allowed := map[RunStatus]map[RunStatus]bool{
		RunPending:         {RunRunning: true, RunCancelled: true},
		RunRunning:         {RunWaitingApproval: true, RunCompleted: true, RunFailed: true, RunCancelled: true},
		RunWaitingApproval: {RunRunning: true, RunCancelled: true},
		RunFailed:          {RunPending: true},
	}
	return allowed[status][next]
}

func (status StepStatus) CanTransitionTo(next StepStatus) bool {
	allowed := map[StepStatus]map[StepStatus]bool{
		StepPending: {StepRunning: true, StepSkipped: true, StepCancelled: true},
		StepRunning: {StepCompleted: true, StepFailed: true, StepCancelled: true},
		StepFailed:  {StepRunning: true},
	}
	return allowed[status][next]
}

func NewID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		panic("generate workflow ID: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(buffer)
}
