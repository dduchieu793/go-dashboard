package capability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
)

type classificationClient struct {
	content string
	err     error
}

func (client classificationClient) Status(context.Context) llm.Status { return llm.Status{} }
func (client classificationClient) Generate(context.Context, string) (llm.Generation, error) {
	return llm.Generation{Content: client.content, Model: "qwen3:4b"}, client.err
}

func TestClassifyText(t *testing.T) {
	capability := NewClassifyText(classificationClient{content: `{"category":"risk","confidence":0.85,"reason":"A release blocker is described."}`})
	result, err := capability.Execute(context.Background(), json.RawMessage(`{"content":"Security review is blocking release."}`))
	if err != nil {
		t.Fatal(err)
	}
	var got Classification
	if err := json.Unmarshal(result.Content, &got); err != nil {
		t.Fatal(err)
	}
	if got.Category != "risk" || got.Model != "qwen3:4b" || result.PromptVersion != ClassificationPromptVersion {
		t.Fatalf("classification = %+v, result = %+v", got, result)
	}
}

func TestClassifyTextRejectsInvalidOutput(t *testing.T) {
	for _, content := range []string{
		`{"category":"unknown","confidence":0.5,"reason":"bad"}`,
		`{"category":"risk","confidence":2,"reason":"bad"}`,
		`{"category":"risk","confidence":0.5,"reason":""}`,
	} {
		capability := NewClassifyText(classificationClient{content: content})
		if _, err := capability.Execute(context.Background(), json.RawMessage(`{"content":"text"}`)); !errors.Is(err, llm.ErrInvalidResponse) {
			t.Errorf("content %s: error = %v", content, err)
		}
	}
}
