package llm

import "context"

type Status struct {
	Available      bool   `json:"available"`
	Model          string `json:"model"`
	ModelAvailable bool   `json:"model_available"`
}

type Client interface {
	Status(ctx context.Context) Status
}
