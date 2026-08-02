package capability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
)

type fakeSummaryGenerator struct{}

func (fakeSummaryGenerator) Generate(_ context.Context, request summary.Request) (summary.Result, error) {
	return summary.Result{Summary: string(request.SummaryType), SummaryType: request.SummaryType, Model: "qwen3:4b"}, nil
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()
	registered := NewSummarizeText(fakeSummaryGenerator{})
	if err := registry.Register(registered); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(registered); !errors.Is(err, ErrDuplicateCapability) {
		t.Fatalf("duplicate Register() error = %v", err)
	}
	if _, err := registry.Resolve("missing"); !errors.Is(err, ErrUnknownCapability) {
		t.Fatalf("Resolve() error = %v", err)
	}
	if names := registry.Names(); len(names) != 1 || names[0] != "summarize_text" {
		t.Errorf("Names() = %v", names)
	}
}

func TestTextCapabilitiesAndComposition(t *testing.T) {
	input := json.RawMessage(`{"content":"source"}`)
	summaryResult, err := NewSummarizeText(fakeSummaryGenerator{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("summarize Execute() error = %v", err)
	}
	actionResult, err := NewExtractActionItems(fakeSummaryGenerator{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("actions Execute() error = %v", err)
	}
	composeInput, _ := json.Marshal(map[string]json.RawMessage{
		"summary": summaryResult.Content, "action_items": actionResult.Content,
	})
	final, err := (ComposeDashboardResult{}).Execute(context.Background(), composeInput)
	if err != nil {
		t.Fatalf("compose Execute() error = %v", err)
	}
	var dashboard DashboardResult
	if err := json.Unmarshal(final.Content, &dashboard); err != nil {
		t.Fatalf("decode dashboard result: %v", err)
	}
	if dashboard.Summary != "brief" || dashboard.ActionItems != "action_items" || len(dashboard.Models) != 1 {
		t.Errorf("dashboard result = %+v", dashboard)
	}
}
