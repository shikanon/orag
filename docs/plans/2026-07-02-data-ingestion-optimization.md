# Data Ingestion Optimization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve data ingestion consistency and parser safety with small, verifiable changes.

**Architecture:** Keep the current synchronous ingestion architecture. Add defensive validation at parser and indexer boundaries, and document larger follow-up options such as async jobs, outbox retries, and batch control.

**Tech Stack:** Go, internal ingest parser/chunker/service packages, Postgres repository, Qdrant vector store, Go unit tests.

---

### Task 1: List Optimization Options

**Files:**
- Create: `docs/plans/2026-07-02-data-ingestion-optimization.md`

**Step 1: Capture candidate optimizations**

- Async ingestion worker: HTTP creates a job and returns quickly; background worker runs parsing, chunking, embedding, and indexing.
- Indexing consistency: avoid Postgres/Qdrant partial success with retry/outbox or explicit job sub-status.
- Idempotency consistency: ensure chunk IDs and document IDs stay aligned when an existing document ID is reused.
- Parser safety: do not fallback PDF/image bytes to raw strings when multimodal parsing is unavailable or empty.
- Chunking quality: improve token estimation for Chinese and split oversized paragraphs.
- Batch control: split embedding/upsert into bounded batches with retry and metrics.
- Error classification: map parse/size/upstream failures to clearer HTTP status codes.

**Step 2: Select immediate changes**

Implement low-risk changes first:
- Parser safety for PDF/images.
- Repository chunk ID remapping for reused document IDs.
- Tests for both behaviors where practical.

### Task 2: Parser Safety

**Files:**
- Modify: `internal/ingest/parser/parser.go`
- Test: `internal/ingest/parser/parser_test.go`

**Step 1: Write failing tests**

Add tests that parse PDF/image content without a successful multimodal response and expect an error instead of binary fallback.

**Step 2: Implement minimal parser change**

For `.pdf`, `.png`, `.jpg`, `.jpeg`, return an explicit error when no non-empty multimodal markdown is available.

**Step 3: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/ingest/parser -v
```

Expected: PASS.

### Task 3: Idempotent Chunk Consistency

**Files:**
- Modify: `internal/storage/postgres/repository.go`
- Test: `internal/storage/postgres/repository_test.go`

**Step 1: Write failing test or helper-level check**

Cover reused document IDs by verifying chunk `DocumentID` and chunk `ID` are both remapped to the canonical existing document ID.

**Step 2: Implement remap helper**

When `existingID != ""`, rewrite each chunk's `DocumentID` and deterministic `ID` from the canonical existing document ID before inserting.

**Step 3: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres -v
```

Expected: PASS.

### Task 4: Contract Verification

**Files:**
- No source changes.

**Step 1: Run focused tests**

Run parser, ingest, and storage tests:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/ingest/... ./internal/storage/postgres -v
```

Expected: PASS.

**Step 2: Run API contract tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v
```

Expected: PASS.

### Task 5: Chunking Quality

**Files:**
- Modify: `internal/ingest/chunker/chunker.go`
- Test: `internal/ingest/chunker/chunker_test.go`

**Step 1: Write failing tests**

Add tests for Chinese text and a single oversized English paragraph. Both cases should split into multiple chunks and each chunk should stay within `SizeTokens`.

**Step 2: Implement mixed-language token estimation**

Replace `strings.Fields` token counting with an internal unit model:
- English and other whitespace-separated text counts by word.
- CJK text counts by character.
- Punctuation does not create standalone tokens.

**Step 3: Split oversized paragraphs**

When a single paragraph exceeds `SizeTokens`, split it into bounded windows with `OverlapTokens` applied between windows.

**Step 4: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/ingest/chunker -v
```

Expected: PASS.
