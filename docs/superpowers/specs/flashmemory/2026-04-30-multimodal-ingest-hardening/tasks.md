# Multimodal Ingest Hardening Tasks

- T001: Add a failing doc graph regression test proving one-document rebuild preserves other documents.
- T002: Implement affected-doc deletion in `PersistDocHierarchyFromResults`.
- T003: Add a failing `/api/substrate/doc/tree` test for base source resolving internal `::` sources.
- T004: Implement index source resolution helper and wire it into the HTTP handler.
- T005: Add a failing ingest extension test for image OCR file types.
- T006: Extend ingest extension support and CLI/core help text for images.
- T007: Run targeted verification:
  - `env GOCACHE=/private/tmp/flashmemory-gocache go test ./internal/ingest ./internal/parser ./cmd/app -run 'Test.*Doc|Test.*Ingest|TestSubstrateDoc'`
