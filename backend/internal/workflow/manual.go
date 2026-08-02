package workflow

import "time"

func ManualSummaryDefinition(modelTimeout time.Duration) Definition {
	stepTimeout := modelTimeout
	if stepTimeout <= 0 {
		stepTimeout = 60 * time.Second
	}
	return Definition{
		ID:          "manual-summary",
		Name:        "Manual Summary",
		Description: "Summarize manual text, extract action items, and compose a dashboard result.",
		Version:     1,
		// Each of the two model-backed steps may consume both attempts.
		Timeout:     4*stepTimeout + 15*time.Second,
		RetryPolicy: RetryPolicy{MaxAttempts: 1},
		Enabled:     true,
		Steps: []StepDefinition{
			{
				ID: "summarize", Capability: "summarize_text", ModelProfile: "general",
				InputMapping: map[string]string{"content": "request.content"}, Timeout: stepTimeout,
				RetryPolicy: RetryPolicy{MaxAttempts: 2, Backoff: 250 * time.Millisecond},
			},
			{
				ID: "actions", Capability: "extract_action_items", ModelProfile: "general",
				InputMapping: map[string]string{"content": "request.content"}, Timeout: stepTimeout,
				RetryPolicy: RetryPolicy{MaxAttempts: 2, Backoff: 250 * time.Millisecond},
			},
			{
				ID: "compose", Capability: "compose_dashboard_result",
				InputMapping: map[string]string{"summary": "steps.summarize", "action_items": "steps.actions"},
				DependsOn:    []string{"summarize", "actions"}, Timeout: 5 * time.Second,
				RetryPolicy: RetryPolicy{MaxAttempts: 1},
			},
		},
	}
}
