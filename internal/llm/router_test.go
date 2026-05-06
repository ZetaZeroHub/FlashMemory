package llm

import (
	"testing"

	"github.com/kinglegendzzh/flashmemory/config"
)

func TestDecideForSearch(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "default-small",
		ModelConfigs: []config.ModelConfig{
			{Name: "small-local", NumCtx: 1024},
			{Name: "mid-local", NumCtx: 4096},
			{
				Name:   "large-local",
				NumCtx: 8192,
				CloudModel: config.CloudModel{
					Enabled: true,
					Type:    "githave",
					Model:   "cloud-auto",
				},
			},
		},
	}

	low := DecideForSearch(cfg, RouteInput{
		Query:      "找一下 login handler 在哪",
		Intent:     "code_search",
		SearchMode: "semantic",
		Limit:      5,
	})
	if low.ModelID != "small-local" {
		t.Fatalf("unexpected low model: %s", low.ModelID)
	}
	if low.ReasoningMode != ReasoningLight {
		t.Fatalf("unexpected low reasoning mode: %s", low.ReasoningMode)
	}

	high := DecideForSearch(cfg, RouteInput{
		Query:          "请分析 auth 和 payment 的跨模块调用链与架构权衡，解释为什么当前设计在高并发下存在潜在瓶颈并给出替代策略",
		Intent:         "deep_analysis",
		SearchMode:     "hybrid",
		Strict:         true,
		EnableReranker: true,
		Limit:          20,
	})
	if high.ModelID != "cloud-auto" {
		t.Fatalf("unexpected high model: %s", high.ModelID)
	}
	if high.Provider != "githave" {
		t.Fatalf("unexpected provider: %s", high.Provider)
	}
	if high.ReasoningMode != ReasoningDeep {
		t.Fatalf("unexpected high reasoning mode: %s", high.ReasoningMode)
	}
	if high.CostHint != "high" {
		t.Fatalf("unexpected high cost hint: %s", high.CostHint)
	}
}

func TestDecideForPromptUsesPromptLengthAndOperation(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "default-small",
		ModelConfigs: []config.ModelConfig{
			{Name: "small-local", NumCtx: 1024},
			{Name: "mid-local", NumCtx: 4096},
			{Name: "large-local", NumCtx: 8192},
		},
	}

	low := DecideForPrompt(cfg, RouteInput{
		Operation:    OperationCompletion,
		PromptLength: 80,
	})
	if low.ModelID != "small-local" {
		t.Fatalf("unexpected low prompt model: %s", low.ModelID)
	}
	if low.ReasoningMode != ReasoningLight {
		t.Fatalf("unexpected low prompt reasoning: %s", low.ReasoningMode)
	}

	high := DecideForPrompt(cfg, RouteInput{
		Operation:    OperationAnalysis,
		PromptLength: 9000,
	})
	if high.ModelID != "large-local" {
		t.Fatalf("unexpected high prompt model: %s", high.ModelID)
	}
	if high.ReasoningMode != ReasoningDeep {
		t.Fatalf("unexpected high prompt reasoning: %s", high.ReasoningMode)
	}
	if high.ContextBudget != 8192 {
		t.Fatalf("unexpected high prompt context budget: %d", high.ContextBudget)
	}
}
