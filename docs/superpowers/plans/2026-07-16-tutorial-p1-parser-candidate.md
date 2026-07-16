# Tutorial P1 Structured JSON Parser Candidate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first honest tutorial P1 candidate: a server-declared structured-JSON parser that uses its own index and can be compared only with a compatible completed P0 baseline.

**Architecture:** A Pack runtime declaration exposes one immutable P1 candidate (`p1_structured_json`) rather than accepting browser-supplied parser configuration. Clone creates a deterministic candidate Knowledge Base; run-time selects a server-owned ingestion service and persists comparison lineage. The comparison endpoint reads both standard evaluation runs and reports only metrics actually measured by ORAG.

**Tech Stack:** Go 1.24, PostgreSQL/Goose, Hertz/OpenAPI, React/TypeScript, TanStack Query, existing `ingest.Service` and `eval.Runner`.

## Global Constraints

- P0 stays `baseline` + `basic`; its stored Pack snapshot, Dataset, profile, top-k, and behavior remain compatible.
- P1 changes exactly one value: parsing declared `.json` Pack documents with `structured_json`; all other inputs are server-derived.
- P1 gets a separate deterministic project Knowledge Base and never replaces the P0 index.
- Candidate start requires a completed P0 run with exact equality of Pack snapshot, Dataset, profile/top-k, server model configuration, and evaluator settings.
- Browser input is limited to `variant` and `idempotency_key`. It cannot supply model, parser, Pack URL, source, resource ID, profile, or evaluator inputs.
- Existing Pack versions are immutable. Undeclared P1 must be shown unavailable, not silently added through code.
- Audit fields are SHA-256 fingerprints and redacted identifiers only: no object coordinates, credentials, keys, or source content.

---

## File Structure

| Path | Responsibility |
| --- | --- |
| `internal/ingest/parser/structured_json.go` | Deterministic JSON-to-Markdown P1 parsing. |
| `internal/ingest/parser/{remote.go,parser_test.go}` | Parser method selection and golden coverage. |
| `internal/ingest/service.go` | Independent app-lifetime candidate ingestor constructor. |
| `internal/tutorial/{manifest.go,runtime_resources.go,clone.go,run.go}` | Immutable variant declaration, roots, public projection, selection, lineage, comparison. |
| `internal/tutorial/{manifest_test.go,runtime_resources_test.go,run_test.go,clone_memory.go}` | Unit coverage and in-memory durable semantics. |
| `migrations/000030_tutorial_p1_parser_candidate.sql` | Forward-only P1 audit columns and lookup index. |
| `internal/storage/postgres/tutorial_run.go` | PostgreSQL persistence and compatible-baseline lookup. |
| `internal/{app/app.go,http/tutorial_clones.go,http/router.go,http/router_test.go}` | Server-owned parser wiring and API route/authorization. |
| `api/openapi.yaml`, `console/src/api/client.ts` | Experimental public contract and generated client. |
| `console/src/features/tutorials/tutorial-experiment-workbench.tsx` | P0/P1 workbench and comparison display. |
| `tests/fixtures/tutorial-packs/text-rag/1.0.1/quick/*` | New immutable JSON Pack fixture; never alter 1.0.0. |
| `docs/tutorials/p1-structured-json-candidate.md` | Method, limits, and Pack publication contract. |

## Task 1: Deterministic structured JSON parsing

**Files:**
- Create: `internal/ingest/parser/structured_json.go`
- Modify: `internal/ingest/parser/remote.go`
- Modify: `internal/ingest/parser/parser_test.go`

**Interfaces:**
- Produces `const MethodStructuredJSON = "structured_json"`.
- Produces `StructuredJSONParser{Fallback BasicParser}` satisfying `Parser`.
- `parser.New(Config{Method: MethodStructuredJSON})` returns this parser.

- [ ] **Step 1: Write the failing golden tests**

