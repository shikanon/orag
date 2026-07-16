# Tutorial P4 sparse-retrieval candidate design

## Goal

Add `p4_sparse_retrieval` as the first retrieval-only Text Quick Pack experiment. It is a direct, auditable P0 child that changes the retriever from the P0 hybrid baseline to server-owned pure sparse retrieval while reusing P0's completed private index.

## Decision and alternatives

P4 is pure sparse retrieval. This keeps the roadmap boundary clear: P4 evaluates one sparse route; a later P5 may evaluate multi-route or fusion behavior. It is not a hybrid candidate and does not create a second identical index.

Alternatives rejected for this increment:

- A Hybrid P4 would overlap the P5 multi-route/fusion chapter and make the tutorial sequence ambiguous.
- Re-indexing the Pack for P4 would introduce parser, splitter, embedding, storage, and index execution as uncontrolled variables. P4 must reuse the direct P0 index instead.

## Immutable declaration and runtime contract

The Pack declares exactly one P4 candidate:

| Field | Value |
| --- | --- |
| ID | `p4_sparse_retrieval` |
| Chapter | `p4_sparse_retrieval` |
| Parser | `basic` |
| Chunking | 800 / 120 |
| Contextual retrieval | disabled |
| Retrieval strategy | `sparse` |
| Index behavior | reuse compatible completed P0 index |
| Parent | compatible completed P0 only |

`RuntimeCandidate` gains a read-only retrieval strategy declaration. Validation accepts only exact P1, P2, P3, and P4 shapes; arbitrary strategies or combinations are rejected. P0's durable strategy is `hybrid`; P1/P2/P3 retain `hybrid`; P4 is `sparse`.

The start API remains restricted to a Pack-declared variant and idempotency key. It never accepts retriever type, rank limits, RRF, storage coordinates, model values, or fallback behavior.

## Execution and audit model

P4 must find a compatible completed P0 before creation. Its run uses that baseline's knowledge-base ID rather than a derived candidate knowledge-base ID. It records the direct `baseline_run_id`, `retrieval_strategy`, and a read-only `reused_baseline_index` fact. It starts at evaluation rather than indexing, so private Pack objects are not read and P0 data is not overwritten.

The P0 chunk and contextual facts remain the facts of the reused baseline index. P4 does not claim a new index shape. The Console may display them as inherited P0 facts only when the API says the index was reused.

`runtimeDefinition` includes the strategy and index-reuse contract in its definition fingerprint, but excludes them from the shared comparison fingerprint so a compatible P0 is discoverable. Legacy baseline compatibility remains unchanged.

## Candidate evaluator isolation

`LiveRunService` gains an app-owned evaluator mapping in addition to candidate ingestors. P0/P1/P2/P3 continue to use the existing hybrid evaluation runner. App wiring constructs one P4 `eval.Runner` with a shallow, server-built `rag.Service` copy whose retriever is `backend.sparse`; it shares the model, packer, prompt policy, dataset service, and evaluation repository with P0. The Console and caller cannot choose this evaluator.

At execution time, the service resolves an evaluator from the immutable runtime definition. P4's evaluator receives the baseline knowledge base and the same dataset/profile/Top-K. A missing evaluator or a mismatch between stored run and definition fails safely as `runtime_unavailable`.

## Comparison

A P4 comparison is valid only if:

- the candidate directly references a completed P0 with a matching shared fingerprint;
- P0 has strategy `hybrid`, P4 has strategy `sparse`, and P4 records index reuse;
- both use the same P0 knowledge base, data set, profile, and Top-K;
- both have ordinary completed evaluation runs.

P4 comparisons expose standard persisted evaluation deltas. They do not report new P4 indexing measurements, and do not infer quality, cost, latency, or availability superiority from strategy selection.

## API, Console, fixture, and verification

OpenAPI exposes read-only `retrieval_strategy` and `reused_baseline_index` on experiment runs, plus the P4 declaration. Console labels P4 from API ID/chapter, displays the fixed strategy and inherited-index state, and provides no retrieval tuning controls.

Create immutable `tests/fixtures/tutorial-packs/text-rag/1.0.4/quick` with P1 through P4 declarations. The controlled real browser harness maps it locally only, executes P0 then P4, verifies that P4 reports sparse retrieval and P0-index reuse, and verifies a comparable response.

Required coverage includes exact manifest validation, definition and reuse execution tests, sparse evaluator selection tests, P4 comparison rejection tests, PostgreSQL persistence migration coverage, HTTP/OpenAPI/Console contract tests, Console build, and the real PostgreSQL/Qdrant/Playwright walkthrough.

## Non-goals

- No browser-configurable retrieval parameters.
- No P4 re-index, P4-on-P1/P2/P3 inheritance, or baseline mutation.
- No hybrid/RRF behavior in P4; that belongs to later multi-route work.
- No claim that one controlled walkthrough proves sparse retrieval quality superiority.
