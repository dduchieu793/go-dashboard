package summary

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
)

type fakeProvider struct {
	generation llm.Generation
	err        error
	prompt     string
	calls      int
}

func (provider *fakeProvider) Status(context.Context) llm.Status { return llm.Status{} }

func (provider *fakeProvider) Generate(_ context.Context, prompt string) (llm.Generation, error) {
	provider.calls++
	provider.prompt = prompt
	return provider.generation, provider.err
}

func TestServiceGenerate(t *testing.T) {
	provider := &fakeProvider{generation: llm.Generation{
		Content: `{"summary":" A concise result. "}`,
		Model:   "llama3.2:1b",
	}}
	service := NewService(provider)

	result, err := service.Generate(context.Background(), Request{
		Content: "  Important source text.  ", SummaryType: TypeBrief,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Summary != "A concise result." || result.Model != "llama3.2:1b" || result.SummaryType != TypeBrief {
		t.Errorf("Generate() result = %+v", result)
	}
	if !strings.Contains(provider.prompt, "<content>\nImportant source text.\n</content>") {
		t.Errorf("prompt does not contain trimmed, delimited content: %q", provider.prompt)
	}
}

func TestServiceGenerateValidation(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		wantErr error
	}{
		{name: "empty content", request: Request{Content: "  ", SummaryType: TypeBrief}, wantErr: ErrEmptyContent},
		{name: "long content", request: Request{Content: strings.Repeat("x", MaxContentLength+1), SummaryType: TypeBrief}, wantErr: ErrContentTooLong},
		{name: "invalid type", request: Request{Content: "text", SummaryType: "unknown"}, wantErr: ErrInvalidSummaryType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeProvider{}
			_, err := NewService(provider).Generate(context.Background(), test.request)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Generate() error = %v, want %v", err, test.wantErr)
			}
			if provider.calls != 0 {
				t.Errorf("provider calls = %d, want 0", provider.calls)
			}
		})
	}
}

func TestServiceGenerateProviderAndOutputErrors(t *testing.T) {
	tests := []struct {
		name        string
		generation  llm.Generation
		providerErr error
		wantErr     error
	}{
		{name: "provider error", providerErr: llm.ErrUnavailable, wantErr: llm.ErrUnavailable},
		{name: "malformed JSON", generation: llm.Generation{Content: "not-json"}, wantErr: llm.ErrInvalidResponse},
		{name: "missing summary", generation: llm.Generation{Content: `{}`}, wantErr: llm.ErrInvalidResponse},
		{name: "blank summary", generation: llm.Generation{Content: `{"summary":" "}`}, wantErr: llm.ErrInvalidResponse},
		{name: "unknown output field", generation: llm.Generation{Content: `{"summary":"ok","extra":true}`}, wantErr: llm.ErrInvalidResponse},
		{name: "multiple objects", generation: llm.Generation{Content: `{"summary":"ok"}{}`}, wantErr: llm.ErrInvalidResponse},
		{name: "oversized output", generation: llm.Generation{Content: strings.Repeat("x", maxGeneratedContentLength+1)}, wantErr: llm.ErrInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeProvider{generation: test.generation, err: test.providerErr}
			_, err := NewService(provider).Generate(context.Background(), Request{Content: "text", SummaryType: TypeDetailed})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Generate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestBuildPromptForEverySummaryType(t *testing.T) {
	for _, summaryType := range []Type{TypeBrief, TypeDetailed, TypeActionItems} {
		t.Run(string(summaryType), func(t *testing.T) {
			prompt := buildPrompt("source", summaryType)
			if !strings.Contains(prompt, "source") || !strings.Contains(prompt, `{"summary":"your summary"}`) {
				t.Errorf("buildPrompt() = %q", prompt)
			}
		})
	}
}