```go
func TestStructuredJSONParserProducesCanonicalMarkdown(t *testing.T) {
    doc, err := (StructuredJSONParser{}).Parse(context.Background(), "service.json", []byte(`{"z":2,"service":{"port":8080,"features":["search","eval"]},"a":true}`))
    if err != nil { t.Fatal(err) }
    want := "# a\n\ntrue\n\n# service\n\n## features\n\n- search\n- eval\n\n## port\n\n8080\n\n# z\n\n2"
    if doc.Markdown != want || doc.Metadata["parser_method"] != MethodStructuredJSON { t.Fatalf("doc=%#v", doc) }
}

func TestStructuredJSONParserRejectsMalformedJSONAndFallsBackForText(t *testing.T) {
    p := StructuredJSONParser{}
    if _, err := p.Parse(context.Background(), "bad.json", []byte(`{"open":`)); err == nil { t.Fatal("expected JSON error") }
    doc, err := p.Parse(context.Background(), "notes.txt", []byte("P0-compatible text"))
    if err != nil || doc.Markdown != "P0-compatible text" || doc.Metadata["parser_method"] != MethodBasic { t.Fatalf("doc=%#v err=%v", doc, err) }
}
```

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/ingest/parser -run 'TestStructuredJSONParser' -count=1`

Expected: FAIL because P1 types do not yet exist.

- [ ] **Step 3: Add the minimal parser**

```go
const MethodStructuredJSON = "structured_json"

type StructuredJSONParser struct{ Fallback BasicParser }

func (p StructuredJSONParser) Parse(ctx context.Context, name string, content []byte) (ParsedDocument, error) {
    if strings.ToLower(filepath.Ext(name)) != ".json" { return p.Fallback.Parse(ctx, name, content) }
    decoder := json.NewDecoder(bytes.NewReader(content))
    decoder.UseNumber()
    var value any
    if err := decoder.Decode(&value); err != nil { return ParsedDocument{}, fmt.Errorf("parse JSON %s: %w", name, err) }
    if decoder.More() { return ParsedDocument{}, fmt.Errorf("parse JSON %s: multiple JSON values", name) }
    markdown := renderStructuredJSON(value, 1)
    if strings.TrimSpace(markdown) == "" { return ParsedDocument{}, fmt.Errorf("no text extracted from %s", name) }
    return ParsedDocument{Markdown: markdown, Metadata: map[string]string{"filename": name, "ext": ".json", "parser_method": MethodStructuredJSON}}, nil
}
```

Render map keys lexicographically, retain `json.Number`, use headings for object keys and `- ` for scalar arrays. In `parser.New`, recognize the method before remote parser construction; other extensions delegate to `BasicParser`.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/ingest/parser -count=1`

Expected: PASS including existing Basic/MinerU/Docling tests.

```bash
git add internal/ingest/parser/structured_json.go internal/ingest/parser/remote.go internal/ingest/parser/parser_test.go
git commit -m "feat: add deterministic structured JSON parser"
```

## Task 2: Immutable P1 declaration and independent roots

**Files:**
- Modify: `internal/tutorial/manifest.go`
- Modify: `internal/tutorial/{manifest_test.go,runtime_resources.go,runtime_resources_test.go,clone.go}`

**Interfaces:**
- Produces `RuntimeCandidate{ID, Chapter, ParserMethod}` and public `ExperimentVariant{ID, Chapter, ParserMethod, Available}`.
- `RuntimeManifest.Candidates` is optional; only declared candidates are runnable.
- `candidateKnowledgeBaseID(projectID, templateID, version, candidateID)` is server-derived and stable.

- [ ] **Step 1: Add a failing strict-manifest test**

```go
func TestParseManifestAcceptsOnlyDeclaredP1StructuredJSONCandidate(t *testing.T) {
    raw := []byte(`{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"corpus/service.json","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":2,"content_type":"application/json"}],"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"评测","items":[{"query":"端口","ground_truth":"8080"}]},"candidates":[{"id":"p1_structured_json","chapter":"p1_document_parser","parser_method":"structured_json"}]}}`)
    manifest, err := ParseManifest(raw, template, pack)
    if err != nil || len(manifest.Runtime.Candidates) != 1 { t.Fatalf("manifest=%#v err=%v", manifest, err) }
}
```

Add invalid table cases for a duplicate ID, non-`p1_structured_json` ID, non-`p1_document_parser` chapter, non-`structured_json` method, and missing JSON document.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/tutorial -run TestParseManifestAcceptsOnlyDeclaredP1 -count=1`

