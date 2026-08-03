package modelrouter

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
)

var (
	ErrUnknownProfile = errors.New("unknown model profile")
	ErrUnknownMapping = errors.New("capability has no model profile mapping")
)

type Profile struct {
	Name     string
	Provider string
	Model    string
	Client   llm.Client
}

type Status struct {
	Name           string   `json:"name"`
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	Available      bool     `json:"available"`
	ModelAvailable bool     `json:"model_available"`
	Capabilities   []string `json:"capabilities"`
}

type Router struct {
	profiles map[string]Profile
	mapping  map[string]string
}

func New(profiles []Profile, mapping map[string]string) (*Router, error) {
	router := &Router{profiles: make(map[string]Profile, len(profiles)), mapping: make(map[string]string, len(mapping))}
	for _, profile := range profiles {
		if profile.Name == "" || profile.Provider == "" || profile.Model == "" || profile.Client == nil {
			return nil, fmt.Errorf("%w: invalid profile", ErrUnknownProfile)
		}
		if _, exists := router.profiles[profile.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate profile %s", ErrUnknownProfile, profile.Name)
		}
		router.profiles[profile.Name] = profile
	}
	for capability, profile := range mapping {
		if _, exists := router.profiles[profile]; !exists {
			return nil, fmt.Errorf("%w: %s for capability %s", ErrUnknownProfile, profile, capability)
		}
		router.mapping[capability] = profile
	}
	return router, nil
}

func (router *Router) Resolve(capability, override string) (Profile, error) {
	profileName := override
	if profileName == "" {
		profileName = router.mapping[capability]
	}
	if profileName == "" {
		return Profile{}, fmt.Errorf("%w: %s", ErrUnknownMapping, capability)
	}
	profile, exists := router.profiles[profileName]
	if !exists {
		return Profile{}, fmt.Errorf("%w: %s", ErrUnknownProfile, profileName)
	}
	return profile, nil
}

func (router *Router) Bind(capability string) llm.Client {
	return boundClient{router: router, capability: capability}
}

func (router *Router) Statuses(ctx context.Context) []Status {
	capabilities := make(map[string][]string)
	for capability, profile := range router.mapping {
		capabilities[profile] = append(capabilities[profile], capability)
	}
	names := make([]string, 0, len(router.profiles))
	for name := range router.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	statuses := make([]Status, 0, len(names))
	for _, name := range names {
		profile := router.profiles[name]
		providerStatus := profile.Client.Status(ctx)
		if capabilities[name] == nil {
			capabilities[name] = []string{}
		}
		sort.Strings(capabilities[name])
		statuses = append(statuses, Status{Name: name, Provider: profile.Provider, Model: profile.Model,
			Available: providerStatus.Available, ModelAvailable: providerStatus.ModelAvailable,
			Capabilities: capabilities[name]})
	}
	return statuses
}

type profileContextKey struct{}

func WithProfile(ctx context.Context, profile string) context.Context {
	if profile == "" {
		return ctx
	}
	return context.WithValue(ctx, profileContextKey{}, profile)
}

func ProfileFromContext(ctx context.Context) string {
	profile, _ := ctx.Value(profileContextKey{}).(string)
	return profile
}

type boundClient struct {
	router     *Router
	capability string
}

func (client boundClient) Status(ctx context.Context) llm.Status {
	profile, err := client.router.Resolve(client.capability, "")
	if err != nil {
		return llm.Status{}
	}
	return profile.Client.Status(ctx)
}

func (client boundClient) Generate(ctx context.Context, prompt string) (llm.Generation, error) {
	profile, err := client.router.Resolve(client.capability, ProfileFromContext(ctx))
	if err != nil {
		return llm.Generation{}, err
	}
	return profile.Client.Generate(ctx, prompt)
}
