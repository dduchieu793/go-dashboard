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
	"github.com/dduchieu793/go-dashboard/backend/internal/modelrouter"
	"github.com/dduchieu793/go-dashboard/backend/internal/orchestration"
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

	profiles := make([]modelrouter.Profile, 0, len(cfg.ModelProfiles))
	for _, name := range []string{"general", "coding", "reasoning"} {
		profile := cfg.ModelProfiles[name]
		profiles = append(profiles, modelrouter.Profile{Name: name, Provider: profile.Provider, Model: profile.Model,
			Client: llm.NewOllamaClient(cfg.OllamaBaseURL, profile.Model, profile.Timeout, profile.KeepAlive)})
	}
	modelRouter, err := modelrouter.New(profiles, map[string]string{
		"summarize_text": "general", "classify_text": "general", "extract_action_items": "reasoning",
	})
	if err != nil {
		logger.Error("create model router", "error", err)
		os.Exit(1)
	}
	generalClient := modelRouter.Bind("summarize_text")
	summaryService := summary.NewService(generalClient)
	actionService := summary.NewService(modelRouter.Bind("extract_action_items"))
	store, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open workflow database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	capabilities := capability.NewRegistry()
	for _, registered := range []capability.Capability{
		capability.NewSummarizeText(summaryService),
		capability.NewClassifyText(modelRouter.Bind("classify_text")),
		capability.NewExtractActionItems(actionService),
		capability.ComposeDashboardResult{},
	} {
		if err := capabilities.Register(registered); err != nil {
			logger.Error("register capability", "error", err)
			os.Exit(1)
		}
	}
	manualSummary := workflow.ManualSummaryDefinition(cfg.OllamaTimeout)
	workflowRegistry := workflow.NewRegistry()
	if err := workflowRegistry.Register(manualSummary); err != nil {
		logger.Error("register workflow", "error", err)
		os.Exit(1)
	}
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
	workflowService := orchestration.NewService(
		orchestration.NewSelector(workflowRegistry, map[string]string{"manual_text": manualSummary.ID}),
		executor,
		manualSummary.ID,
	)
	router := httpapi.NewRouter(logger, cfg.FrontendOrigin, generalClient, summaryService, workflowService, httpapi.Catalogs{
		Models:       modelRouter,
		Capabilities: capabilities,
		Workflows:    workflowRegistry,
	})
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
