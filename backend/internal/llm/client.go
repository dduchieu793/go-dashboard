package llm

import (
	"context"
	"errors"
)

var (
	ErrUnavailable     = errors.New("LLM provider unavailable")
	ErrModelNotFound   = errors.New("configured model not found")
	ErrInvalidResponse = errors.New("invalid LLM response")
)

type Status struct {
	Available      bool   `json:"available"`
	Model          string `json:"model"`
	ModelAvailable bool   `json:"model_available"`
}

type Client interface {
	Status(ctx context.Context) Status
	Generate(ctx context.Context, prompt string) (Generation, error)
}

type Generation struct {
	Content string
	Model   string
}
