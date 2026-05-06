# LLM Routing Integration Plan

## Technical Design

- Extend `internal/llm.RouteInput` with operation and prompt length fields for non-search routes.
- Add prompt routing helpers that reuse existing `ModelConfigs` buckets: first for low, middle for medium, last for high complexity.
- Keep `utils.Completion(prompt)` signature unchanged, but internally select the model config through the LLM router.
- Keep `DefaultModelCompletionWithModel` as the explicit search keyword extraction entrypoint.
- Add structured logs for routed model, complexity, reasoning mode, and routing reason.

## Public Interfaces

- No new config keys.
- Existing HTTP/CLI inputs remain unchanged.
- Existing route data in search responses remains compatible.

## Compatibility

- If `ModelConfigs` is empty, keep the existing `cloud.GetModelConfigByPromptLength` behavior.
- If there is only one model config, use it for all complexities.
- Cloud model behavior continues to come from the selected existing `ModelConfig`.

## Verification

- `internal/llm` tests cover prompt/operation complexity.
- utils tests cover pure model-config selection without network calls.
- search tests cover route-selected keyword model propagation where practical.
