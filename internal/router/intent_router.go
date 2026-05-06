package router

import (
	"fmt"
	"strings"
)

const (
	IntentCodeSearch   = "code_search"
	IntentDocQA        = "doc_qa"
	IntentGraphReason  = "graph_reasoning"
	IntentDeepAnalysis = "deep_analysis"
	IntentToolCall     = "tool_call"
	IntentFallback     = "fallback"
	SearchModeSemantic = "semantic"
	SearchModeKeyword  = "keyword"
	SearchModeHybrid   = "hybrid"
	SearchModeAuto     = "auto"
)

// IntentDecision is the minimal explainable routing output.
type IntentDecision struct {
	Intent     string   `json:"intent"`
	SearchMode string   `json:"search_mode"`
	Confidence float64  `json:"confidence"`
	Signals    []string `json:"signals,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

type intentRule struct {
	intent string
	tokens []string
	weight int
}

var rules = []intentRule{
	{
		intent: IntentCodeSearch,
		weight: 2,
		tokens: []string{
			"函数", "方法", "class", "struct", "interface", "package", "调用", "定义", "实现",
			"代码", "文件", ".go", ".py", ".js", ".ts", ".java", ".cpp", "import", "bug", "修复",
		},
	},
	{
		intent: IntentDocQA,
		weight: 2,
		tokens: []string{
			"文档", "pdf", "ppt", "pptx", "doc", "docx", "markdown", "md", "章节", "页", "slide",
			"论文", "白皮书", "spec", "规范", "手册",
		},
	},
	{
		intent: IntentGraphReason,
		weight: 3,
		tokens: []string{
			"依赖", "调用链", "关系图", "graph", "拓扑", "节点", "边", "路径", "上游", "下游",
		},
	},
	{
		intent: IntentDeepAnalysis,
		weight: 2,
		tokens: []string{
			"为什么", "why", "对比", "权衡", "tradeoff", "架构", "设计", "策略", "分析", "评估", "推理",
		},
	},
	{
		intent: IntentToolCall,
		weight: 3,
		tokens: []string{
			"执行", "运行", "run", "命令", "tool", "工具", "脚本", "invoke", "调用api", "terminal",
		},
	},
}

var intentPriority = []string{
	IntentToolCall,
	IntentGraphReason,
	IntentDeepAnalysis,
	IntentDocQA,
	IntentCodeSearch,
	IntentFallback,
}

// RouteQuery classifies user query into a minimal route plan.
func RouteQuery(query string) IntentDecision {
	raw := strings.TrimSpace(query)
	if raw == "" {
		return IntentDecision{
			Intent:     IntentFallback,
			SearchMode: SearchModeSemantic,
			Confidence: 0.2,
			Reason:     "empty query, fallback to semantic search",
		}
	}

	lowered := strings.ToLower(raw)
	scores := map[string]int{
		IntentCodeSearch:   0,
		IntentDocQA:        0,
		IntentGraphReason:  0,
		IntentDeepAnalysis: 0,
		IntentToolCall:     0,
	}
	signals := map[string][]string{
		IntentCodeSearch:   {},
		IntentDocQA:        {},
		IntentGraphReason:  {},
		IntentDeepAnalysis: {},
		IntentToolCall:     {},
	}

	for _, rule := range rules {
		for _, token := range rule.tokens {
			if strings.Contains(lowered, token) {
				scores[rule.intent] += rule.weight
				signals[rule.intent] = append(signals[rule.intent], token)
			}
		}
	}

	intent, score := pickIntent(scores)
	if score <= 0 {
		intent = IntentFallback
	}

	mode := modeForIntent(intent)
	total := 0
	for _, v := range scores {
		total += v
	}
	confidence := 0.3
	if total > 0 && score > 0 {
		confidence = float64(score) / float64(total)
	}

	matched := signals[intent]
	reason := fmt.Sprintf("intent=%s, matched_signals=%v", intent, matched)
	if intent == IntentFallback {
		reason = "no explicit signal matched, fallback to semantic search"
	}

	return IntentDecision{
		Intent:     intent,
		SearchMode: mode,
		Confidence: confidence,
		Signals:    matched,
		Reason:     reason,
	}
}

func pickIntent(scores map[string]int) (string, int) {
	bestIntent := IntentFallback
	bestScore := -1
	for _, intent := range intentPriority {
		score, ok := scores[intent]
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestIntent = intent
		}
	}
	return bestIntent, bestScore
}

func modeForIntent(intent string) string {
	switch intent {
	case IntentGraphReason, IntentDeepAnalysis:
		return SearchModeHybrid
	case IntentToolCall:
		return SearchModeKeyword
	case IntentFallback, IntentCodeSearch, IntentDocQA:
		return SearchModeSemantic
	default:
		return SearchModeSemantic
	}
}
