# Tutorial P2 Chunk Candidate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Add the experimental p2_recursive_400_80 Text Quick Pack candidate, a direct P0 fork whose only execution difference is deterministic recursive chunking at 400 tokens with 80-token overlap.

**Architecture:** Tutorial P0, P1, and P2 are Pack-declared sibling variants. The tutorial-owned P0/P1 splitter is pinned at 800/120, P2 uses an app-owned 400/80 ingestor and a separate Knowledge Base. A comparison fingerprint covers shared inputs only; a definition fingerprint and durable run fields record parser and chunking choices. Index-derived chunk count and average token count are persisted and returned separately from ordinary evaluation metrics.

**Tech Stack:** Go 1.26, Hertz/OpenAPI, PostgreSQL with pgx migrations, Qdrant, React/TanStack Query, Vitest, Playwright.

## Global Constraints

- Keep tutorial runtime endpoints and schemas experimental.
- Browser requests contain only variant and idempotency_key; never accept parser, splitter, model, dataset, storage, or evaluator configuration.
- P2 is a direct P0 child. It must not require, select, or inherit a P1 run.
- P0 and P1 tutorial splitters are fixed at 800 tokens and 120 overlap; P2 is fixed at 400 and 80. General ingestion configuration remains independent.
- Candidate Knowledge Bases remain project-owned and distinct from P0 and every other candidate.
- Comparison output contains only persisted evaluation values and measured index statistics. Do not invent cost, latency, confidence, or quality claims.
- Create a new text-rag/1.0.2/quick fixture. Never change the checked-in 1.0.0 or 1.0.1 fixtures.

---

## File Map

