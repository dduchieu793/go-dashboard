package summary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dduchieu793/go-dashboard/backend/internal/llm"
)

const MaxContentLength = 50_000
const maxGeneratedContentLength = 64 << 10

var (
	ErrEmptyContent       = errors.New("content is required")
	ErrContentTooLong     = errors.New("content exceeds maximum length")
	ErrInvalidSummaryType = errors.New("unsupported summary type")
)

type Type string

const (
	TypeBrief       Type = "brief"
	TypeDetailed    Type = "detailed"
	TypeActionItems Type = "action_items"
)

type Request struct {
	Content     string `json:"content"`
	SummaryType Type   `json:"summary_type"`
}

type Result struct {
	Summary     string `json:"summary"`
	SummaryType Type   `json:"summary_type"`
	Model       string `json:"model"`
}

type Generator interface {
	Generate(ctx context.Context, request Request) (Result, error)
}

type Service struct {
	provider llm.Client
}

func NewService(provider llm.Client) *Service {
	return &Service{provider: provider}
}

func (service *Service) Generate(ctx context.Context, request Request) (Result, error) {
	content := strings.TrimSpace(request.Content)
	if content == "" {
		return Result{}, ErrEmptyContent
	}
	if len([]rune(content)) > MaxContentLength {
		return Result{}, ErrContentTooLong
	}
	if !request.SummaryType.Valid() {
		return Result{}, ErrInvalidSummaryType
	}

	generation, err := service.provider.Generate(ctx, buildPrompt(content, request.SummaryType))
	if err != nil {
		return Result{}, err
	}
	var output struct {
		Summary string `json:"summary"`
	}
	if len(generation.Content) > maxGeneratedContentLength {
		return Result{}, fmt.Errorf("%w: summary JSON exceeds maximum length", llm.ErrInvalidResponse)
	}
	decoder := json.NewDecoder(strings.NewReader(generation.Content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil || strings.TrimSpace(output.Summary) == "" {
		return Result{}, fmt.Errorf("%w: summary JSON is malformed", llm.ErrInvalidResponse)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Result{}, fmt.Errorf("%w: summary JSON is malformed", llm.ErrInvalidResponse)
	}
	return Result{
		Summary: strings.TrimSpace(output.Summary), SummaryType: request.SummaryType, Model: generation.Model,
	}, nil
}

func (summaryType Type) Valid() bool {
	switch summaryType {
	case TypeBrief, TypeDetailed, TypeActionItems:
		return true
	default:
		return false
	}
}