Expected: FAIL because strict decoding rejects `candidates`.

- [ ] **Step 3: Implement declaration and roots**

```go
type RuntimeCandidate struct {
    ID string `json:"id"`
    Chapter string `json:"chapter"`
    ParserMethod string `json:"parser_method"`
}
type ExperimentVariant struct {
    ID string `json:"id"`
    Chapter string `json:"chapter,omitempty"`
    ParserMethod string `json:"parser_method"`
    Available bool `json:"available"`
}
```

Validate and defensive-copy candidates. Create the baseline root with its existing ID formula, then each candidate root with:

```go
candidateID := tutorialResourceID("tkb", job.ProjectID, job.TemplateID, job.TemplateVersion, candidate.ID)
metadata["tutorial_variant"] = candidate.ID
metadata["tutorial_parser_method"] = candidate.ParserMethod
```

Expose only `Experiment.Variants` in `GetExperiment`; keep the full Pack manifest private.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/tutorial -run 'Test(ParseManifest|ResourceInitializer)' -count=1`

Expected: PASS; P0 root unchanged, P1 root distinct/idempotent.

```bash
git add internal/tutorial/manifest.go internal/tutorial/manifest_test.go internal/tutorial/runtime_resources.go internal/tutorial/runtime_resources_test.go internal/tutorial/clone.go
git commit -m "feat: declare tutorial parser candidates"
```

## Task 3: Durable lineage and compatible candidate execution

**Files:**
- Create: `migrations/000030_tutorial_p1_parser_candidate.sql`
- Modify: `internal/tutorial/{run.go,run_test.go,clone_memory.go}`
- Modify: `internal/storage/postgres/tutorial_run.go`
- Modify: `internal/ingest/service.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- `Start(ctx, subject, projectID, variant, idempotencyKey)`.
- `ExperimentRun` persists `baseline_run_id`, `comparison_fingerprint`, `definition_fingerprint`, `knowledge_base_id`, `dataset_id`, `profile`, `top_k`, and `parser_method`.
- `FindCompletedBaseline(ctx, tenantID, projectID, experimentID, fingerprint)`.
- `Compare(ctx, subject, projectID, experimentID, candidateRunID)` returns standard metric absolute/delta values.

- [ ] **Step 1: Write failing behavior tests**

```go
func TestLiveRunRequiresCompatibleBaselineForP1AndUsesIndependentIndex(t *testing.T) {
    // Build an installed JSON Pack declaring p1_structured_json.
    // Assert P1-before-P0 is ErrBaselineRequired; complete P0; then assert
    // P1 stores P0 as parent, uses another KB, and selects structured_json.
}
func TestLiveRunRejectsComparisonFingerprintMismatch(t *testing.T) {
    // A completed baseline from another model/config fingerprint cannot parent P1.
}
```

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/tutorial -run 'TestLiveRun(RequiresCompatibleBaselineForP1|RejectsComparisonFingerprintMismatch)' -count=1`

Expected: FAIL because P0 only accepts baseline and stores no lineage.

- [ ] **Step 3: Add the forward-only migration**

```sql
-- +goose Up
ALTER TABLE tutorial_experiment_runs DROP CONSTRAINT tutorial_experiment_runs_variant_check;
ALTER TABLE tutorial_experiment_runs
  ADD CONSTRAINT tutorial_experiment_runs_variant_check CHECK (variant ~ '^[a-z][a-z0-9_]{0,63}$'),
  ADD COLUMN baseline_run_id TEXT NOT NULL DEFAULT '' REFERENCES tutorial_experiment_runs(id),
  ADD COLUMN comparison_fingerprint TEXT NOT NULL DEFAULT '',
  ADD COLUMN definition_fingerprint TEXT NOT NULL DEFAULT '',
  ADD COLUMN knowledge_base_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN dataset_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN profile TEXT NOT NULL DEFAULT '',
  ADD COLUMN top_k INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN parser_method TEXT NOT NULL DEFAULT '';
