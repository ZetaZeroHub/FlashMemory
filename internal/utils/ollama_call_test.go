package utils

import (
	"testing"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/llm"
)

func TestSelectCompletionModelConfigUsesLLMRouter(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "default-small",
		ModelConfigs: []config.ModelConfig{
			{Name: "small-local", NumCtx: 1024},
			{Name: "mid-local", NumCtx: 4096},
			{Name: "large-local", NumCtx: 8192},
		},
	}

	low, lowDecision := SelectCompletionModelConfig(cfg, "short prompt", llm.OperationCompletion)
	if low == nil || low.Name != "small-local" {
		t.Fatalf("expected small-local for low prompt, got %#v", low)
	}
	if lowDecision.ModelID != "small-local" {
		t.Fatalf("unexpected low decision model: %s", lowDecision.ModelID)
	}

	longPrompt := make([]byte, 9000)
	for i := range longPrompt {
		longPrompt[i] = 'x'
	}
	high, highDecision := SelectCompletionModelConfig(cfg, string(longPrompt), llm.OperationAnalysis)
	if high == nil || high.Name != "large-local" {
		t.Fatalf("expected large-local for high prompt, got %#v", high)
	}
	if highDecision.ReasoningMode != llm.ReasoningDeep {
		t.Fatalf("unexpected high reasoning mode: %s", highDecision.ReasoningMode)
	}
}

func TestSelectCompletionModelConfigFallsBackWhenNoModelConfigs(t *testing.T) {
	cfg := &config.Config{DefaultModel: "default-only"}

	selected, decision := SelectCompletionModelConfig(cfg, "short prompt", llm.OperationCompletion)
	if selected != nil {
		t.Fatalf("expected nil selected config when ModelConfigs is empty, got %#v", selected)
	}
	if decision.ModelID != "default-only" {
		t.Fatalf("expected default-only decision, got %s", decision.ModelID)
	}
}
