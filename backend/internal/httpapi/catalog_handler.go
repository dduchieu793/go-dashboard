package httpapi

import (
	"net/http"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/capability"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type CapabilityCatalog interface {
	Metadata() []capability.Metadata
}

type WorkflowCatalog interface {
	Enabled() []workflow.Definition
}

type Catalogs struct {
	Models             ModelCatalog
	Capabilities       CapabilityCatalog
	Workflows          WorkflowCatalog
	Slack              SlackApplication
	SlackSigningSecret string
}

type CatalogHandler struct {
	capabilities CapabilityCatalog
	workflows    WorkflowCatalog
}

type WorkflowMetadata struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     int                    `json:"version"`
	Timeout     string                 `json:"timeout"`
	Enabled     bool                   `json:"enabled"`
	Steps       []WorkflowStepMetadata `json:"steps"`
}

type WorkflowStepMetadata struct {
	ID           string   `json:"id"`
	Capability   string   `json:"capability"`
	ModelProfile string   `json:"model_profile,omitempty"`
	DependsOn    []string `json:"depends_on"`
	Timeout      string   `json:"timeout"`
}

func NewCatalogHandler(capabilities CapabilityCatalog, workflows WorkflowCatalog) *CatalogHandler {
	return &CatalogHandler{capabilities: capabilities, workflows: workflows}
}

func (handler *CatalogHandler) Capabilities(response http.ResponseWriter, _ *http.Request) {
	if handler.capabilities == nil {
		writeJSON(response, http.StatusOK, map[string]any{"capabilities": []capability.Metadata{}})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"capabilities": handler.capabilities.Metadata()})
}

func (handler *CatalogHandler) Workflows(response http.ResponseWriter, _ *http.Request) {
	if handler.workflows == nil {
		writeJSON(response, http.StatusOK, map[string]any{"workflows": []WorkflowMetadata{}})
		return
	}
	definitions := handler.workflows.Enabled()
	items := make([]WorkflowMetadata, 0, len(definitions))
	for _, definition := range definitions {
		steps := make([]WorkflowStepMetadata, 0, len(definition.Steps))
		for _, step := range definition.Steps {
			dependencies := append([]string{}, step.DependsOn...)
			steps = append(steps, WorkflowStepMetadata{ID: step.ID, Capability: step.Capability,
				ModelProfile: step.ModelProfile, DependsOn: dependencies, Timeout: formatDuration(step.Timeout)})
		}
		items = append(items, WorkflowMetadata{ID: definition.ID, Name: definition.Name,
			Description: definition.Description, Version: definition.Version,
			Timeout: formatDuration(definition.Timeout), Enabled: definition.Enabled, Steps: steps})
	}
	writeJSON(response, http.StatusOK, map[string]any{"workflows": items})
}

func formatDuration(duration time.Duration) string { return duration.String() }
