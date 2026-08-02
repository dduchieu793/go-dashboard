package artifact

import (
	"encoding/json"
	"time"
)

type Artifact struct {
	ID            string          `json:"id"`
	WorkflowRunID string          `json:"workflow_run_id"`
	StepRunID     string          `json:"step_run_id"`
	Type          string          `json:"type"`
	Content       json.RawMessage `json:"content"`
	Model         string          `json:"model,omitempty"`
	PromptVersion string          `json:"prompt_version,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}
