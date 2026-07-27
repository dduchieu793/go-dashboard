package llm

import "context"

type Status struct {
	Available bool   `json:"available"`
	Model     string `json:"model,omitempty"`
}

type Client interface {
	Status(ctx context.Context) Status
}
