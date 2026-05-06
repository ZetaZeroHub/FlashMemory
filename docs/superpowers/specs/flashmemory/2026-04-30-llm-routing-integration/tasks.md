# LLM Routing Integration Tasks

- T001: Add failing `internal/llm` tests for prompt length and operation complexity routing.
- T002: Implement route input extensions and prompt route helper.
- T003: Add failing utils pure-function tests for routed model config selection without network calls.
- T004: Use LLM router inside `utils.Completion` while preserving old fallback behavior.
- T005: Add or update search path tests proving routed `KeywordModel` is used.
- T006: Align CLI/HTTP search logging and route metadata with routed model decisions.
- T007: Run targeted verification:
  - `env GOCACHE=/private/tmp/flashmemory-gocache go test ./internal/llm ./internal/utils ./internal/search ./cmd/app -run 'Test.*LLM|Test.*Route|Test.*Search'`