| File | Responsibility |
| --- | --- |
| internal/tutorial/manifest.go | Declare and validate the immutable P2 chunk candidate. |
| internal/tutorial/{clone.go,runtime_resources.go} | Project public P2 metadata and provision generic candidate roots. |
| internal/ingest/service.go | Build parser/splitter variants without copying live source locks. |
| internal/tutorial/{run_definition.go,run.go,comparison.go} | Freeze P0/P1/P2 definitions, select ingestors, record measurements, prove direct comparisons. |
| migrations/000031_tutorial_p2_chunk_candidate.sql | Add P2 run audit and measured index-stat columns. |
| internal/{tutorial/clone_memory.go,storage/postgres/tutorial_run.go} | Round-trip all new fields and atomically save index statistics. |
| internal/app/app.go | Construct pinned tutorial P0/P1/P2 ingestion services. |
| api/openapi.yaml | Publish the experimental P2 fields and index-stat comparison contract. |
| console/src/features/tutorials/tutorial-experiment-workbench.tsx | Render P0/P1/P2 truthfully from server declarations. |
| tests/fixtures/tutorial-packs/text-rag/1.0.2/quick/* | Immutable JSON Pack fixture with enough content to create different P0/P2 chunk counts. |
| scripts/console-real-backend-tutorial-clone-e2e.sh and console/e2e/real-backend-tutorial-clone.spec.ts | Verify P0, P1, P2, and comparisons using real PostgreSQL, Qdrant, and browser. |
| docs/tutorials/p2-recursive-chunk-candidate.md | Explain direct-P0 scope, measured statistics, and public Pack publication boundary. |

## Task 1: Declare P2 and provision a generic candidate Knowledge Base

**Files:**
- Modify: internal/tutorial/manifest.go
- Modify: internal/tutorial/manifest_test.go
- Modify: internal/tutorial/clone.go
- Modify: internal/tutorial/clone_test.go
- Modify: internal/tutorial/runtime_resources.go
- Modify: internal/tutorial/runtime_resources_test.go

**Interfaces:**
- Produces TutorialP2RecursiveChunkCandidateID, TutorialP2ChunkingChapter, TutorialBaselineChunkSizeTokens, TutorialBaselineChunkOverlapTokens, TutorialP2ChunkSizeTokens, and TutorialP2ChunkOverlapTokens.
- Extends RuntimeCandidate and public ExperimentVariant with read-only ChunkSizeTokens and ChunkOverlapTokens.
- Keeps the P1 JSON-document requirement and adds P2's exact bounded chunk declaration.

- [ ] **Step 1: Write failing declaration and public-projection tests**

~~~go
func TestManifestAcceptsDeclaredP2RecursiveChunkCandidate(t *testing.T) {
    manifest, err := ParseManifest([]byte(validP2Manifest), textTemplate(), quickPack())
    if err != nil { t.Fatal(err) }
    got := manifest.Runtime.Candidates[1]
    if got.ID != TutorialP2RecursiveChunkCandidateID || got.ChunkSizeTokens != 400 || got.ChunkOverlapTokens != 80 {
        t.Fatalf("candidate = %#v", got)
    }
}

func TestManifestRejectsInvalidP2ChunkDeclaration(t *testing.T) {
    for _, raw := range [][]byte{p2WrongChapter, p2WrongParser, p2ZeroSize, p2OverlapAtSize} {
        if _, err := ParseManifest(raw, textTemplate(), quickPack()); !errors.Is(err, ErrManifestInvalid) {
            t.Fatalf("err = %v, want ErrManifestInvalid", err)
        }
    }
}
~~~

- [ ] **Step 2: Run the tests and verify they fail**

Run: go test ./internal/tutorial -run 'TestManifest(AcceptsDeclaredP2|RejectsInvalidP2)' -count=1

Expected: FAIL because P2 constants and chunk fields do not exist.

- [ ] **Step 3: Implement exact candidate validation**

~~~go
const (
    TutorialP2RecursiveChunkCandidateID = "p2_recursive_400_80"
    TutorialP2ChunkingChapter           = "p2_chunking"
    TutorialBaselineChunkSizeTokens     = 800
    TutorialBaselineChunkOverlapTokens  = 120
    TutorialP2ChunkSizeTokens           = 400
    TutorialP2ChunkOverlapTokens        = 80
)

type RuntimeCandidate struct {
    ID                 string
    Chapter            string
    ParserMethod       string
    ChunkSizeTokens    int
    ChunkOverlapTokens int
}

func validP2Candidate(candidate RuntimeCandidate) bool {
    return candidate.ID == TutorialP2RecursiveChunkCandidateID &&
        candidate.Chapter == TutorialP2ChunkingChapter &&
        candidate.ParserMethod == "basic" &&
        candidate.ChunkSizeTokens == TutorialP2ChunkSizeTokens &&
        candidate.ChunkOverlapTokens == TutorialP2ChunkOverlapTokens
}
~~~

Make validateRuntimeCandidates accept only the existing P1 shape or validP2Candidate, reject nonzero chunk fields on P1, preserve duplicate-ID rejection, and use generic candidate Knowledge Base title/description in RuntimeResources.

- [ ] **Step 4: Verify the focused suite passes**

Run: go test ./internal/tutorial -run 'TestManifest|TestPublicExperiment|TestRuntimeResources' -count=1

Expected: PASS; public responses include only declared ID/chapter/parser/chunk values and no Pack object location.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tutorial/manifest.go internal/tutorial/manifest_test.go internal/tutorial/clone.go internal/tutorial/clone_test.go internal/tutorial/runtime_resources.go internal/tutorial/runtime_resources_test.go
git commit -m "feat: declare tutorial P2 chunk candidate"
~~~

## Task 2: Pin tutorial splitters and derive direct-P0 definitions

**Files:**
- Modify: internal/ingest/service.go
- Modify: internal/ingest/service_test.go
- Modify: internal/tutorial/run_definition.go
- Modify: internal/tutorial/run.go
- Modify: internal/tutorial/run_test.go
- Modify: internal/app/app.go

**Interfaces:**
- ingest.NewVariantService(base *Service, documentParser parser.Parser, splitter chunker.Recursive) *Service.
- ExperimentRun gains ChunkSizeTokens and ChunkOverlapTokens.
- ConfigureCandidateIngestors maps immutable candidate IDs, not parser method names, to app-owned ingestors.

- [ ] **Step 1: Write failing P0/P1/P2 splitter tests**

~~~go
func TestLiveRunP2UsesDirectP0AndFixedRecursiveSplitter(t *testing.T) {
    service, repo, baselineIngest, p2Ingest := newP2Service(t)
    baseline := runToCompletion(t, service, repo, "baseline")
    candidate := runToCompletion(t, service, repo, TutorialP2RecursiveChunkCandidateID)
    if candidate.BaselineRunID != baseline.ID || candidate.ParserMethod != "basic" {
        t.Fatalf("candidate lineage = %#v", candidate)
    }
    if candidate.ChunkSizeTokens != 400 || candidate.ChunkOverlapTokens != 80 || p2Ingest.calls != 1 || baselineIngest.calls != 1 {
        t.Fatalf("candidate = %#v", candidate)
    }
}

func TestLiveRunP1KeepsPinnedP0Chunking(t *testing.T) {
    run := startP1AfterBaseline(t)
    if run.ChunkSizeTokens != 800 || run.ChunkOverlapTokens != 120 {
        t.Fatalf("P1 chunking = %d/%d", run.ChunkSizeTokens, run.ChunkOverlapTokens)
    }
}
~~~

- [ ] **Step 2: Run tests and verify they fail**

Run: go test ./internal/tutorial ./internal/ingest -run 'TestLiveRunP2UsesDirectP0|TestLiveRunP1KeepsPinnedP0Chunking' -count=1

Expected: FAIL because candidates are selected only by parser method and no splitter audit values exist.

- [ ] **Step 3: Implement app-owned fixed variants**

~~~go
func NewVariantService(base *Service, documentParser parser.Parser, splitter chunker.Recursive) *Service {
    if base == nil { return nil }
    return &Service{
        Parser: documentParser, Splitter: splitter, Embedder: base.Embedder,
        Contextualizer: base.Contextualizer, RAPTORBuilder: base.RAPTORBuilder,
        GraphBuilder: base.GraphBuilder, KnowledgeBases: base.KnowledgeBases,
        Indexer: base.Indexer, Jobs: base.Jobs, Uploads: base.Uploads,
        MaxDocumentBytes: base.MaxDocumentBytes,
    }
}
~~~

Initialize every tutorial definition as basic/800/120. P1 changes only parser_method to structured_json. P2 changes only chunk configuration to 400/80. Include chunk values in the definition-fingerprint payload but exclude parser/chunk values from comparison input so direct P0 discovery succeeds. In app.go build a Basic/800/120 tutorial base service, then register P1 and P2 by candidate ID. Do not reuse the configurable general-ingestion splitter for tutorial Live Runs.

- [ ] **Step 4: Verify unit and app wiring tests pass**

Run: go test ./internal/tutorial ./internal/ingest ./internal/app -count=1

Expected: PASS; only P2 differs from P0 in splitter fields and P1 retains P0 splitter fields.

- [ ] **Step 5: Commit**

~~~bash
git add internal/ingest/service.go internal/ingest/service_test.go internal/tutorial/run_definition.go internal/tutorial/run.go internal/tutorial/run_test.go internal/app/app.go
git commit -m "feat: run pinned tutorial chunk candidates"
~~~

## Task 3: Persist measured chunk facts and allow generic direct comparisons

**Files:**
- Create: migrations/000031_tutorial_p2_chunk_candidate.sql
- Modify: internal/ingest/chunker/chunker.go
- Modify: internal/ingest/chunker/chunker_test.go
- Modify: internal/tutorial/run.go
- Modify: internal/tutorial/clone_memory.go
- Modify: internal/tutorial/comparison.go
- Modify: internal/tutorial/run_test.go
- Modify: internal/storage/postgres/tutorial_run.go

**Interfaces:**
- chunker.TokenCount(markdown string) int returns the same deterministic text-unit count used by Recursive.
- ExperimentRunRepository.RecordExperimentRunIndexStats(ctx, tenantID, runID string, chunkCount int, averageChunkTokens float64, now time.Time) (ExperimentRun, bool, error).
- ExperimentRunComparison.IndexMetrics []ExperimentMetricDelta contains chunk_count and average_chunk_tokens.

- [ ] **Step 1: Write failing persistence and comparison tests**

~~~go
func TestLiveRunP2PersistsMeasuredChunkStats(t *testing.T) {
    run := runToCompletion(t, newP2RunService(t), TutorialP2RecursiveChunkCandidateID)
    if run.IndexedChunkCount < 2 || run.AverageChunkTokens <= 0 {
        t.Fatalf("index stats = %#v", run)
    }
}

func TestLiveRunComparisonAllowsP2AndReportsIndexMetrics(t *testing.T) {
    comparison := completeAndCompareP2(t)
    if !comparison.Comparable || metric(comparison.IndexMetrics, "chunk_count") == nil {
        t.Fatalf("comparison = %#v", comparison)
    }
}
~~~

- [ ] **Step 2: Run tests and verify they fail**

Run: go test ./internal/tutorial ./internal/ingest/chunker ./internal/storage/postgres -run 'TestLiveRunP2Persists|TestLiveRunComparisonAllowsP2' -count=1

Expected: FAIL because index results are discarded and PostgreSQL has no columns for splitter or measured statistics.

- [ ] **Step 3: Add the migration and atomic statistic update**

~~~sql
-- +goose Up
ALTER TABLE tutorial_experiment_runs
  ADD COLUMN chunk_size_tokens INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN chunk_overlap_tokens INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN indexed_chunk_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN average_chunk_tokens DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
  DROP COLUMN average_chunk_tokens,
  DROP COLUMN indexed_chunk_count,
  DROP COLUMN chunk_overlap_tokens,
  DROP COLUMN chunk_size_tokens;
~~~

Accumulate len(result.Chunks) and chunker.TokenCount(chunk.Content) in LiveRunService.index. Save aggregate values before advancing to evaluation. Add all columns to every insert, returning/select scanner, and memory clone. Permit RecordExperimentRunIndexStats only while status is running and stage is index_private_pack. runsComparable must accept only direct P0 children with a valid P1 or P2 stored shape, equal shared fingerprints, evaluation IDs, and measured stats.

- [ ] **Step 4: Verify durable comparison coverage**

Run: go test ./internal/tutorial ./internal/storage/postgres ./internal/ingest/chunker -count=1

Expected: PASS; P2 returns ordinary evaluation deltas plus persisted index deltas, while legacy P0 records remain non-comparable.

- [ ] **Step 5: Commit**

~~~bash
git add migrations/000031_tutorial_p2_chunk_candidate.sql internal/ingest/chunker/chunker.go internal/ingest/chunker/chunker_test.go internal/tutorial/run.go internal/tutorial/clone_memory.go internal/tutorial/comparison.go internal/tutorial/run_test.go internal/storage/postgres/tutorial_run.go
git commit -m "feat: audit tutorial chunk candidate indexes"
~~~

## Task 4: Publish P2 through OpenAPI, Console, fixture, docs, and real E2E

**Files:**
- Modify: api/openapi.yaml
- Modify: internal/http/router_test.go
- Modify: console/src/api/client.ts
- Modify: console/src/api/schema.d.ts
- Modify: console/src/test/handlers.ts
- Modify: console/src/features/tutorials/tutorial-experiment-workbench.tsx
- Modify: console/src/features/tutorials/tutorials.test.tsx
- Modify: console/src/tutorials.css
- Create: tests/fixtures/tutorial-packs/text-rag/1.0.2/quick/manifest.json
- Create: tests/fixtures/tutorial-packs/text-rag/1.0.2/quick/corpus/service.json
- Modify: scripts/console-real-backend-tutorial-clone-e2e.sh
- Modify: console/e2e/real-backend-tutorial-clone.spec.ts
- Create: docs/tutorials/p2-recursive-chunk-candidate.md
- Modify: docs/tutorials/clone-and-pack-install.md
- Modify: README.md
- Modify: ROADMAP.md
- Modify: ROADMAP_EN.md
- Modify: CHANGELOG.md

**Interfaces:**
- TutorialExperimentVariant and TutorialExperimentRun expose read-only chunk configuration and index statistics.
- TutorialExperimentRunComparison.index_metrics is an array of TutorialExperimentMetricDelta.
- Console labels derive from variant.id and variant.chapter; UI never infers availability or submits chunk values.

- [ ] **Step 1: Write failing HTTP and Console assertions**

~~~go
func TestTutorialCloneRoutesRunP0ThenP2AndCompareMeasuredChunks(t *testing.T) {
    baseline := startAndComplete(t, handler, "baseline")
    p2 := startAndComplete(t, handler, "p2_recursive_400_80")
    comparison := performJSON(handler, "GET", comparisonURL(p2.ID), "", token)
    if comparison.Code != http.StatusOK || !strings.Contains(comparison.Body, "\"chunk_count\"") {
        t.Fatalf("comparison = %d %s", comparison.Code, comparison.Body)
    }
    if strings.Contains(comparison.Body, "object_key") || baseline.ID == "" { t.Fatal("unexpected response") }
}
~~~

~~~tsx
it('runs P0 then P2 and renders measured chunk differences', async () => {
  useTutorialLiveRunHandlers()
  renderApp('/projects/prj_clone/tutorial/experiments/texp_clone')
  await user.click(await screen.findByRole('button', { name: '运行 P0 基线' }))
  await user.click(await screen.findByRole('button', { name: '运行 P2 切分候选' }))
  expect(await screen.findByRole('heading', { name: 'P0 与 P2 使用相同的对比输入' })).toBeVisible()
  expect(screen.getByText('chunk_count')).toBeVisible()
})
~~~

- [ ] **Step 2: Run focused contract/UI tests and verify they fail**

Run: go test ./internal/http -run TestTutorialCloneRoutesRunP0ThenP2AndCompareMeasuredChunks -count=1

Run: npm --prefix console test -- --run src/features/tutorials/tutorials.test.tsx

Expected: FAIL because OpenAPI/client/UI contain P1-specific labels and no P2/index-metric contract.

- [ ] **Step 3: Implement contract and generic UI**

~~~yaml
TutorialExperimentRunComparison:
  type: object
  required: [baseline, candidate, comparable]
  properties:
    metrics:
      type: array
      items: {$ref: "#/components/schemas/TutorialExperimentMetricDelta"}
    index_metrics:
      type: array
      description: Persisted index measurements, not inferred quality values.
      items: {$ref: "#/components/schemas/TutorialExperimentMetricDelta"}
~~~

Use an explicit variant-copy map keyed by baseline, p1_structured_json, and p2_recursive_400_80. Render P2 as Recursive 400 / overlap 80 and show server-returned chunk values and IndexMetrics in their own table. Retain direct-P0 baseline error wording. Generate schema.d.ts only with npm --prefix console run api:generate.

Create the 1.0.2 JSON fixture with P1 and P2 declarations and more than 800 deterministic text units. The E2E script may copy that fixture into its temporary 1.0.0 catalog alias only because the embedded production catalog remains on public 1.0.0; it must not edit source fixtures or deploy an official Pack. Playwright completes P0, P1, P2, and both comparisons.

- [ ] **Step 4: Document boundaries and run the complete validation matrix**

Document that P2 is a direct P0 fork, P1/P2 do not compose, and index metrics are measured facts rather than quality claims. Preserve the separately documented official public-Pack release requirement and call 1.0.2 an unpublished controlled fixture.

Run:

~~~bash
go test ./...
make vet openapi-validate sdk-check docs-build
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run api:generate
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run typecheck
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console test -- --run
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run build
ORAG_NODE_BIN="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node" make console-real-tutorial-clone-e2e
git diff --check
~~~

Expected: every command exits zero; real browser coverage proves P0/P1/P2 isolated indexes and P0-to-P1/P2 comparison evidence.

- [ ] **Step 5: Commit**

~~~bash
git add api/openapi.yaml internal/http/router_test.go console/src/api/client.ts console/src/api/schema.d.ts console/src/test/handlers.ts console/src/features/tutorials/tutorial-experiment-workbench.tsx console/src/features/tutorials/tutorials.test.tsx console/src/tutorials.css tests/fixtures/tutorial-packs/text-rag/1.0.2 scripts/console-real-backend-tutorial-clone-e2e.sh console/e2e/real-backend-tutorial-clone.spec.ts docs/tutorials/p2-recursive-chunk-candidate.md docs/tutorials/clone-and-pack-install.md README.md ROADMAP.md ROADMAP_EN.md CHANGELOG.md
git commit -m "feat: expose tutorial P2 chunk comparisons"
~~~

## Self-Review

- Spec coverage: Task 1 constrains Pack declaration and separate roots; Task 2 makes P0/P1/P2 splitter choices fixed and server-owned; Task 3 persists actual chunk facts and protects direct P0 comparison; Task 4 publishes, documents, and proves the full flow.
- Placeholder scan: constants, function names, migration columns, test names, fixture version, endpoint schema, and validation commands are explicit.
- Type consistency: p2_recursive_400_80, p2_chunking, chunk_size_tokens, chunk_overlap_tokens, indexed_chunk_count, average_chunk_tokens, and index_metrics use the same names throughout.

## Execution Mode

Execute tasks inline in this worktree. Each task produces a reviewable commit and must pass its focused tests before the next task begins.
