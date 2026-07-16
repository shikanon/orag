# Tutorial P3 contextual-retrieval candidate implementation plan

> **For Codex:** Execute this plan in order on `codex/tutorial-p3-contextual-candidate`. Keep each task independently testable and commit its coherent changes before proceeding.

**Goal:** Ship `p3_contextual_retrieval` as a direct-P0 tutorial candidate that changes only deterministic, server-owned contextualization and reports auditable index facts.

**Architecture:** The tutorial manifest declares an exact P3 shape; app wiring builds an isolated ingest service for every tutorial variant so global ingestion configuration cannot alter the experiment. The run state records contextualization facts measured from the actual indexed chunks, and comparison exposes those facts only for a valid direct P0/P3 pair. API, Console, fixture, documentation, and real browser walkthrough consume the same read-only contract.

**Tech stack:** Go, PostgreSQL migrations, Qdrant, OpenAPI 3.1, React/TypeScript Console, Playwright, Docker Compose, GitHub Actions.

---

### Task 1: Declare and validate the exact P3 candidate

**Files:**
- Modify: `internal/tutorial/manifest.go`
- Modify: `internal/tutorial/clone.go`
- Modify: `internal/tutorial/manifest_test.go`
- Modify: `internal/tutorial/clone_test.go`

**Step 1: Write failing manifest tests.**

Add an accepted P3 runtime candidate with ID and chapter `p3_contextual_retrieval`, Basic parser, inherited 800/120 splitting, and `contextual_retrieval: true`. Add rejected cases for disabled P3 contextualization, changed splitter values, and an unrelated candidate that enables contextualization.

**Step 2: Add the declaration to the runtime contract.**

Introduce `TutorialP3ContextualRetrievalCandidateID`, `TutorialP3ContextualRetrievalChapter`, `TutorialP3ContextualPromptVersion`, fixed P3 limits, and a server-owned P3 system prompt. Add `ContextualRetrieval bool` to `RuntimeCandidate` and public `ExperimentVariant`. Preserve zero/false behavior for the baseline, P1, and P2.

**Step 3: Enforce exact validation.**

Extend manifest validation so P1 and P2 reject contextual retrieval and P3 accepts only its documented shape. Continue rejecting arbitrary candidate IDs and combinations.

**Step 4: Verify.**

Run:

```bash
go test ./internal/tutorial -run 'Test.*(Manifest|Clone|Variant)' -count=1
```

**Step 5: Commit.**

```bash
git add internal/tutorial/manifest.go internal/tutorial/clone.go internal/tutorial/manifest_test.go internal/tutorial/clone_test.go
git commit -m "feat: declare tutorial P3 contextual candidate"
```

### Task 2: Isolate P3 ingestion and bind it to the stored definition

**Files:**
- Modify: `internal/ingest/service.go`
- Modify: `internal/ingest/service_test.go`
- Modify: `internal/ingest/contextual.go`
- Modify: `internal/ingest/contextual_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/tutorial/run_definition.go`
- Modify: `internal/tutorial/run_definition_test.go`

**Step 1: Write failing variant-service tests.**

Cover a variant service that deliberately has no contextualizer even when the base service has one, and a variant service that uses the supplied contextualizer while retaining shared embedder, store, and meter dependencies.

**Step 2: Make contextualizer selection explicit.**

Change `ingest.NewVariantService` to accept a `Contextualizer` argument and assign it directly. Add optional `SystemPrompt` support to `LLMContextualizer`, preserving the current built-in prompt when it is empty. Test that the supplied prompt is sent as the system message.

**Step 3: Wire all tutorial variants.**

In `internal/app/app.go`, build P0, P1, and P2 services with a nil contextualizer. Build P3 with Basic parsing, 800/120 splitting, the configured chat model, fixed P3 limits and prompt, and `ContextualFailureFail`.

**Step 4: Make replay identity include the contextual contract.**

Add contextualization enabled and prompt-version fields to `runtimeDefinition`, its fingerprint payload, equality method, and legacy baseline checks. Candidate definitions obtain the declaration from the runtime manifest; only P3 receives the fixed prompt version.

**Step 5: Verify.**

Run:

```bash
go test ./internal/ingest ./internal/tutorial ./internal/app -run 'Test.*(Variant|Contextual|Definition)' -count=1
```

**Step 6: Commit.**

```bash
git add internal/ingest/service.go internal/ingest/service_test.go internal/ingest/contextual.go internal/ingest/contextual_test.go internal/app/app.go internal/tutorial/run_definition.go internal/tutorial/run_definition_test.go
git commit -m "feat: isolate tutorial contextual ingestion"
```

### Task 3: Persist actual contextualization facts and compare direct P0/P3 runs

**Files:**
- Add: `migrations/000032_tutorial_p3_contextual_candidate.sql`
- Modify: `internal/tutorial/run.go`
- Modify: `internal/tutorial/run_test.go`
- Modify: `internal/tutorial/comparison.go`
- Modify: `internal/tutorial/comparison_test.go`
- Modify: `internal/storage/postgres/tutorial_run.go`
- Modify: `internal/storage/postgres/tutorial_clone_memory.go`
- Modify: `internal/storage/postgres/tutorial_clone_test.go`

**Step 1: Write failing run and comparison tests.**

