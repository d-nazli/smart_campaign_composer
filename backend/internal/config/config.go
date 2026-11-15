// Package config reads process configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	LLMModeMock   = "mock"
	LLMModeClaude = "claude"

	DefaultModel = "claude-sonnet-5-5"
)

// LLM describes which LLM implementation the process uses.
type LLM struct {
	Mode   string // "mock" or "claude" (already resolved, never "auto")
	Model  string // only meaningful when Mode == "claude"
	APIKey string // never log this
	Reason string // human-readable explanation of why this mode was chosen
}

// ResolveLLM turns the LLM_MODE / ANTHROPIC_API_KEY pair into a concrete mode.
//
//	auto (or empty): claude if a key is present, otherwise mock
//	mock:            always mock, even when a key is present
//	claude:          requires a key, otherwise an error
func ResolveLLM(mode, apiKey, model string) (LLM, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}

	switch mode {
	case "", "auto":
		if apiKey != "" {
			return LLM{Mode: LLMModeClaude, Model: model, APIKey: apiKey, Reason: "LLM_MODE=auto and ANTHROPIC_API_KEY is set"}, nil
		}
		return LLM{Mode: LLMModeMock, Model: model, Reason: "LLM_MODE=auto and ANTHROPIC_API_KEY is empty"}, nil
	case LLMModeMock:
		return LLM{Mode: LLMModeMock, Model: model, Reason: "LLM_MODE=mock"}, nil
	case LLMModeClaude:
		if apiKey == "" {
			return LLM{}, fmt.Errorf("LLM_MODE=claude but ANTHROPIC_API_KEY is empty")
		}
		return LLM{Mode: LLMModeClaude, Model: model, APIKey: apiKey, Reason: "LLM_MODE=claude"}, nil
	default:
		return LLM{}, fmt.Errorf("invalid LLM_MODE %q (want auto, mock or claude)", mode)
	}
}

// Env returns the environment variable or the fallback when it is unset/empty.
func Env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// LoadLLM reads the LLM settings from the environment.
func LoadLLM() (LLM, error) {
	return ResolveLLM(os.Getenv("LLM_MODE"), os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("CLAUDE_MODEL"))
}
