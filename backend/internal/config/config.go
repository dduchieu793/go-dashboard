package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                string
	HTTPPort              string
	OllamaBaseURL         string
	OllamaModel           string
	OllamaTimeout         time.Duration
	OllamaKeepAlive       string
	FrontendOrigin        string
	DatabasePath          string
	ModelProfiles         map[string]ModelProfile
	Slack                 SlackConfig
	AttachmentStoragePath string
}

type SlackConfig struct {
	SigningSecret      string
	BotToken           string
	APIBaseURL         string
	RequestTimeout     time.Duration
	MaxContextMessages int
	MaxContextChars    int
}

func (config SlackConfig) Enabled() bool { return config.SigningSecret != "" && config.BotToken != "" }

type ModelProfile struct {
	Provider  string
	Model     string
	Timeout   time.Duration
	KeepAlive string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:         strings.TrimSpace(os.Getenv("APP_ENV")),
		HTTPPort:       strings.TrimSpace(os.Getenv("HTTP_PORT")),
		OllamaBaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL")), "/"),
		OllamaModel:    strings.TrimSpace(os.Getenv("OLLAMA_MODEL")),
		FrontendOrigin: strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")), "/"),
		DatabasePath:   strings.TrimSpace(os.Getenv("DATABASE_PATH")),
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
	timeoutValue := strings.TrimSpace(os.Getenv("OLLAMA_GENERATE_TIMEOUT"))
	if timeoutValue == "" {
		timeoutValue = "60s"
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil || timeout <= 0 {
		return Config{}, errors.New("OLLAMA_GENERATE_TIMEOUT must be a positive duration")
	}
	cfg.OllamaTimeout = timeout
	cfg.OllamaKeepAlive = strings.TrimSpace(os.Getenv("OLLAMA_KEEP_ALIVE"))
	if cfg.OllamaKeepAlive == "" {
		cfg.OllamaKeepAlive = "-1m"
	}
	if _, err := time.ParseDuration(cfg.OllamaKeepAlive); err != nil {
		return Config{}, errors.New("OLLAMA_KEEP_ALIVE must be a duration such as 30m, -1m, or 0s")
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = "./data/dashboard.db"
	}
	codingKeepAlive, err := durationSetting("OLLAMA_CODING_KEEP_ALIVE", "15m")
	if err != nil {
		return Config{}, err
	}
	reasoningKeepAlive, err := durationSetting("OLLAMA_REASONING_KEEP_ALIVE", "0s")
	if err != nil {
		return Config{}, err
	}
	codingModel := strings.TrimSpace(os.Getenv("OLLAMA_CODING_MODEL"))
	if codingModel == "" {
		codingModel = "qwen2.5-coder:7b"
	}
	reasoningModel := strings.TrimSpace(os.Getenv("OLLAMA_REASONING_MODEL"))
	if reasoningModel == "" {
		reasoningModel = "deepseek-r1:8b"
	}
	cfg.ModelProfiles = map[string]ModelProfile{
		"general":   {Provider: "ollama", Model: cfg.OllamaModel, Timeout: timeout, KeepAlive: cfg.OllamaKeepAlive},
		"coding":    {Provider: "ollama", Model: codingModel, Timeout: timeout, KeepAlive: codingKeepAlive},
		"reasoning": {Provider: "ollama", Model: reasoningModel, Timeout: timeout, KeepAlive: reasoningKeepAlive},
	}
	cfg.Slack.SigningSecret = strings.TrimSpace(os.Getenv("SLACK_SIGNING_SECRET"))
	cfg.Slack.BotToken = strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN"))
	cfg.Slack.APIBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("SLACK_API_BASE_URL")), "/")
	if cfg.Slack.APIBaseURL == "" {
		cfg.Slack.APIBaseURL = "https://slack.com/api"
	}
	if err := validateHTTPURL("SLACK_API_BASE_URL", cfg.Slack.APIBaseURL); err != nil {
		return Config{}, err
	}
	if (cfg.Slack.SigningSecret == "") != (cfg.Slack.BotToken == "") {
		return Config{}, errors.New("SLACK_SIGNING_SECRET and SLACK_BOT_TOKEN must be configured together")
	}
	slackTimeout, err := positiveDurationSetting("SLACK_REQUEST_TIMEOUT", "15s")
	if err != nil {
		return Config{}, err
	}
	cfg.Slack.RequestTimeout = slackTimeout
	cfg.Slack.MaxContextMessages, err = positiveIntSetting("SLACK_MAX_CONTEXT_MESSAGES", 200)
	if err != nil {
		return Config{}, err
	}
	cfg.Slack.MaxContextChars, err = positiveIntSetting("SLACK_MAX_CONTEXT_CHARS", 50000)
	if err != nil {
		return Config{}, err
	}
	if cfg.Slack.MaxContextChars < 1000 {
		return Config{}, errors.New("SLACK_MAX_CONTEXT_CHARS must be at least 1000")
	}
	cfg.AttachmentStoragePath = strings.TrimSpace(os.Getenv("ATTACHMENT_STORAGE_PATH"))
	if cfg.AttachmentStoragePath == "" {
		cfg.AttachmentStoragePath = "./data/attachments"
	}
	return cfg, nil
}

func durationSetting(name, fallback string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		value = fallback
	}
	if _, err := time.ParseDuration(value); err != nil {
		return "", fmt.Errorf("%s must be a duration", name)
	}
	return value, nil
}

func positiveDurationSetting(name, fallback string) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		value = fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func positiveIntSetting(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
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
