# Multimodal Ingest Hardening Plan

## Technical Design

- Change doc hierarchy persistence from project-wide replacement to affected-doc replacement. Compute affected `doc_id` values from current results, then delete and rewrite only those docs.
- Add an index-layer source resolution helper that normalizes base source and resolves `source = base` or `source LIKE base || '::%'`.
- Use that helper in `/api/substrate/doc/tree` so clients can pass raw file paths or internal source anchors.
- Extend ingest document extension support to include image OCR formats already supported by the parser.

## Public Interfaces

- `/api/substrate/doc/tree` keeps the same request and response envelope.
- `source` semantics become more permissive: exact source and base file path are both accepted.
- `SupportedTextDocumentExtensions()` now includes supported image extensions.

## Compatibility

- No database schema changes.
- No config changes.
- Existing exact-source queries continue to work.

## Verification

- Unit tests cover affected-doc replacement and image collection.
- HTTP tests cover base-source lookup for PDF/PPT-style internal anchors.
- Targeted verification excludes unrelated historical package failures.
