package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
)

type HealthHandler struct {
	llmClient llm.Client
}

func NewHealthHandler(llmClient llm.Client) *HealthHandler {
	return &HealthHandler{llmClient: llmClient}
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
