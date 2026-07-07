# Resumable Large File Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add resumable large-file upload so clients can continue from the last accepted byte after an interrupted upload.

**Architecture:** Add upload sessions owned by tenant and knowledge base. Clients create a session, append raw bytes at an explicit `Upload-Offset`, query progress, then complete the session to reuse the existing ingestion pipeline.

**Tech Stack:** Go, Hertz HTTP handlers, existing `internal/ingest` service, OpenAPI YAML, contract and HTTP unit tests.

---

### Task 1: Upload Session Model

**Files:**
- Create: `internal/ingest/uploads.go`

- [ ] **Step 1: Add upload session types and memory store**

Define `UploadSession`, `UploadStatus`, `UploadStore`, and `MemoryUploadStore`. Store bytes in a local temporary file while metadata lives in memory.

- [ ] **Step 2: Add append, get, complete, and cancel operations**

Implement offset validation. Return a conflict when the client offset differs from the stored received byte count.

### Task 2: HTTP Routes

**Files:**
- Modify: `internal/http/router.go`

- [ ] **Step 1: Register resumable upload routes**

Add `POST /v1/knowledge-bases/:id/uploads`, `GET /v1/uploads/:id`, `PUT /v1/uploads/:id`, `POST /v1/uploads/:id:complete`, and `DELETE /v1/uploads/:id`.

- [ ] **Step 2: Implement handlers**

Validate the knowledge base at session creation, parse `Upload-Offset`, stream chunk bytes into the upload store, and call `Ingest` only on completion.

### Task 3: Tests

**Files:**
- Modify: `internal/http/router_test.go`

- [ ] **Step 1: Add happy-path resume test**

Create a session, upload the first bytes, query offset, upload remaining bytes, complete, and assert an ingestion response is returned.

- [ ] **Step 2: Add offset-conflict and cancel tests**

Verify wrong offsets return `409 upload_offset_mismatch` with the current offset, and canceled sessions are no longer accessible.

### Task 4: OpenAPI And Docs

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `docs/api.md`
- Modify: `docs/api/ingestion-and-query.md`

- [ ] **Step 1: Document endpoints and schemas**

Add request and response schemas for session creation, upload status, chunk append, completion, and cancel.

- [ ] **Step 2: Add usage guidance**

Document the resume loop: create session, send chunks with `Upload-Offset`, recover by querying status, then complete.

### Task 5: Validation And PR

**Files:**
- All changed files

- [ ] **Step 1: Format and test**

Run `gofmt` on changed Go files, `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/http ./tests/contract -v`, and the required `go test ./tests/contract -v`.

- [ ] **Step 2: Commit and open PR**

Create a feature branch, commit only resumable upload files, push to origin, and open a PR with the test results in the description.
