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
	t.Setenv("OLLAMA_GENERATE_TIMEOUT", "60s")
	t.Setenv("OLLAMA_KEEP_ALIVE", "-1m")
	t.Setenv("FRONTEND_ORIGIN", "http://localhost:5173")
	t.Setenv("DATABASE_PATH", "./data/test.db")
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
	if cfg.OllamaTimeout.String() != "1m0s" {
		t.Errorf("OllamaTimeout = %s, want 1m0s", cfg.OllamaTimeout)
	}
	if cfg.OllamaKeepAlive != "-1m" {
		t.Errorf("OllamaKeepAlive = %q, want -1m", cfg.OllamaKeepAlive)
	}
	if cfg.DatabasePath != "./data/test.db" {
		t.Errorf("DatabasePath = %q, want ./data/test.db", cfg.DatabasePath)
	}
	if cfg.FrontendOrigin != "http://localhost:5173" {
		t.Errorf("FrontendOrigin = %q, want origin without trailing slash", cfg.FrontendOrigin)
	}
	if cfg.ModelProfiles["general"].Model != "llama3.2:1b" ||
		cfg.ModelProfiles["coding"].Model != "qwen2.5-coder:7b" ||
		cfg.ModelProfiles["reasoning"].Model != "deepseek-r1:8b" {
		t.Errorf("ModelProfiles = %+v", cfg.ModelProfiles)
	}
	if cfg.ModelProfiles["reasoning"].KeepAlive != "0s" {
		t.Errorf("reasoning keep-alive = %q, want 0s", cfg.ModelProfiles["reasoning"].KeepAlive)
	}
}

func TestLoadUsesConfiguredModelProfiles(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("OLLAMA_CODING_MODEL", "coder-custom")
	t.Setenv("OLLAMA_REASONING_MODEL", "reasoning-custom")
	t.Setenv("OLLAMA_CODING_KEEP_ALIVE", "20m")
	t.Setenv("OLLAMA_REASONING_KEEP_ALIVE", "30s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelProfiles["coding"].Model != "coder-custom" || cfg.ModelProfiles["coding"].KeepAlive != "20m" ||
		cfg.ModelProfiles["reasoning"].Model != "reasoning-custom" || cfg.ModelProfiles["reasoning"].KeepAlive != "30s" {
		t.Fatalf("ModelProfiles = %+v", cfg.ModelProfiles)
	}
}

func TestLoadRejectsInvalidProfileKeepAlive(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("OLLAMA_REASONING_KEEP_ALIVE", "later")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OLLAMA_REASONING_KEEP_ALIVE") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadUsesStorageAndKeepAliveDefaults(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("OLLAMA_KEEP_ALIVE", "")
	t.Setenv("DATABASE_PATH", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OllamaKeepAlive != "-1m" || cfg.DatabasePath != "./data/dashboard.db" {
		t.Errorf("defaults = keepAlive %q, database %q", cfg.OllamaKeepAlive, cfg.DatabasePath)
	}
}

func TestLoadRejectsInvalidKeepAlive(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("OLLAMA_KEEP_ALIVE", "forever")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OLLAMA_KEEP_ALIVE") {
		t.Fatalf("Load() error = %v, want keep-alive validation error", err)
	}
}

func TestLoadUsesDefaultOllamaTimeout(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("OLLAMA_GENERATE_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OllamaTimeout.String() != "1m0s" {
		t.Errorf("OllamaTimeout = %s, want 1m0s", cfg.OllamaTimeout)
	}
}

func TestLoadRejectsInvalidOllamaTimeout(t *testing.T) {
	for _, timeout := range []string{"later", "0s", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("OLLAMA_GENERATE_TIMEOUT", timeout)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "OLLAMA_GENERATE_TIMEOUT") {
				t.Fatalf("Load() error = %v, want timeout validation error", err)
			}
		})
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
