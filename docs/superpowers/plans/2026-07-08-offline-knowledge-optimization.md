# Offline Knowledge Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build ORAG offline knowledge optimization: nightly history mining, recall replay, Codex deep analysis, evidence-grounded optimization items, shadow retrieval, and eval regression.

**Architecture:** Add a focused `internal/offlineknowledge` package for domain models, orchestration, validation, shadow retrieval, and Codex tool contracts. Add PostgreSQL persistence, HTTP management routes, config, metrics, and app wiring while keeping Codex read-only and keeping optimization items separate from the primary knowledge base.

**Tech Stack:** Go, Hertz HTTP server, PostgreSQL JSONB, Qdrant-backed existing retrievers, existing eval runner, Prometheus text metrics, OpenAPI contract tests.

---

## File Structure

- Create: `internal/offlineknowledge/types.go` for run, question cluster, optimization item, evidence, state, and request/response types.
- Create: `internal/offlineknowledge/repository.go` for persistence interfaces.
- Create: `internal/offlineknowledge/service.go` for run orchestration, history extraction, recall replay hooks, item creation, and state transitions.
- Create: `internal/offlineknowledge/validator.go` for evidence quote, content fingerprint, conclusion-level validation, and re-validation.
- Create: `internal/offlineknowledge/codex.go` for Codex analyzer request/response schemas and read-only tool contracts.
- Create: `internal/offlineknowledge/shadow.go` for shadow retrieval logic and event recording.
- Create: `internal/offlineknowledge/memory_repository.go` for unit tests and memory backend.
- Create: `internal/storage/postgres/offline_knowledge.go` for PostgreSQL repository implementation.
- Create: `migrations/000013_offline_knowledge_optimization.sql` for tables, indexes, partition base table, and uniqueness constraints.
- Modify: `internal/config/config.go` and `configs/config.example.yaml` for `maintenance.offline_knowledge_organizer`.
- Modify: `internal/observability/metrics.go` for offline knowledge and shadow metrics.
- Modify: `internal/http/router.go` for management API routes and handlers.
- Modify: `internal/app/app.go` to wire config, repository, service, metrics, and shadow retriever hooks.
- Modify: `api/openapi.yaml` for new API schemas and routes.
- Test: `internal/offlineknowledge/*_test.go`, `internal/storage/postgres/offline_knowledge_test.go`, `internal/http/router_test.go`, `internal/config/config_test.go`, `internal/observability/metrics_test.go`, `tests/contract/openapi_test.go`.

### Task 1: Domain Types And State Machine

**Files:**
- Create: `internal/offlineknowledge/types.go`
- Test: `internal/offlineknowledge/types_test.go`

- [ ] **Step 1: Define domain enums and models**

Create `RunStatus`, `ItemStatus`, `ItemType`, `RecallQuality`, `FailureType`, `SourceFingerprint`, `Evidence`, `DeepSearchStep`, `OfflineKnowledgeRun`, `QuestionCluster`, `OptimizationItem`, and `ShadowRetrievalEvent`.

