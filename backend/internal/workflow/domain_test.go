package workflow

import (
	"errors"
	"testing"
	"time"
)

func validDefinition() Definition {
	return Definition{
		ID: "manual-summary", Name: "Manual Summary", Version: 1, Timeout: time.Minute, Enabled: true,
		Steps: []StepDefinition{
			{ID: "summarize", Capability: "summarize_text", Timeout: 30 * time.Second, RetryPolicy: RetryPolicy{MaxAttempts: 1}},
			{ID: "actions", Capability: "extract_action_items", Timeout: 30 * time.Second, RetryPolicy: RetryPolicy{MaxAttempts: 1}},
			{ID: "compose", Capability: "compose_dashboard_result", DependsOn: []string{"summarize", "actions"}, Timeout: time.Second, RetryPolicy: RetryPolicy{MaxAttempts: 1}},
		},
	}
}

func TestDefinitionValidate(t *testing.T) {
	if err := validDefinition().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDefinitionRejectsInvalidSteps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{name: "duplicate", mutate: func(definition *Definition) { definition.Steps[1].ID = "summarize" }},
		{name: "unknown dependency", mutate: func(definition *Definition) { definition.Steps[2].DependsOn = []string{"missing"} }},
		{name: "forward dependency", mutate: func(definition *Definition) { definition.Steps[0].DependsOn = []string{"compose"} }},
		{name: "duplicate dependency", mutate: func(definition *Definition) { definition.Steps[2].DependsOn = []string{"summarize", "summarize"} }},
		{name: "invalid retry", mutate: func(definition *Definition) { definition.Steps[0].RetryPolicy.MaxAttempts = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validDefinition()
			test.mutate(&definition)
			if err := definition.Validate(); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
			}
		})
	}
}

func TestStatusTransitions(t *testing.T) {
	if !RunPending.CanTransitionTo(RunRunning) || !RunFailed.CanTransitionTo(RunPending) || RunCompleted.CanTransitionTo(RunRunning) {
		t.Error("unexpected run transition rules")
	}
	if !StepPending.CanTransitionTo(StepRunning) || !StepFailed.CanTransitionTo(StepRunning) || StepCompleted.CanTransitionTo(StepRunning) {
		t.Error("unexpected step transition rules")
	}
}
