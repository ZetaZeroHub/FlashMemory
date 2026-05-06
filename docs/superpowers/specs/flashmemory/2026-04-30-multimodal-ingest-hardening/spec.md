# Multimodal Ingest Hardening Spec

- Date: 2026-04-30
- Status: implementation spec
- Scope: close the concrete multimodal substrate defects found in blind review

## User Stories

- As a user running incremental or watch ingest, rebuilding one document must not delete the doc graph for other documents in the same project.
- As a DeepMemory adapter or HTTP client, I can query `/api/substrate/doc/tree` with the original document path even when stored nodes use internal `::page`, `::slide`, or `::ocr` source suffixes.
- As a CLI user, `fm ingest` can collect image files already supported by `DetectLang=image` and `DocParser(image)`.

## Acceptance Criteria

- AC1: After two documents are persisted, rebuilding only one document leaves the other document's `doc_nodes` and `doc_edges` intact.
- AC2: `/api/substrate/doc/tree` accepts either exact internal source or base source such as `docs/a.pdf` for nodes stored as `docs/a.pdf::page_1`.
- AC3: `CollectTextDocuments` supports `.png`, `.jpg`, `.jpeg`, `.webp`, `.bmp`, `.tif`, and `.tiff`.
- AC4: Existing `doc_nodes`, `doc_edges`, and `parse_artifacts` schemas are unchanged.

## Non-Goals

- No Docling or marker-pdf integration.
- No PDF/PPT parsing quality rewrite.
- No DeepMemory governance or memory lifecycle changes.
