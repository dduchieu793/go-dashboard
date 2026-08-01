package config

import (
	"strings"
	"testing"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_PORT", "8080")
	t.Setenv("OLLAMA_BASE_URL", "http://localhost:11434")
	t.Setenv("OLLAMA_MODEL", "llama3.2:1b")
	t.Setenv("FRONTEND_ORIGIN", "http://localhost:5173")
}

func TestLoad(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("APP_ENV", " development ")
	t.Setenv("HTTP_PORT", " 8080 ")
	t.Setenv("OLLAMA_BASE_URL", " http://localhost:11434/ ")
	t.Setenv("OLLAMA_MODEL", " llama3.2:1b ")
	t.Setenv("FRONTEND_ORIGIN", " http://localhost:5173/ ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.HTTPPort != "8080" {
		t.Errorf("HTTPPort = %q, want 8080", cfg.HTTPPort)
	}
	if cfg.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("OllamaBaseURL = %q, want URL without trailing slash", cfg.OllamaBaseURL)
	}
	if cfg.OllamaModel != "llama3.2:1b" {
		t.Errorf("OllamaModel = %q, want llama3.2:1b", cfg.OllamaModel)
	}
	if cfg.FrontendOrigin != "http://localhost:5173" {
		t.Errorf("FrontendOrigin = %q, want origin without trailing slash", cfg.FrontendOrigin)
	}
}

func TestLoadReportsMissingVariablesInStableOrder(t *testing.T) {
	for _, name := range []string{
		"APP_ENV",
		"HTTP_PORT",
		"OLLAMA_BASE_URL",
		"OLLAMA_MODEL",
		"FRONTEND_ORIGIN",
	} {
		t.Setenv(name, "")
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing variables error")
	}

	const want = "required environment variables are missing: APP_ENV, HTTP_PORT, OLLAMA_BASE_URL, OLLAMA_MODEL, FRONTEND_ORIGIN"
	if err.Error() != want {
		t.Errorf("Load() error = %q, want %q", err, want)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	tests := []string{"not-a-port", "0", "-1", "65536"}
	for _, port := range tests {
		t.Run(port, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("HTTP_PORT", port)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "HTTP_PORT") {
				t.Fatalf("Load() error = %v, want HTTP_PORT validation error", err)
			}
		})
	}
}

func TestLoadRejectsInvalidURLs(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{name: "unsupported scheme", variable: "OLLAMA_BASE_URL", value: "ftp://localhost:11434"},
		{name: "missing host", variable: "OLLAMA_BASE_URL", value: "http:///api"},
		{name: "query", variable: "OLLAMA_BASE_URL", value: "http://localhost:11434?debug=true"},
		{name: "fragment", variable: "FRONTEND_ORIGIN", value: "http://localhost:5173#app"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(test.variable, test.value)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("Load() error = %v, want %s validation error", err, test.variable)
			}
		})
	}
}
