package llm

import (
	"strings"

	"github.com/kinglegendzzh/flashmemory/config"
)

const (
	ReasoningLight    = "light"
	ReasoningBalanced = "balanced"
	ReasoningDeep     = "deep"

	OperationSearch     = "search"
	OperationCompletion = "completion"
	OperationAnalysis   = "analysis"
	OperationModule     = "module_analysis"
)

// RouteInput captures the signals used for model routing.
type RouteInput struct {
	Query          string
	Intent         string
	Operation      string
	PromptLength   int
	SearchMode     string
	Strict         bool
	EnableReranker bool
	Limit          int
}

// RouteDecision is the LLM routing output for observability and execution.
type RouteDecision struct {
	Provider      string   `json:"provider"`
	ModelID       string   `json:"model_id"`
	ContextBudget int      `json:"context_budget"`
	ReasoningMode string   `json:"reasoning_mode"`
	CostHint      string   `json:"cost_hint"`
	Complexity    int      `json:"complexity"`
	Signals       []string `json:"signals,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

// DecideForSearch chooses model strategy for search-time LLM calls.
func DecideForSearch(cfg *config.Config, in RouteInput) RouteDecision {
	if strings.TrimSpace(in.Operation) == "" {
		in.Operation = OperationSearch
	}
	return decide(cfg, in, "selected by query complexity + intent + search controls")
}

// DecideForPrompt chooses a model strategy for prompt-based LLM calls.
func DecideForPrompt(cfg *config.Config, in RouteInput) RouteDecision {
	if strings.TrimSpace(in.Operation) == "" {
		in.Operation = OperationCompletion
	}
	return decide(cfg, in, "selected by prompt complexity + operation")
}

func decide(cfg *config.Config, in RouteInput, reason string) RouteDecision {
	score, signals := estimateComplexity(in)
	mode := ReasoningBalanced
	if score <= 3 {
		mode = ReasoningLight
	} else if score >= 8 {
		mode = ReasoningDeep
	}
	costHint := "low"
	if score >= 8 {
		costHint = "high"
	} else if score >= 5 {
		costHint = "medium"
	}

	decision := RouteDecision{
		Provider:      "local",
		ModelID:       "",
		ContextBudget: 2048,
		ReasoningMode: mode,
		CostHint:      costHint,
		Complexity:    score,
		Signals:       signals,
		Reason:        "complexity driven routing",
	}
	if cfg == nil {
		return decision
	}

	model := cfg.DefaultModel
	contextBudget := 2048
	if len(cfg.ModelConfigs) > 0 {
		mc := pickModelConfig(cfg, score)
		if mc.Name != "" {
			model = mc.Name
		}
		if mc.NumCtx > 0 {
			contextBudget = mc.NumCtx
		} else if mc.MaxTokens > 0 {
			contextBudget = mc.MaxTokens
		}
		if mc.CloudModel.Enabled {
			decision.Provider = mc.CloudModel.Type
			if strings.TrimSpace(mc.CloudModel.Model) != "" {
				model = mc.CloudModel.Model
			}
		}
	}

	decision.ModelID = model
	decision.ContextBudget = contextBudget
	decision.Reason = reason
	return decision
}

func estimateComplexity(in RouteInput) (int, []string) {
	complexity := 1
	signals := make([]string, 0, 8)

	queryLen := len([]rune(strings.TrimSpace(in.Query)))
	if in.PromptLength > 0 {
		queryLen = in.PromptLength
	}
	if queryLen > 220 {
		complexity += 4
		signals = append(signals, "very_long_query")
	} else if queryLen > 120 {
		complexity += 3
		signals = append(signals, "long_query")
	} else if queryLen > 60 {
		complexity += 2
		signals = append(signals, "medium_query")
	}

	lowerQ := strings.ToLower(in.Query)
	if strings.Contains(lowerQ, "why") || strings.Contains(in.Query, "为什么") || strings.Contains(in.Query, "根因") {
		complexity += 2
		signals = append(signals, "why_reasoning")
	}
	if strings.Contains(lowerQ, "tradeoff") || strings.Contains(in.Query, "权衡") || strings.Contains(in.Query, "架构") {
		complexity += 2
		signals = append(signals, "architecture_tradeoff")
	}

	switch strings.TrimSpace(in.Operation) {
	case OperationAnalysis, OperationModule:
		complexity += 3
		signals = append(signals, in.Operation)
	case OperationCompletion:
		signals = append(signals, OperationCompletion)
	case OperationSearch:
		signals = append(signals, OperationSearch)
	}

	switch in.Intent {
	case "graph_reasoning":
		complexity += 2
		signals = append(signals, "graph_reasoning")
	case "deep_analysis":
		complexity += 3
		signals = append(signals, "deep_analysis")
	case "doc_qa":
		complexity += 1
		signals = append(signals, "doc_qa")
	}

	if strings.EqualFold(in.SearchMode, "hybrid") {
		complexity += 1
		signals = append(signals, "hybrid_mode")
	}
	if in.EnableReranker {
		complexity += 1
		signals = append(signals, "reranker_enabled")
	}
	if in.Strict {
		complexity += 1
		signals = append(signals, "strict_mode")
	}
	if in.Limit > 10 {
		complexity += 1
		signals = append(signals, "large_limit")
	}
	return complexity, signals
}

// SelectModelConfig exposes the same low/medium/high bucket selection used by
// route decisions while preserving the existing ModelConfigs ordering contract.
func SelectModelConfig(cfg *config.Config, complexity int) (config.ModelConfig, bool) {
	if cfg == nil || len(cfg.ModelConfigs) == 0 {
		return config.ModelConfig{}, false
	}
	return pickModelConfig(cfg, complexity), true
}

func pickModelConfig(cfg *config.Config, complexity int) config.ModelConfig {
	if cfg == nil || len(cfg.ModelConfigs) == 0 {
		return config.ModelConfig{}
	}
	if len(cfg.ModelConfigs) == 1 {
		return cfg.ModelConfigs[0]
	}

	// Low/Medium/High buckets mapped to model config indexes.
	last := len(cfg.ModelConfigs) - 1
	if complexity <= 3 {
		return cfg.ModelConfigs[0]
	}
	if complexity <= 7 {
		mid := len(cfg.ModelConfigs) / 2
		return cfg.ModelConfigs[mid]
	}
	return cfg.ModelConfigs[last]
}
