package trigger

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidRequest = errors.New("invalid normalized request")

type Request struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Type       string            `json:"type"`
	Content    string            `json:"content"`
	Metadata   map[string]string `json:"metadata"`
	ReceivedAt time.Time         `json:"received_at"`
}

func (request Request) Validate() error {
	if strings.TrimSpace(request.ID) == "" || strings.TrimSpace(request.Source) == "" ||
		strings.TrimSpace(request.Type) == "" || strings.TrimSpace(request.Content) == "" || request.ReceivedAt.IsZero() {
		return ErrInvalidRequest
	}
	return nil
}
