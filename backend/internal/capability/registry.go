package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrDuplicateCapability = errors.New("duplicate capability")
	ErrUnknownCapability   = errors.New("unknown capability")
)

type Result struct {
	Content       json.RawMessage
	ArtifactType  string
	Model         string
	PromptVersion string
}

type Metadata struct {
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	InputSchema         json.RawMessage `json:"input_schema"`
	OutputSchema        json.RawMessage `json:"output_schema"`
	DefaultModelProfile string          `json:"default_model_profile,omitempty"`
	LLMBacked           bool            `json:"llm_backed"`
}

type Capability interface {
	Name() string
	Metadata() Metadata
	ValidateInput(input json.RawMessage) error
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

type Registry struct {
	capabilities map[string]Capability
}

func NewRegistry() *Registry {
	return &Registry{capabilities: make(map[string]Capability)}
}

func (registry *Registry) Register(capability Capability) error {
	name := capability.Name()
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrDuplicateCapability)
	}
	if _, exists := registry.capabilities[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateCapability, name)
	}
	registry.capabilities[name] = capability
	return nil
}

func (registry *Registry) Resolve(name string) (Capability, error) {
	resolved, exists := registry.capabilities[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCapability, name)
	}
	return resolved, nil
}

func (registry *Registry) Names() []string {
	names := make([]string, 0, len(registry.capabilities))
	for name := range registry.capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (registry *Registry) Metadata() []Metadata {
	items := make([]Metadata, 0, len(registry.capabilities))
	for _, item := range registry.capabilities {
		items = append(items, item.Metadata())
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}
