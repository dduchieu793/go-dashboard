package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
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

	required := []struct {
		name  string
		value string
	}{
		{name: "APP_ENV", value: cfg.AppEnv},
		{name: "HTTP_PORT", value: cfg.HTTPPort},
		{name: "OLLAMA_BASE_URL", value: cfg.OllamaBaseURL},
		{name: "OLLAMA_MODEL", value: cfg.OllamaModel},
		{name: "FRONTEND_ORIGIN", value: cfg.FrontendOrigin},
	}
	var missing []string
	for _, variable := range required {
		if variable.value == "" {
			missing = append(missing, variable.name)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("required environment variables are missing: %s", strings.Join(missing, ", "))
	}
	port, err := strconv.Atoi(cfg.HTTPPort)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, errors.New("HTTP_PORT must be an integer between 1 and 65535")
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
