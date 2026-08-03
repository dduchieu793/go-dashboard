package workflow

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrDuplicateDefinition = errors.New("duplicate workflow definition")
	ErrUnknownWorkflow     = errors.New("unknown workflow")
)

type Registry struct {
	definitions map[string]map[int]Definition
}

func NewRegistry() *Registry {
	return &Registry{definitions: make(map[string]map[int]Definition)}
}

func (registry *Registry) Register(definition Definition) error {
	if err := definition.Validate(); err != nil {
		return err
	}
	versions := registry.definitions[definition.ID]
	if versions == nil {
		versions = make(map[int]Definition)
		registry.definitions[definition.ID] = versions
	}
	if _, exists := versions[definition.Version]; exists {
		return fmt.Errorf("%w: %s version %d", ErrDuplicateDefinition, definition.ID, definition.Version)
	}
	versions[definition.Version] = definition
	return nil
}

func (registry *Registry) Resolve(id string, version int) (Definition, error) {
	versions := registry.definitions[id]
	if len(versions) == 0 {
		return Definition{}, fmt.Errorf("%w: %s", ErrUnknownWorkflow, id)
	}
	if version > 0 {
		definition, exists := versions[version]
		if !exists {
			return Definition{}, fmt.Errorf("%w: %s version %d", ErrUnknownWorkflow, id, version)
		}
		return definition, nil
	}
	latestVersion := 0
	for candidate := range versions {
		if candidate > latestVersion && versions[candidate].Enabled {
			latestVersion = candidate
		}
	}
	if latestVersion == 0 {
		return Definition{}, fmt.Errorf("%w: %s has no enabled version", ErrUnknownWorkflow, id)
	}
	return versions[latestVersion], nil
}

func (registry *Registry) Enabled() []Definition {
	definitions := make([]Definition, 0, len(registry.definitions))
	for id := range registry.definitions {
		if definition, err := registry.Resolve(id, 0); err == nil {
			definitions = append(definitions, definition)
		}
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].ID < definitions[j].ID })
	return definitions
}
