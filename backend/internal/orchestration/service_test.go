package orchestration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type engineStub struct{ request trigger.Request }

func (engine *engineStub) Start(_ context.Context, request trigger.Request) (workflow.Run, error) {
	engine.request = request
	return workflow.Run{ID: "run_1", Request: request}, nil
}
func (*engineStub) GetRun(context.Context, string) (workflow.Run, error)  { return workflow.Run{}, nil }
func (*engineStub) ListRuns(context.Context, int) ([]workflow.Run, error) { return nil, nil }
func (*engineStub) Retry(context.Context, string) (workflow.Run, error)   { return workflow.Run{}, nil }
func (*engineStub) Cancel(context.Context, string) (workflow.Run, error)  { return workflow.Run{}, nil }

func TestServiceSelectsAndAuditsWorkflow(t *testing.T) {
	registry := workflow.NewRegistry()
	definition := workflow.ManualSummaryDefinition(time.Second)
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	engine := &engineStub{}
	service := NewService(NewSelector(registry, map[string]string{"manual_text": definition.ID}), engine, definition.ID)
	request := trigger.Request{ID: "req_1", Source: "ui", Type: "manual_text", Content: "text", Metadata: map[string]string{}, ReceivedAt: time.Now()}
	if _, err := service.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if engine.request.Metadata["selected_workflow"] != definition.ID || engine.request.Metadata["selection_method"] != "rule" {
		t.Fatalf("selection metadata = %+v", engine.request.Metadata)
	}
	request.Type = "unknown"
	if _, err := service.Start(context.Background(), request); !errors.Is(err, ErrSelectionRequired) {
		t.Fatalf("unknown selection error = %v", err)
	}
}
