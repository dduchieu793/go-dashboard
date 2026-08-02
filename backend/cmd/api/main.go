package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/capability"
	"github.com/dduchieu793/go-dashboard/backend/internal/config"
	"github.com/dduchieu793/go-dashboard/backend/internal/httpapi"
	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
	"github.com/dduchieu793/go-dashboard/backend/internal/storage"
	"github.com/dduchieu793/go-dashboard/backend/internal/summary"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	applicationCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ollamaClient := llm.NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaModel, cfg.OllamaTimeout, cfg.OllamaKeepAlive)
	summaryService := summary.NewService(ollamaClient)
	store, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open workflow database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	capabilities := capability.NewRegistry()
	for _, registered := range []capability.Capability{
		capability.NewSummarizeText(summaryService),
		capability.NewExtractActionItems(summaryService),
		capability.ComposeDashboardResult{},
	} {
		if err := capabilities.Register(registered); err != nil {
			logger.Error("register capability", "error", err)
			os.Exit(1)
		}
	}
	manualSummary := workflow.ManualSummaryDefinition(cfg.OllamaTimeout)
	executor, err := workflow.NewExecutor(manualSummary, capabilities, store, logger)
	if err != nil {
		logger.Error("create workflow executor", "error", err)
		os.Exit(1)
	}
	if err := executor.Initialize(context.Background()); err != nil {
		logger.Error("initialize workflows", "error", err)
		os.Exit(1)
	}
	if err := executor.StartWorker(applicationCtx); err != nil {
		logger.Error("start workflow worker", "error", err)
		os.Exit(1)
	}
	router := httpapi.NewRouter(logger, cfg.FrontendOrigin, ollamaClient, summaryService, executor)
	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      manualSummary.Timeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting", "environment", cfg.AppEnv, "address", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
			stop()
		}
	case <-applicationCtx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	if err := executor.Shutdown(shutdownCtx); err != nil {
		logger.Error("workflow worker shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("HTTP server stopped")
}
