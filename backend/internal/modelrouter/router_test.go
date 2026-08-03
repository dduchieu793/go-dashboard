package modelrouter

import (
	"context"
	"errors"
	"testing"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
)

type fakeClient struct{ model string }

func (client fakeClient) Status(context.Context) llm.Status {
	return llm.Status{Available: true, Model: client.model, ModelAvailable: true}
}
func (client fakeClient) Generate(context.Context, string) (llm.Generation, error) {
	return llm.Generation{Content: `{}`, Model: client.model}, nil
}

func TestRouterResolvesMappingAndOverride(t *testing.T) {
	router, err := New([]Profile{
		{Name: "general", Provider: "ollama", Model: "qwen", Client: fakeClient{model: "qwen"}},
		{Name: "reasoning", Provider: "ollama", Model: "deepseek", Client: fakeClient{model: "deepseek"}},
	}, map[string]string{"summarize_text": "general"})
	if err != nil {
		t.Fatal(err)
	}
	client := router.Bind("summarize_text")
	generation, err := client.Generate(context.Background(), "prompt")
	if err != nil || generation.Model != "qwen" {
		t.Fatalf("default generation = %+v, %v", generation, err)
	}
	generation, err = client.Generate(WithProfile(context.Background(), "reasoning"), "prompt")
	if err != nil || generation.Model != "deepseek" {
		t.Fatalf("override generation = %+v, %v", generation, err)
	}
	statuses := router.Statuses(context.Background())
	if len(statuses) != 2 || statuses[0].Name != "general" || len(statuses[0].Capabilities) != 1 {
		t.Fatalf("statuses = %+v", statuses)
	}
}

func TestRouterRejectsUnknownProfilesAndMappings(t *testing.T) {
	if _, err := New(nil, map[string]string{"summary": "missing"}); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("New() error = %v", err)
	}
	router, err := New([]Profile{{Name: "general", Provider: "ollama", Model: "qwen", Client: fakeClient{model: "qwen"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := router.Resolve("missing", ""); !errors.Is(err, ErrUnknownMapping) {
		t.Fatalf("Resolve() error = %v", err)
	}
}