Use a recording P3 ingestor that returns chunks containing `ContextualText`. Assert a completed P3 run records the enabled flag, count, and deterministic average. Assert P3 cannot start before P0 and cannot be comparable when its contextual facts are absent. Assert a valid P0/P3 comparison contains the two existing chunk metrics plus `contextualized_chunk_count` and `average_context_tokens`.

**Step 2: Add durable fields and measured index statistics.**

Add the three contextual audit fields to `ExperimentRun`. Extend internal index statistics to count non-empty `ContextualText` values and total `chunker.TokenCount` values. Persist an `ExperimentRunIndexStats` value atomically with the index-stage transition instead of expanding positional repository arguments.

**Step 3: Add database and memory persistence.**

Add migration 000032 with non-null defaults for the three fields. Update PostgreSQL INSERT, SELECT, scan, stage transition, clone-memory behavior, and migration coverage to preserve them.

**Step 4: Restrict comparability.**

Retain existing P1/P2 comparison behavior. Add exact P3 validation requiring a matching direct P0 baseline, matching shared fingerprint, P3 enabled flag, and positive contextual facts. Append P3 contextual index metrics only when either compared run has contextualization facts.

**Step 5: Verify.**

Run:

```bash
go test ./internal/tutorial ./internal/storage/postgres -run 'Test.*(P3|Contextual|ExperimentRun|Comparison|Migration)' -count=1
```

**Step 6: Commit.**

```bash
git add migrations/000032_tutorial_p3_contextual_candidate.sql internal/tutorial/run.go internal/tutorial/run_test.go internal/tutorial/comparison.go internal/tutorial/comparison_test.go internal/storage/postgres/tutorial_run.go internal/storage/postgres/tutorial_clone_memory.go internal/storage/postgres/tutorial_clone_test.go
git commit -m "feat: audit tutorial P3 contextual runs"
```

### Task 4: Expose the immutable contract through API, Console, fixture, and docs

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `console/src/api/generated-schema.ts`
- Modify: `console/src/features/tutorials/*`
- Modify: `console/src/features/tutorials/*.test.tsx`
- Add: `tests/fixtures/tutorial-packs/text-rag/1.0.3/quick/*`
- Modify: `scripts/console-real-backend-tutorial-clone-e2e.sh`
- Modify: `internal/app/http_test.go`
- Add: `docs/tutorials/p3-contextual-retrieval-candidate.md`
- Modify: `README.md`
- Modify: `docs/README.md`
- Modify: `docs/tutorials/tutorial-clone.md`
- Modify: `docs/api/tutorial-clone.md`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP.zh-CN.md`
- Modify: `CHANGELOG.md`

**Step 1: Extend the public read model.**

Add read-only `contextual_retrieval`, `contextual_retrieval_enabled`, `contextualized_chunk_count`, and `average_context_tokens` fields to the OpenAPI schemas. Regenerate the Console schema and update its API fixtures. Keep the start request restricted to variant and idempotency key.

**Step 2: Render P3 without client-side configuration.**

Use IDs and chapters from the API to label P3. Show contextualization status and render contextual index metrics only when the server returns them. Add Console tests proving the browser cannot alter prompt, model, limits, or contextualization behavior.

**Step 3: Add controlled 1.0.3 pack and HTTP coverage.**

Create the immutable fixture with computed SHA-256 checksum and P1/P2/P3 declarations. Update the real walkthrough’s local catalog mapping, then add HTTP contract tests for P0 followed by P3 and its read-only audit values.

**Step 4: Document the experiment and its boundary.**

Document that P3 is direct-P0, strict-fail, server-owned, and measures index facts rather than quality superiority. Link it from the README, tutorial index, API guide, phase-based roadmap, and changelog.

**Step 5: Verify.**

Run:

```bash
make fmt
go test ./...
npm --prefix console ci --ignore-scripts
npm --prefix console run generate:api
npm --prefix console test -- --runInBand
npm --prefix console run build
ORAG_NODE_BIN="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node" make console-real-tutorial-clone-e2e
```

**Step 6: Commit.**

```bash
git add api/openapi.yaml console tests/fixtures/tutorial-packs/text-rag/1.0.3 scripts/console-real-backend-tutorial-clone-e2e.sh internal/app/http_test.go docs README.md ROADMAP.md ROADMAP.zh-CN.md CHANGELOG.md
git commit -m "feat: publish tutorial P3 contextual walkthrough"
```

### Task 5: Review, publish, and deploy

**Files:**
- Review all changes in this branch.

**Step 1: Review the diff and repository state.**

Run:

```bash
git diff origin/main...HEAD --check
git status --short
go vet ./...
```

**Step 2: Publish the branch and open a pull request.**

Use the GitHub publishing workflow to push `codex/tutorial-p3-contextual-candidate` and create a PR targeting `main`. Wait for all required GitHub Actions checks and resolve failures before merge.

**Step 3: Merge and synchronize.**

Merge with a merge commit after checks pass, fast-forward the canonical local `main` to `origin/main`, and verify it is clean except for the user-owned `.superpowers/` directory.

**Step 4: Deploy documentation atomically.**

On `8.134.24.116`, build the documentation release from the merged commit under `/var/www/orag-docs-releases/<commit>`, switch `/var/www/orag-docs` atomically, reload nginx after config validation, and verify the root site plus the P3 tutorial URL over HTTPS.

**Step 5: Clean up the completed branch.**

Remove the P3 worktree and local branch only after merge and deployment verification. Preserve unrelated worktrees.