CREATE INDEX tutorial_experiment_runs_compatible_baseline_idx
  ON tutorial_experiment_runs (tenant_id, project_id, experiment_id, variant, comparison_fingerprint, updated_at DESC)
  WHERE status = 'completed';
-- +goose Down
DROP INDEX IF EXISTS tutorial_experiment_runs_compatible_baseline_idx;
ALTER TABLE tutorial_experiment_runs DROP CONSTRAINT tutorial_experiment_runs_variant_check;
ALTER TABLE tutorial_experiment_runs ADD CONSTRAINT tutorial_experiment_runs_variant_check CHECK (variant IN ('baseline'));
ALTER TABLE tutorial_experiment_runs DROP COLUMN parser_method, DROP COLUMN top_k, DROP COLUMN profile, DROP COLUMN dataset_id, DROP COLUMN knowledge_base_id, DROP COLUMN definition_fingerprint, DROP COLUMN comparison_fingerprint, DROP COLUMN baseline_run_id;
```

Update every PostgreSQL insert/returning/select/scanner and in-memory transition. Preserve idempotency uniqueness.

- [ ] **Step 4: Implement server-owned fingerprint/ingestor selection**

Hash a JSON struct containing manifest SHA-256, template/version/tier, dataset ID, profile/top-k, chat/embedding/rerank/multimodal provider+model, prompt-cache mode, and evaluator settings. Exclude secrets/object locations. The definition fingerprint adds variant/parser method.

Add:

```go
func NewVariantService(base *Service, documentParser parser.Parser) *Service {
    return &Service{Parser: documentParser, Splitter: base.Splitter, Embedder: base.Embedder,
        Contextualizer: base.Contextualizer, RAPTORBuilder: base.RAPTORBuilder, GraphBuilder: base.GraphBuilder,
        KnowledgeBases: base.KnowledgeBases, Indexer: base.Indexer, Jobs: base.Jobs,
        Uploads: base.Uploads, MaxDocumentBytes: base.MaxDocumentBytes}
}
```

Build one app-lifetime structured-JSON service before traffic; never copy a live `sync.Map`. For P1, query a completed matching P0 parent before create, persist that server result, revalidate snapshot/fingerprint on execution, index only the P1 KB, and invoke the same private Pack reader + `eval.Runner`.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/tutorial ./internal/storage/postgres ./internal/app -count=1 && go run ./cmd/oragctl migrate --help >/dev/null`

Expected: PASS; P1 cannot run without compatible P0 and cannot reuse P0 index.

```bash
git add migrations/000030_tutorial_p1_parser_candidate.sql internal/tutorial/run.go internal/tutorial/run_test.go internal/tutorial/clone_memory.go internal/storage/postgres/tutorial_run.go internal/ingest/service.go internal/app/app.go
git commit -m "feat: run auditable tutorial parser candidates"
```

## Task 4: Project-scoped API, UI, fixture, and docs

**Files:**
- Modify: `internal/http/{tutorial_clones.go,router.go,router_test.go}`
- Modify: `api/openapi.yaml`, `console/src/api/client.ts`
- Modify: `console/src/features/tutorials/{tutorial-experiment-workbench.tsx,tutorials.test.tsx}`, `console/src/tutorials.css`
- Create: `tests/fixtures/tutorial-packs/text-rag/1.0.1/quick/{manifest.json,corpus/service.json}`
- Modify: fixture catalog + `scripts/console-real-backend-tutorial-clone-e2e.sh`
- Create: `docs/tutorials/p1-structured-json-candidate.md`
- Modify: `docs/tutorials/clone-and-pack-install.md`, `README.md`

**Interfaces:**
- `POST .../runs` accepts only `{"variant":"baseline|declared-id","idempotency_key":"..."}`.
- `GET .../runs/{run_id}/comparison` returns a completed P1 candidate and its stored P0 parent.
- Metric rows are `{name, baseline, candidate, absolute_delta, relative_delta}`.

