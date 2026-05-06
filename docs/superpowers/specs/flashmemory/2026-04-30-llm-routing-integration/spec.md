# LLM Routing Integration Spec

- Date: 2026-04-30
- Status: implementation spec
- Scope: connect LLM route decisions to the main model-calling paths

## User Stories

- As a user, search and deep analysis paths should not only expose route decisions; model calls should use the routed model where applicable.
- As an operator, the first version should reuse existing `ModelConfigs` ordering as low/medium/high routing buckets without a new `fm.yaml` schema.
- As an existing user, single-model or default-model configurations should behave as before.

## Acceptance Criteria

- AC1: `RouteDecision.ModelID` is used for search keyword extraction.
- AC2: `utils.Completion` routes prompt-based LLM calls through `internal/llm` model selection.
- AC3: analyzer and module analyzer calls using `utils.Completion` automatically use the routed model path.
- AC4: No request/response envelope, CLI flag, or config schema changes are required.

## Non-Goals

- No AST fan-in, Gamma/Delta, query entropy, or cross-module statistical signal implementation.
- No new cloud provider abstraction.
- No config migration.