```go
package offlineknowledge

import "time"

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

type ItemStatus string

const (
	ItemStatusCandidate          ItemStatus = "candidate"
	ItemStatusEvidenceValidating ItemStatus = "evidence_validating"
	ItemStatusNeedsReview        ItemStatus = "needs_review"
	ItemStatusVerified           ItemStatus = "verified"
	ItemStatusShadowEnabled      ItemStatus = "shadow_enabled"
	ItemStatusRegressionPassed   ItemStatus = "regression_passed"
	ItemStatusRegressionFailed   ItemStatus = "regression_failed"
	ItemStatusPublished          ItemStatus = "published"
	ItemStatusKnowledgeGap       ItemStatus = "knowledge_gap"
	ItemStatusRejected           ItemStatus = "rejected"
	ItemStatusStale              ItemStatus = "stale"
	ItemStatusDeprecated         ItemStatus = "deprecated"
)

type ItemType string

const (
	ItemTypeAnswer       ItemType = "answer_item"
	ItemTypeQueryRewrite ItemType = "query_rewrite_item"
	ItemTypeKnowledgeGap ItemType = "knowledge_gap_item"
)

type RecallQuality string

const (
	RecallQualityHit          RecallQuality = "hit"
	RecallQualityPartialHit   RecallQuality = "partial_hit"
	RecallQualityMiss         RecallQuality = "miss"
	RecallQualityBadAnswer    RecallQuality = "bad_answer"
	RecallQualityNoAnswerInKB RecallQuality = "no_answer_in_kb"
	RecallQualityAmbiguous    RecallQuality = "ambiguous"
	RecallQualityDuplicate    RecallQuality = "duplicate"
)

type SourceFingerprint struct {
	DocID            string `json:"doc_id"`
	DocVersion       string `json:"doc_version"`
	ChunkID          string `json:"chunk_id"`
	ChunkContentHash string `json:"chunk_content_hash"`
}

type Evidence struct {
	ChunkID string `json:"chunk_id"`
	DocID   string `json:"doc_id"`
	Quote   string `json:"quote"`
	Supports string `json:"supports"`
}

type DeepSearchStep struct {
	Step        int    `json:"step"`
	Tool        string `json:"tool"`
	Query       string `json:"query"`
	Observation string `json:"observation"`
	Decision    string `json:"decision"`
}

type OfflineKnowledgeRun struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	KBID         string    `json:"kb_id"`
	Status       RunStatus `json:"status"`
	WindowStart  time.Time `json:"window_start"`
	WindowEnd    time.Time `json:"window_end"`
	ConfigHash   string    `json:"config_hash"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type QuestionCluster struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	RunID              string    `json:"run_id"`
	KBID               string    `json:"kb_id"`
	CanonicalQuestion  string    `json:"canonical_question"`
	NormalizedQuestion string    `json:"normalized_question"`
	QuestionHash       string    `json:"question_hash"`
	EmbeddingRef       string    `json:"embedding_ref,omitempty"`
	OccurrenceCount    int       `json:"occurrence_count"`
	SampleQuestions    []string  `json:"sample_questions"`
	TraceIDs           []string  `json:"trace_ids"`
	CreatedAt          time.Time `json:"created_at"`
}

