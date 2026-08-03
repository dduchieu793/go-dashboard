package workflow

import (
	"errors"
	"testing"
	"time"
)

func TestRegistryResolvesLatestEnabledVersion(t *testing.T) {
	registry := NewRegistry()
	first := ManualSummaryDefinition(time.Second)
	second := first
	second.Version = 2
	if err := registry.Register(first); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(second); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(first.ID, 0)
	if err != nil || resolved.Version != 2 || len(registry.Enabled()) != 1 {
		t.Fatalf("Resolve() = %+v, %v", resolved, err)
	}
	if err := registry.Register(first); !errors.Is(err, ErrDuplicateDefinition) {
		t.Fatalf("duplicate Register() error = %v", err)
	}
	if _, err := registry.Resolve("missing", 0); !errors.Is(err, ErrUnknownWorkflow) {
		t.Fatalf("missing Resolve() error = %v", err)
	}
}
