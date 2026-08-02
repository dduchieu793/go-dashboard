package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
)

var ErrInvalidInput = errors.New("invalid capability input")

const PromptVersion = "manual-summary-v1"

type textInput struct {
	Content string `json:"content"`
}

type summaryCapability struct {
	name        string
	summaryType summary.Type
	service     summary.Generator
}

func NewSummarizeText(service summary.Generator) Capability {
	return &summaryCapability{name: "summarize_text", summaryType: summary.TypeBrief, service: service}
}

func NewExtractActionItems(service summary.Generator) Capability {
	return &summaryCapability{name: "extract_action_items", summaryType: summary.TypeActionItems, service: service}
}

func (capability *summaryCapability) Name() string { return capability.name }

func (capability *summaryCapability) ValidateInput(input json.RawMessage) error {
	var decoded textInput
	if err := json.Unmarshal(input, &decoded); err != nil || strings.TrimSpace(decoded.Content) == "" {
		return ErrInvalidInput
	}
	return nil
}

func (capability *summaryCapability) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	if err := capability.ValidateInput(input); err != nil {
		return Result{}, err
	}
	var decoded textInput
	_ = json.Unmarshal(input, &decoded)
	generated, err := capability.service.Generate(ctx, summary.Request{Content: decoded.Content, SummaryType: capability.summaryType})
	if err != nil {
		return Result{}, err
	}
	content, err := json.Marshal(generated)
	if err != nil {
		return Result{}, fmt.Errorf("encode capability result: %w", err)
	}
	return Result{Content: content, ArtifactType: capability.name, Model: generated.Model, PromptVersion: PromptVersion}, nil
}

type ComposeDashboardResult struct{}

type composeInput struct {
	Summary     summary.Result `json:"summary"`
	ActionItems summary.Result `json:"action_items"`
}

type DashboardResult struct {
	Summary     string   `json:"summary"`
	ActionItems string   `json:"action_items"`
	Models      []string `json:"models"`
}

func (ComposeDashboardResult) Name() string { return "compose_dashboard_result" }

func (ComposeDashboardResult) ValidateInput(input json.RawMessage) error {
	var decoded composeInput
	if err := json.Unmarshal(input, &decoded); err != nil || strings.TrimSpace(decoded.Summary.Summary) == "" ||
		strings.TrimSpace(decoded.ActionItems.Summary) == "" {
		return ErrInvalidInput
	}
	return nil
}

func (capability ComposeDashboardResult) Execute(_ context.Context, input json.RawMessage) (Result, error) {
	if err := capability.ValidateInput(input); err != nil {
		return Result{}, err
	}
	var decoded composeInput
	_ = json.Unmarshal(input, &decoded)
	models := []string{decoded.Summary.Model}
	if decoded.ActionItems.Model != decoded.Summary.Model {
		models = append(models, decoded.ActionItems.Model)
	}
	content, err := json.Marshal(DashboardResult{
		Summary: decoded.Summary.Summary, ActionItems: decoded.ActionItems.Summary, Models: models,
	})
	if err != nil {
		return Result{}, fmt.Errorf("encode dashboard result: %w", err)
	}
	return Result{Content: content, ArtifactType: "dashboard_result"}, nil
}
