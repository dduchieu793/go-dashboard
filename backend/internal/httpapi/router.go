package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
)

func NewRouter(logger *slog.Logger, frontendOrigin string, llmClient llm.Client) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(requestLogger(logger))
	router.Use(middleware.Recoverer)
	router.Use(cors(frontendOrigin))

	health := NewHealthHandler(llmClient)
	router.Get("/health", health.Health)
	router.Get("/api/v1/system/llm-status", health.LLMStatus)
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
			}
			next.ServeHTTP(response, request)
		})
	}
}