type OptimizationItem struct {
	ID                 string              `json:"id"`
	TenantID           string              `json:"tenant_id"`
	RunID              string              `json:"run_id"`
	KBID               string              `json:"kb_id"`
	QuestionClusterID  string              `json:"question_cluster_id"`
	ItemType           ItemType            `json:"item_type"`
	Status             ItemStatus          `json:"status"`
	CanonicalQuestion  string              `json:"canonical_question"`
	FinalAnswer        string              `json:"final_answer,omitempty"`
	RecallQuality      RecallQuality       `json:"recall_quality"`
	FailureType        string              `json:"failure_type,omitempty"`
	Confidence         float64             `json:"confidence"`
	SourceFingerprints []SourceFingerprint `json:"source_fingerprints"`
	Evidence           []Evidence          `json:"evidence"`
	DeepSearchSteps    []DeepSearchStep    `json:"deep_search_steps"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	PublishedAt        time.Time           `json:"published_at,omitempty"`
}
```

- [ ] **Step 2: Add state transition validation**

Add `CanTransition(from, to ItemStatus) bool` to `types.go`. Include forward transitions, regression failed recovery, stale re-validation, and rejected manual reopen.

- [ ] **Step 3: Test state transitions**

Add tests verifying allowed and denied transitions.

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/offlineknowledge -run TestCanTransition -v`

Expected: PASS.

### Task 2: Config And Metrics

**Files:**
- Modify: `internal/config/config.go`
- Modify: `configs/config.example.yaml`
- Modify: `internal/observability/metrics.go`
- Test: `internal/config/config_test.go`
- Test: `internal/observability/metrics_test.go`

- [ ] **Step 1: Add offline knowledge config**

Add `OfflineKnowledgeOrganizerConfig` under existing maintenance config with fields from the design: enabled, schedule, lookback days, max question count, max Codex concurrency, tool quota, shadow TTL, regression thresholds, and conclusion judge flag.

- [ ] **Step 2: Add default values and validation**

Defaults: disabled, schedule `0 2 * * *`, lookback 7 days, max questions 500, max clusters 200, max Codex concurrency 4, max deep search steps 12, shadow TTL 14 days, min verify confidence 0.8, min publish confidence 0.9.

- [ ] **Step 3: Register metrics**

Add counters and gauges for offline runs, extracted questions, clusters, replay count, Codex analysis, deep search steps, validation, item statuses, re-validation, shadow hits, shadow write drops, recall lift, answer quality lift, citation coverage lift, and hallucination risk.

- [ ] **Step 4: Run config and metrics tests**

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/config ./internal/observability -v`

Expected: PASS.

### Task 3: PostgreSQL Schema And Repository

**Files:**
- Create: `migrations/000013_offline_knowledge_optimization.sql`
- Create: `internal/storage/postgres/offline_knowledge.go`
- Test: `internal/storage/postgres/offline_knowledge_test.go`

- [ ] **Step 1: Add migration**

Create tables `offline_knowledge_runs`, `offline_question_clusters`, `optimization_items`, `optimization_item_events`, and partitioned `shadow_retrieval_events`. Include `tenant_id` in every table, `source_fingerprints_json` on items, unique `(tenant_id, kb_id, window_start, window_end, config_hash)` on runs, unique `(tenant_id, kb_id, question_hash)` on question clusters, and indexes for `status`, `kb_id`, `question_hash`, `item_type`, `trace_id`, and `created_at`.

- [ ] **Step 2: Implement repository**

Implement methods: `CreateRun`, `GetRun`, `ListRuns`, `UpsertQuestionCluster`, `CreateOptimizationItem`, `GetOptimizationItem`, `ListOptimizationItems`, `UpdateOptimizationItemStatus`, `AppendItemEvent`, `RecordShadowEvent`, and `ListShadowEvents`.

- [ ] **Step 3: Test tenant isolation and indexes**

Test that tenant A cannot read tenant B runs or items through repository filters. Test duplicate run windows return a conflict-like repository error.

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres -run OfflineKnowledge -v`

Expected: PASS.

### Task 4: Memory Repository And Evidence Validator

**Files:**
- Create: `internal/offlineknowledge/repository.go`
- Create: `internal/offlineknowledge/memory_repository.go`
- Create: `internal/offlineknowledge/validator.go`
- Test: `internal/offlineknowledge/validator_test.go`

- [ ] **Step 1: Define repository and source reader interfaces**

Define `Repository`, `SourceReader`, and `ConclusionJudge` interfaces. `SourceReader` returns chunk text, doc version, chunk content hash, tenant id, and knowledge base id.

- [ ] **Step 2: Implement validator**

Validate tenant, knowledge base, source fingerprint, quote containment, confidence threshold, and answer/evidence consistency. For `answer_item`, call `ConclusionJudge` so final answer conclusions are independently checked against quotes.

- [ ] **Step 3: Test validator rejection cases**

Add tests for missing quote, wrong tenant, wrong KB, stale content hash, low confidence, and failed conclusion judge.

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/offlineknowledge -run Validator -v`

Expected: PASS.

### Task 5: Codex Analyzer Contracts And Tool Governance

**Files:**
- Create: `internal/offlineknowledge/codex.go`
- Test: `internal/offlineknowledge/codex_test.go`

- [ ] **Step 1: Add Codex request and response types**

Define request fields: canonical question, sample questions, tenant id, kb id, baseline recall results, trace summaries, metadata, and constraints. Define response fields matching recall quality, failure type, confidence, final answer, evidence, missing evidence, deep search steps, and recommended action.

- [ ] **Step 2: Add tool contract and quota model**

Define read-only tool names and quota counters: text search, vector search, neighbors, document chunks, graph chunks, eval lookup, existing item lookup, replay recall. Enforce max tool QPS, max rows per call, max steps, max tokens, and timeout before each tool call.

- [ ] **Step 3: Test schema validation**

Test valid Codex output, invalid enum, missing evidence for answer item, and over-budget deep search steps.

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/offlineknowledge -run Codex -v`

Expected: PASS.

### Task 6: Offline Service Orchestration

**Files:**
- Create: `internal/offlineknowledge/service.go`
- Test: `internal/offlineknowledge/service_test.go`

- [ ] **Step 1: Implement run creation**

Create a run using tenant id, kb id, time window, and config hash. Use repository uniqueness to deduplicate repeated windows. Use `__all__` as the knowledge base sentinel when the run covers all knowledge bases.

- [ ] **Step 2: Implement question extraction and clustering hooks**

Define interfaces for history source and clusterer. Include explicit negative feedback and low-priority long-tail queue in the extracted signal model.

- [ ] **Step 3: Implement item build flow**

For each cluster, call recall replay, Codex analyzer, validator, and repository create. Route results to verified, needs_review, knowledge_gap, rejected, or duplicate.

- [ ] **Step 4: Implement re-validation**

Support re-validation by item id and bulk re-validation by tenant, kb, doc id, chunk hash, or status. Move stale items to evidence_validating and then to verified, needs_review, rejected, or deprecated.

- [ ] **Step 5: Run service tests**

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/offlineknowledge -run Service -v`

Expected: PASS.

### Task 7: Shadow Retrieval

**Files:**
- Create: `internal/offlineknowledge/shadow.go`
- Test: `internal/offlineknowledge/shadow_test.go`

- [ ] **Step 1: Implement shadow matching**

Search verified, shadow_enabled, regression_passed, and published optimization items for a tenant and knowledge base. Return matches with score and source `optimization_library`.

- [ ] **Step 2: Enforce answer item behavior**

For `answer_item`, return source chunk references and guidance metadata. Do not return `final_answer` as a direct answer payload.

- [ ] **Step 3: Record bounded shadow events**

Record shadow events through repository with sampling, TTL metadata, and write-drop metrics. Shadow write failure returns the retrieval result and records a dropped event metric.

- [ ] **Step 4: Test shadow behavior**

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/offlineknowledge -run Shadow -v`

Expected: PASS.

### Task 8: HTTP Management API

**Files:**
- Modify: `internal/http/router.go`
- Test: `internal/http/router_test.go`

- [ ] **Step 1: Register routes**

Add routes for runs, run questions, item list, item detail, verify, reject, enable shadow, publish, disable, single revalidate, bulk revalidate, and item eval results.

- [ ] **Step 2: Implement handlers with tenant filters**

Every handler obtains tenant id from the existing request context and passes it into service methods. List endpoints require tenant filter and support status, kb id, item type, and bounded time window filters.

- [ ] **Step 3: Test API behavior**

Test manual run trigger, item list filtering, cross-tenant denial, disable item, and revalidate request.

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/http -run OfflineKnowledge -v`

Expected: PASS.

### Task 9: Eval Regression Integration

**Files:**
- Modify: `internal/offlineknowledge/service.go`
- Modify: `internal/eval/service.go` only if a small runner option is required for optimization overlay.
- Test: `internal/offlineknowledge/service_test.go`
- Test: `internal/eval/service_test.go`

- [ ] **Step 1: Add regression runner interface**

Define an interface that runs paired baseline and with-optimization evaluations and returns recall lift, answer quality lift, citation coverage lift, latency delta, token cost delta, and hallucination risk.

- [ ] **Step 2: Wire regression result to item state**

Move shadow_enabled items to regression_passed when thresholds pass. Move them to regression_failed when lift is too low, latency delta is too high, or hallucination risk exceeds threshold.

- [ ] **Step 3: Require full regression for rewrite rules**

For `query_rewrite_item`, use a full dataset rather than only trigger questions. Regression failure must not publish the rewrite rule.

- [ ] **Step 4: Run regression tests**

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/offlineknowledge ./internal/eval -run Regression -v`

Expected: PASS.

### Task 10: App Wiring, OpenAPI, And Contract Tests

**Files:**
- Modify: `internal/app/app.go`
- Modify: `api/openapi.yaml`
- Modify: `docs/api.md`
- Modify: `docs/api/ingestion-and-query.md`
- Test: `tests/contract/openapi_test.go`
- Test: `tests/contract/examples_test.go`

- [ ] **Step 1: Wire service in app**

Initialize offline knowledge repository, service, validator, Codex analyzer adapter, shadow retriever, and metrics from config.

- [ ] **Step 2: Update OpenAPI**

Add request/response schemas for runs, question clusters, optimization items, evidence, fingerprints, validation report, state transitions, re-validation, and eval results.

- [ ] **Step 3: Update API docs**

Document that `answer_item` is recall guidance only, source validation uses doc version and chunk content hash, and shadow retrieval defaults to non-invasive mode.

- [ ] **Step 4: Run required validation**

Run: `gofmt` on changed Go files.

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/offlineknowledge ./internal/config ./internal/observability ./internal/http ./internal/storage/postgres -v`

Expected: PASS.

Run: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v`

Expected: PASS.

## Self-Review Checklist

- The plan covers nightly runs, history extraction, recall replay hooks, Codex deep analysis contracts, optimization item storage, evidence validation, state machine, APIs, metrics, shadow retrieval, and eval regression.
- The plan includes tenant isolation, content fingerprints, conclusion-level validation, distributed run deduplication, isolated replay resources, tool quotas, rewrite full regression, indexes, shadow event TTL, embedding storage, and long-tail/negative feedback signals.
- The plan avoids direct answer injection from `answer_item`; generated answers still require real chunks in context.
- The plan preserves the required project validation command: `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v`.
