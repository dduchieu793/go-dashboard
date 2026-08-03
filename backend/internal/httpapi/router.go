package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
)

func NewRouter(logger *slog.Logger, frontendOrigin string, llmClient llm.Client, summaryService summary.Generator, workflowApplication WorkflowApplication, configuredCatalogs ...Catalogs) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(requestLogger(logger))
	router.Use(middleware.Recoverer)
	router.Use(cors(frontendOrigin))

	var catalogs Catalogs
	if len(configuredCatalogs) > 0 {
		catalogs = configuredCatalogs[0]
	}
	health := NewHealthHandler(llmClient, catalogs.Models)
	catalog := NewCatalogHandler(catalogs.Capabilities, catalogs.Workflows)
	router.Get("/health", health.Health)
	router.Get("/api/v1/system/llm-status", health.LLMStatus)
	router.Get("/api/v1/system/model-statuses", health.ModelStatuses)
	router.Get("/api/v1/capabilities", catalog.Capabilities)
	router.Get("/api/v1/workflows", catalog.Workflows)
	router.Post("/api/v1/summaries/generate", NewSummaryHandler(logger, summaryService).Generate)
	workflows := NewWorkflowHandler(logger, workflowApplication)
	router.Post("/api/v1/workflows/manual-summary/runs", workflows.StartManualSummary)
	router.Get("/api/v1/workflow-runs", workflows.List)
	router.Get("/api/v1/workflow-runs/{id}", workflows.Get)
	router.Post("/api/v1/workflow-runs/{id}/retry", workflows.Retry)
	router.Post("/api/v1/workflow-runs/{id}/cancel", workflows.Cancel)
	return router
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			started := time.Now()
			wrapped := middleware.NewWrapResponseWriter(response, request.ProtoMajor)
			next.ServeHTTP(wrapped, request)
			logger.InfoContext(request.Context(), "HTTP request",
				"method", request.Method,
				"path", request.URL.Path,
				"status", wrapped.Status(),
				"duration", time.Since(started),
				"request_id", middleware.GetReqID(request.Context()),
			)
		})
	}
}

func cors(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Origin") == allowedOrigin {
				response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				response.Header().Set("Vary", "Origin")
				response.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				response.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				if request.Method == http.MethodOptions {
					response.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(response, request)
		})
	}
}
