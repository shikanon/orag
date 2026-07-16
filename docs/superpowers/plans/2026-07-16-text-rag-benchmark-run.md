# Text RAG Benchmark Run Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (\`- [ ]\`) syntax for tracking.

**Goal:** Make the \`text-rag\` Benchmark Pack runnable and reproducible with durable server-owned evidence, Console visibility, and a no-key real browser regression.

**Architecture:** Generalize the Quick-only runtime gate to the two declared \`text-rag\` tiers. A run definition derives Pack, environment, and build evidence from the saved Manifest and app configuration, persists it beside existing tutorial audit facts, and includes it in comparison/definition fingerprints. A controlled \`1.0.9/benchmark\` fixture is served only through the temporary E2E catalog path.

**Tech Stack:** Go, PostgreSQL/goose, Qdrant, OpenAPI 3.0, React/TypeScript, Vite/Vitest/Playwright, Docker Compose.

## Global Constraints

- Only \`text-rag\` Quick and Benchmark Packs become runnable; visual document, video and official Replay remain unavailable.
- Benchmark uses \`high_precision\` with Top-K 8; Quick remains \`realtime\` with Top-K 5.
- \`ORAG_BUILD_REVISION\` is a non-secret, trimmed identifier with default \`dev\`; it must never contain credentials or a private URL.
- P0–P7 Context Pack remains 5/6000 and P8 remains 3/6000 under evaluator v5.
- Browser requests only choose variant and idempotency key; reproducibility evidence is server-owned and read-only.

---

### Task 1: Admit verified Benchmark manifests and create Benchmark runtime roots

**Files:**
- Modify: \`internal/tutorial/manifest.go\`
- Modify: \`internal/tutorial/clone.go\`
- Modify: \`internal/tutorial/runtime_resources.go\`
- Test: \`internal/tutorial/manifest_test.go\`, \`internal/tutorial/clone_test.go\`, \`internal/tutorial/runtime_resources_test.go\`

**Interfaces:**
- Produces: \`supportsTextRuntime(templateID, tier string) bool\`; valid baseline profiles \`realtime\` and \`high_precision\`.

- [ ] **Step 1: Write failing tests**

\`\`\`go
func TestParseManifestAcceptsTextBenchmarkRuntime(t *testing.T) {
    // a text-rag benchmark Manifest with high_precision/top_k 8 parses successfully
}
func TestCloneRunnerCreatesBenchmarkRuntimeResources(t *testing.T) {
    // completed benchmark clone has RuntimeStatus == "ready" and BaselineProfile == "high_precision"
}
\`\`\`

- [ ] **Step 2: Verify failure**

Run: \`go test ./internal/tutorial -run 'Test(ParseManifestAcceptsTextBenchmarkRuntime|CloneRunnerCreatesBenchmarkRuntimeResources)' -count=1\`

Expected: FAIL because only \`realtime\` and \`text-rag/quick\` are admitted.

- [ ] **Step 3: Implement the contract**

\`\`\`go
func supportsTextRuntime(templateID, tier string) bool {
    return templateID == "text-rag" && (tier == "quick" || tier == "benchmark")
}
func validRuntimeProfile(profile string) bool {
    return profile == "realtime" || profile == "high_precision"
}
\`\`\`

Use the new gate in clone resource creation and run-definition admission. In \`ResourceInitializer.Ensure\`, make the benchmark dataset kind \`tutorial_benchmark\`; preserve \`tutorial_baseline\` for Quick.

- [ ] **Step 4: Verify and commit**

Run: \`go test ./internal/tutorial -run 'Test(ParseManifestAcceptsTextBenchmarkRuntime|CloneRunnerCreatesBenchmarkRuntimeResources)' -count=1\`

Expected: PASS.

\`\`\`bash
git add internal/tutorial/manifest.go internal/tutorial/clone.go internal/tutorial/runtime_resources.go internal/tutorial/manifest_test.go internal/tutorial/clone_test.go internal/tutorial/runtime_resources_test.go
git commit -m "feat: enable text benchmark runtime packs"
\`\`\`

### Task 2: Persist server-owned reproducibility evidence

**Files:**
- Modify: \`internal/config/config.go\`, \`internal/app/app.go\`
- Modify: \`internal/tutorial/run.go\`, \`internal/tutorial/run_definition.go\`, \`internal/tutorial/comparison.go\`
- Modify: \`internal/storage/postgres/tutorial_run.go\`
- Test: \`internal/tutorial/run_test.go\`, \`internal/tutorial/run_definition_test.go\`, \`internal/tutorial/comparison_test.go\`, \`internal/storage/postgres/tutorial_clone_test.go\`
- Create: \`migrations/000038_tutorial_benchmark_reproducibility.sql\`

**Interfaces:**
- Produces \`ExperimentRun.PackManifestSHA256\`, \`RuntimeEnvironmentSHA256\`, and \`BuildRevision\`.

- [ ] **Step 1: Write failing tests**

\`\`\`go
func TestBenchmarkDefinitionPersistsReproductionEvidence(t *testing.T) {
    // a benchmark run has manifest SHA, environment SHA and build revision "benchmark-e2e-v1"
}
func TestRunsComparableRejectsDifferentBuildRevision(t *testing.T) {
    // otherwise identical P0/P8 runs are not comparable if BuildRevision differs
}
\`\`\`

Also assert migration columns are non-null, default to \`''\`, and PostgreSQL scanning/insertion preserves their values.

- [ ] **Step 2: Verify failure**

Run: \`go test ./internal/tutorial ./internal/storage/postgres -run 'Test(BenchmarkDefinitionPersistsReproductionEvidence|RunsComparableRejectsDifferentBuildRevision|TutorialBenchmarkReproducibilityMigration)' -count=1\`

Expected: FAIL because evidence fields and migration do not exist.

- [ ] **Step 3: Implement immutable evidence**

\`\`\`go
type RuntimeEnvironment struct {
    // existing provider/model fields
    BuildRevision string \`json:"build_revision"\`
}
type ExperimentRun struct {
    PackManifestSHA256       string \`json:"pack_manifest_sha256,omitempty"\`
    RuntimeEnvironmentSHA256 string \`json:"runtime_environment_sha256,omitempty"\`
    BuildRevision            string \`json:"build_revision,omitempty"\`
}
\`\`\`

Add \`ServerConfig.BuildRevision\`, load \`ORAG_BUILD_REVISION\` with \`dev\` default, trim it and reject empty/over-200/control-character values. Compute Manifest and environment SHA values in \`runtimeDefinition\`, copy the build revision into the run, and include all three values in definition matching and comparison validation. Add them to all PostgreSQL columns, insert placeholders and scanners.

- [ ] **Step 4: Verify and commit**

Run: \`go test ./internal/tutorial ./internal/storage/postgres -run 'Test(BenchmarkDefinitionPersistsReproductionEvidence|RunsComparableRejectsDifferentBuildRevision|TutorialBenchmarkReproducibilityMigration)' -count=1\`

Expected: PASS.

\`\`\`bash
git add internal/config/config.go internal/app/app.go internal/tutorial/run.go internal/tutorial/run_definition.go internal/tutorial/comparison.go internal/tutorial/run_test.go internal/tutorial/run_definition_test.go internal/tutorial/comparison_test.go internal/storage/postgres/tutorial_run.go internal/storage/postgres/tutorial_clone_test.go migrations/000038_tutorial_benchmark_reproducibility.sql
git commit -m "feat: persist tutorial benchmark reproduction evidence"
\`\`\`

### Task 3: Publish the OpenAPI and Console audit surface

**Files:**
- Modify: \`api/openapi.yaml\`, \`console/src/api/schema.d.ts\`
- Modify: \`console/src/features/tutorials/tutorial-experiment-workbench.tsx\`
- Modify: \`console/src/test/handlers.ts\`, \`console/src/features/tutorials/tutorials.test.tsx\`

**Interfaces:**
- Consumes: read-only run evidence fields and \`Experiment.pack_tier\`.
- Produces: Benchmark labels and audit rows without new browser inputs.

- [ ] **Step 1: Write a failing Console test**

\`\`\`tsx
it('shows immutable Benchmark reproduction evidence', async () => {
  // mock high_precision/8 benchmark run renders manifest SHA, environment SHA and benchmark-e2e-v1
})
\`\`\`

- [ ] **Step 2: Extend contract and generate types**

Add three read-only string properties to \`TutorialExperimentRun\`.

Run:
\`\`\`bash
npm --prefix console run api:generate
make openapi-validate
\`\`\`

Expected: generated schema declares all fields and OpenAPI checks pass.

- [ ] **Step 3: Implement UI**

Render \`文本 Benchmark 可复现实验\` when tier is benchmark; retain Quick copy otherwise. Add audit rows for Pack tier, Manifest SHA, environment SHA and build revision, rendering shortened SHA values only. Extend MSW benchmark mocks with non-empty values; do not add any client-side configuration control.

- [ ] **Step 4: Verify and commit**

Run:
\`\`\`bash
npm --prefix console run typecheck
npm --prefix console test -- --run
npm --prefix console run build
\`\`\`

Expected: all pass.

\`\`\`bash
git add api/openapi.yaml console/src/api/schema.d.ts console/src/features/tutorials/tutorial-experiment-workbench.tsx console/src/test/handlers.ts console/src/features/tutorials/tutorials.test.tsx
git commit -m "feat: expose benchmark reproduction audit"
\`\`\`

### Task 4: Add controlled fixture and real browser reproduction

**Files:**
- Create: \`tests/fixtures/tutorial-packs/text-rag/1.0.9/benchmark/corpus/service.json\`
- Create: \`tests/fixtures/tutorial-packs/text-rag/1.0.9/benchmark/corpus/operations.txt\`
- Create: \`tests/fixtures/tutorial-packs/text-rag/1.0.9/benchmark/manifest.json\`
- Create: \`console/e2e/real-backend-tutorial-benchmark.spec.ts\`
- Create: \`scripts/console-real-backend-tutorial-benchmark-e2e.sh\`
- Modify: \`Makefile\`

**Interfaces:**
- Produces: \`make console-real-tutorial-benchmark-e2e\`; fixed \`ORAG_BUILD_REVISION=benchmark-e2e-v1\`.

- [ ] **Step 1: Write browser test first**

\`\`\`ts
test('installs and reproduces a Benchmark Pack P0 to P8 comparison', async ({ page }) => {
  // choose Benchmark Pack; clone; run P0; run P8; assert high_precision/8,
  // 5/6000 -> 3/6000, P0 reuse, evidence rows, and comparable result.
})
\`\`\`

- [ ] **Step 2: Verify failure**

Run: \`npm --prefix console run test:e2e -- e2e/real-backend-tutorial-benchmark.spec.ts\`

Expected: FAIL until the fixture server path and implementation exist.

- [ ] **Step 3: Add fixture and harness**

Create two objects and calculate exact SHA-256/byte values. Manifest must declare \`tier: benchmark\`, \`high_precision\`, Top-K 8, four frozen evaluation items and P1–P8 contracts. Copy the real tutorial script with an independent Compose project, ports and temp directory. Map only fixture \`1.0.9/benchmark\` into temporary \`1.0.0/benchmark\`, rewrite only that temporary Manifest version, set \`ORAG_BUILD_REVISION=benchmark-e2e-v1\`, run the new Playwright spec and verify private objects.

- [ ] **Step 4: Verify and commit**

Run: \`make console-real-tutorial-benchmark-e2e\`

Expected: PostgreSQL/Qdrant/API/Console succeeds and P0/P8 evidence/comparison assertions pass.

\`\`\`bash
git add tests/fixtures/tutorial-packs/text-rag/1.0.9/benchmark console/e2e/real-backend-tutorial-benchmark.spec.ts scripts/console-real-backend-tutorial-benchmark-e2e.sh Makefile
git commit -m "test: add reproducible text benchmark walkthrough"
\`\`\`

### Task 5: Document scope and run release verification

**Files:**
- Modify: \`README.md\`, \`ROADMAP.md\`, \`CHANGELOG.md\`
- Modify: \`docs/tutorials/clone-and-pack-install.md\`, \`docs/api/README.md\`, \`docs/README.md\`, \`docs-site/index.html\`
- Create: \`docs/tutorials/text-rag-benchmark-run.md\`, \`docs-site/tutorials/text-rag-benchmark.html\`

- [ ] **Step 1: Write documentation**

Document the controlled fixture, \`make console-real-tutorial-benchmark-e2e\`, \`high_precision\`/8, the three evidence fields, direct P0→candidate comparison, and external full CRUD-RAG publication requirements. Mark only Benchmark Run complete in ROADMAP; retain Replay and visual/video work as pending.

- [ ] **Step 2: Run full verification**

\`\`\`bash
go test ./...
make vet
make openapi-validate
npm --prefix console run typecheck
npm --prefix console test -- --run
npm --prefix console run build
make console-real-tutorial-benchmark-e2e
./scripts/build-docs-site.sh /tmp/orag-docs-benchmark-preview
\`\`\`

Expected: all pass; the preview includes the benchmark page and OpenAPI evidence fields.

- [ ] **Step 3: Commit**

\`\`\`bash
git add README.md ROADMAP.md CHANGELOG.md docs/tutorials/clone-and-pack-install.md docs/api/README.md docs/README.md docs-site/index.html docs/tutorials/text-rag-benchmark-run.md docs-site/tutorials/text-rag-benchmark.html
git commit -m "docs: publish reproducible text benchmark guide"
\`\`\`

