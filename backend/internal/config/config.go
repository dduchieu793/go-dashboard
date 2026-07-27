package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	AppEnv         string
	HTTPPort       string
	OllamaBaseURL  string
	OllamaModel    string
	FrontendOrigin string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:         strings.TrimSpace(os.Getenv("APP_ENV")),
		HTTPPort:       strings.TrimSpace(os.Getenv("HTTP_PORT")),
		OllamaBaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL")), "/"),
		OllamaModel:    strings.TrimSpace(os.Getenv("OLLAMA_MODEL")),
		FrontendOrigin: strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")), "/"),
	}

	required := map[string]string{
		"APP_ENV":         cfg.AppEnv,
		"HTTP_PORT":       cfg.HTTPPort,
		"OLLAMA_BASE_URL": cfg.OllamaBaseURL,
		"OLLAMA_MODEL":    cfg.OllamaModel,
		"FRONTEND_ORIGIN": cfg.FrontendOrigin,
	}
	var missing []string
	for name, value := range required {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("required environment variables are missing: %s", strings.Join(missing, ", "))
	}
	if err := validateHTTPURL("OLLAMA_BASE_URL", cfg.OllamaBaseURL); err != nil {
		return Config{}, err
	}
	if err := validateHTTPURL("FRONTEND_ORIGIN", cfg.FrontendOrigin); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateHTTPURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be a valid HTTP(S) URL", name)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New(name + " must not contain a query or fragment")
	}
	return nil
}
