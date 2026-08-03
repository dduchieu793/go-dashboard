package orchestration

import (
	"context"
	"errors"
	"fmt"

	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

var (
	ErrSelectionRequired = errors.New("workflow selection is required")
	ErrWorkflowMismatch  = errors.New("selected workflow is not served by the configured executor")
)

type Selection struct {
	WorkflowID string `json:"workflow_id"`
	Method     string `json:"method"`
	Reason     string `json:"reason"`
}

type Selector struct {
	registry *workflow.Registry
	rules    map[string]string
}

func NewSelector(registry *workflow.Registry, rules map[string]string) *Selector {
	return &Selector{registry: registry, rules: rules}
}

func (selector *Selector) Select(request trigger.Request) (Selection, error) {
	if explicit := request.Metadata["workflow_id"]; explicit != "" {
		if _, err := selector.registry.Resolve(explicit, 0); err != nil {
			return Selection{}, err
		}
		return Selection{WorkflowID: explicit, Method: "explicit", Reason: "The caller selected this workflow."}, nil
	}
	if matched := selector.rules[request.Type]; matched != "" {
		if _, err := selector.registry.Resolve(matched, 0); err != nil {
			return Selection{}, err
		}
		return Selection{WorkflowID: matched, Method: "rule", Reason: "The normalized request type matched a configured rule."}, nil
	}
	return Selection{}, ErrSelectionRequired
}

type Engine interface {
	Start(ctx context.Context, request trigger.Request) (workflow.Run, error)
	GetRun(ctx context.Context, id string) (workflow.Run, error)
	ListRuns(ctx context.Context, limit int) ([]workflow.Run, error)
	Retry(ctx context.Context, id string) (workflow.Run, error)
	Cancel(ctx context.Context, id string) (workflow.Run, error)
}

type Service struct {
	selector   *Selector
	engine     Engine
	workflowID string
}

func NewService(selector *Selector, engine Engine, workflowID string) *Service {
	return &Service{selector: selector, engine: engine, workflowID: workflowID}
}

func (service *Service) Start(ctx context.Context, request trigger.Request) (workflow.Run, error) {
	selection, err := service.selector.Select(request)
	if err != nil {
		return workflow.Run{}, err
	}
	if selection.WorkflowID != service.workflowID {
		return workflow.Run{}, fmt.Errorf("%w: %s", ErrWorkflowMismatch, selection.WorkflowID)
	}
	if request.Metadata == nil {
		request.Metadata = make(map[string]string)
	}
	request.Metadata["selected_workflow"] = selection.WorkflowID
	request.Metadata["selection_method"] = selection.Method
	request.Metadata["selection_reason"] = selection.Reason
	return service.engine.Start(ctx, request)
}

func (service *Service) GetRun(ctx context.Context, id string) (workflow.Run, error) {
	return service.engine.GetRun(ctx, id)
}
func (service *Service) ListRuns(ctx context.Context, limit int) ([]workflow.Run, error) {
	return service.engine.ListRuns(ctx, limit)
}
func (service *Service) Retry(ctx context.Context, id string) (workflow.Run, error) {
	return service.engine.Retry(ctx, id)
}
func (service *Service) Cancel(ctx context.Context, id string) (workflow.Run, error) {
	return service.engine.Cancel(ctx, id)
}
