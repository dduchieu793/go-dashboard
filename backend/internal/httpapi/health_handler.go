package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
	"github.com/dduchieu793/go-dashboard/backend/internal/modelrouter"
)

type ModelCatalog interface {
	Statuses(ctx context.Context) []modelrouter.Status
}

type HealthHandler struct {
	llmClient llm.Client
	catalog   ModelCatalog
}

func NewHealthHandler(llmClient llm.Client, catalogs ...ModelCatalog) *HealthHandler {
	handler := &HealthHandler{llmClient: llmClient}
	if len(catalogs) > 0 && catalogs[0] != nil {
		handler.catalog = catalogs[0]
	}
	return handler
}

func (h *HealthHandler) ModelStatuses(response http.ResponseWriter, request *http.Request) {
	if h.catalog == nil {
		writeJSON(response, http.StatusOK, map[string]any{"profiles": []modelrouter.Status{}})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"profiles": h.catalog.Statuses(request.Context())})
}

func (h *HealthHandler) Health(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) LLMStatus(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, http.StatusOK, h.llmClient.Status(request.Context()))
}

func writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}
