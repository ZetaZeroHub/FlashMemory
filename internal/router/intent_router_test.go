package router

import "testing"

func TestRouteQuery(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedIntent string
		expectedMode   string
	}{
		{
			name:           "code search query",
			query:          "帮我找 login handler 在哪个文件实现的",
			expectedIntent: IntentCodeSearch,
			expectedMode:   SearchModeSemantic,
		},
		{
			name:           "doc qa query",
			query:          "总结这个 pdf 第3页的关键观点",
			expectedIntent: IntentDocQA,
			expectedMode:   SearchModeSemantic,
		},
		{
			name:           "graph reasoning query",
			query:          "分析 auth 和 payment 的依赖关系图",
			expectedIntent: IntentGraphReason,
			expectedMode:   SearchModeHybrid,
		},
		{
			name:           "deep analysis query",
			query:          "比较两种索引策略的 tradeoff",
			expectedIntent: IntentDeepAnalysis,
			expectedMode:   SearchModeHybrid,
		},
		{
			name:           "tool call query",
			query:          "运行 ingest 命令导入 docs",
			expectedIntent: IntentToolCall,
			expectedMode:   SearchModeKeyword,
		},
		{
			name:           "fallback query",
			query:          "",
			expectedIntent: IntentFallback,
			expectedMode:   SearchModeSemantic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteQuery(tt.query)
			if got.Intent != tt.expectedIntent {
				t.Fatalf("intent mismatch: got=%s want=%s", got.Intent, tt.expectedIntent)
			}
			if got.SearchMode != tt.expectedMode {
				t.Fatalf("search mode mismatch: got=%s want=%s", got.SearchMode, tt.expectedMode)
			}
			if got.Intent != IntentFallback && len(got.Signals) == 0 {
				t.Fatalf("expected non-empty signals for intent %s", got.Intent)
			}
		})
	}
}
