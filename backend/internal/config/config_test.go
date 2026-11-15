package config

import "testing"

func TestResolveLLM(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		key      string
		model    string
		wantMode string
		wantErr  bool
	}{
		{"auto without key is mock", "auto", "", "", LLMModeMock, false},
		{"empty mode without key is mock", "", "", "", LLMModeMock, false},
		{"auto with key is claude", "auto", "sk-test", "", LLMModeClaude, false},
		{"mock stays mock even with key", "mock", "sk-test", "", LLMModeMock, false},
		{"claude with key", "claude", "sk-test", "", LLMModeClaude, false},
		{"claude without key fails", "claude", "", "", "", true},
		{"whitespace key counts as empty", "auto", "   ", "", LLMModeMock, false},
		{"case insensitive", "MOCK", "", "", LLMModeMock, false},
		{"invalid mode fails", "gpt", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveLLM(tt.mode, tt.key, tt.model)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got.Mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", got.Mode, tt.wantMode)
			}
		})
	}
}

func TestResolveLLMDefaultModel(t *testing.T) {
	got, err := ResolveLLM("mock", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != DefaultModel {
		t.Fatalf("model = %q, want %q", got.Model, DefaultModel)
	}
	got, _ = ResolveLLM("mock", "", "claude-custom")
	if got.Model != "claude-custom" {
		t.Fatalf("model = %q, want claude-custom", got.Model)
	}
}
