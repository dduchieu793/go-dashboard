package trigger

import (
	"errors"
	"testing"
	"time"
)

func TestRequestValidate(t *testing.T) {
	request := Request{ID: "req_1", Source: "ui", Type: "manual_text", Content: "text", ReceivedAt: time.Now()}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	request.Content = " "
	if err := request.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRequest", err)
	}
}