- [ ] **Step 1: Write failing HTTP tests**

```go
started := performJSON(h, "POST", runURL, `{"variant":"p1_structured_json","idempotency_key":"p1-before-p0"}`, token)
if started.Code != http.StatusConflict || !strings.Contains(started.Body, `"code":"tutorial_baseline_required"`) { t.Fatalf("%d %s", started.Code, started.Body) }
comparison := performJSON(h, "GET", candidateURL+"/comparison", "", token)
if comparison.Code != http.StatusOK || !strings.Contains(comparison.Body, `"comparable":true`) || !strings.Contains(comparison.Body, `"parser_method":"structured_json"`) { t.Fatalf("%d %s", comparison.Code, comparison.Body) }
```

Also assert cross-project access is 404/403, strict request binding rejects `knowledge_base_id`/parser/profile spoofing, and the candidate run derives resource IDs server-side.

- [ ] **Step 2: Implement contract and generate client**

Add `Variant string` to the private request, pass only trimmed variant/idempotency to the service, and map `ErrBaselineRequired` to `409 tutorial_baseline_required`. Register `GET /projects/:project_id/tutorial-experiments/:experiment_id/runs/:run_id/comparison`; authorize read and resolve experiment/project equality first.

Keep the API experimental. The OpenAPI `variant` description must state values come from `TutorialExperiment.variants`; audit fields are read-only. Do not invent token/cost/latency/confidence data when evaluation did not persist it.

Create a new `1.0.1` JSON-bearing fixture with a computed SHA-256 and the declared P1 candidate; never mutate `1.0.0`. Extend real E2E to clone 1.0.1, complete P0, complete P1, and observe `comparable:true`.

- [ ] **Step 3: Implement workbench behavior**

Render immutable P0 plus declared P1 cards. P0 is first. P1 is disabled until a completed P0 is visible, but server enforcement remains authoritative. Submit only selected variant/idempotency key. Poll only selected nonterminal run. For completed P1 display parser method, source P0 run ID, evaluation run IDs, fingerprints, comparability, metric absolutes/deltas; render `—` for an unavailable metric. P0-only Packs say “此不可变 Pack 未声明 P1 解析候选”, never zero/failed.

- [ ] **Step 4: Document method and Pack publication boundary**

Explain P1 is experimental and applies only to declared Packs; it compares same frozen Pack/Dataset/model fingerprint/profile/top-k/evaluator. Explain mock E2E proves wiring, not quality. The release instruction is: publish a new semantic Pack directory through official Pack CI credentials, verify anonymous HTTPS + SHA-256, update catalog; never overwrite 1.0.0 or put storage credentials in this repo/console.

- [ ] **Step 5: Run the complete gate and commit**

```bash
go test ./...
make vet
make openapi-validate
make sdk-check
make docs-build
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run api:generate
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run typecheck
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console test -- --run
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run build
ORAG_NODE_BIN="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node" make console-real-tutorial-clone-e2e
```

Expected: all commands exit 0 and the real mock P0→P1 path is comparable.

```bash
git add internal/http api/openapi.yaml console tests/fixtures scripts/console-real-backend-tutorial-clone-e2e.sh docs README.md
git commit -m "feat: add tutorial P1 parser workbench"
```

## Self-Review

- Spec coverage: Tasks 1–3 make parser choice the only P1 variable, keep indexes independent, and prove baseline compatibility with durable fingerprints. Task 4 prevents API spoofing, presents truthful comparison output, creates an immutable test Pack, and documents public Pack publication separately from runtime credentials.
- Placeholder scan: paths, type names, migration columns, route, request fields, commands, and expected results are concrete. Official Pack publication is intentionally an external artifact release requiring its separate credentials; it is not simulated as a runtime write.
- Type consistency: `p1_structured_json`, `p1_document_parser`, `structured_json`, `comparison_fingerprint`, and `baseline_run_id` are used consistently.

## Execution Mode

The owner has already authorized direct Roadmap iteration. Execute inline in the isolated `codex/tutorial-p1-parser-candidate` worktree, using the task-level test gates and commits before publishing a PR.

