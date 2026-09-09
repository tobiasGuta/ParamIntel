package aiadvisor

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const (
	ProviderGemini     = "gemini"
	defaultGeminiModel = "gemini-3.5-flash-lite"
)

// Provider turns sanitized application structure into candidate hypotheses.
// Provider output is never discovery evidence; every accepted candidate must
// still pass ParamIntel's normal active verification and negative controls.
type Provider interface {
	Name() string
	Model() string
	Suggest(ctx context.Context, input Input, limit int) ([]Suggestion, error)
}

type ProviderConfig struct {
	Provider string
	APIKey   string
	Model    string
	Client   *http.Client
}

func NewProvider(cfg ProviderConfig) (Provider, error) {
	name := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch name {
	case ProviderGemini:
		model := strings.TrimSpace(cfg.Model)
		if model == "" {
			model = defaultGeminiModel
		}
		return NewGeminiProvider(GeminiConfig{
			APIKey: cfg.APIKey,
			Model:  model,
			Client: cfg.Client,
		})
	default:
		return nil, fmt.Errorf("unsupported AI provider %q", cfg.Provider)
	}
}

func DefaultAPIKeyEnv(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderGemini:
		return "GEMINI_API_KEY", nil
	default:
		return "", fmt.Errorf("unsupported AI provider %q", provider)
	}
}
