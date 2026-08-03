package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/capability"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type capabilityCatalogStub struct{ items []capability.Metadata }

func (catalog capabilityCatalogStub) Metadata() []capability.Metadata { return catalog.items }

func TestCatalogHandlerReturnsCapabilities(t *testing.T) {
	handler := NewCatalogHandler(capabilityCatalogStub{items: []capability.Metadata{{Name: "classify_text"}}}, nil)
	response := httptest.NewRecorder()
	handler.Capabilities(response, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
	var body struct {
		Capabilities []capability.Metadata `json:"capabilities"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(body.Capabilities) != 1 || body.Capabilities[0].Name != "classify_text" {
		t.Fatalf("response = %d %+v", response.Code, body)
	}
}

func TestCatalogHandlerReturnsReadableWorkflowMetadata(t *testing.T) {
	registry := workflow.NewRegistry()
	definition := workflow.ManualSummaryDefinition(90 * time.Second)
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	handler := NewCatalogHandler(nil, registry)
	response := httptest.NewRecorder()
	handler.Workflows(response, httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil))
	var body struct {
		Workflows []WorkflowMetadata `json:"workflows"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(body.Workflows) != 1 {
		t.Fatalf("response = %d %+v", response.Code, body)
	}
	if body.Workflows[0].Timeout != definition.Timeout.String() || body.Workflows[0].Steps[0].Timeout != "1m30s" {
		t.Fatalf("workflow durations = %+v", body.Workflows[0])
	}
}
