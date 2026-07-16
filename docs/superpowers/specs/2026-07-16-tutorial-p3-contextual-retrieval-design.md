# Tutorial P3 Contextual Retrieval Candidate Design

## Goal

Add the first contextual-retrieval tutorial experiment as a Pack-declared P3 candidate. It must be a direct, auditable child of P0 and change only chunk contextualization. It must not inherit P1 parsing or P2 chunking.

## Current state

The tutorial Live Run already supports immutable direct-P0 candidates, distinct candidate knowledge bases, persisted execution definitions, standard-evaluation deltas, and measured chunk-index facts. `ingest.Service` can contextualize every parsed chunk before embedding, but tutorial variant services currently copy the general ingestion contextualizer. That makes contextualization dependent on a global ingestion setting rather than the tutorial declaration, which is incompatible with a single-variable experiment.

P0/P1/P2 use a Basic-or-structured parser plus pinned recursive splitting. P3 must preserve P0's Basic parser and 800/120 splitter, but enable contextualization under a server-owned contract.

## Decision

Declare one exact candidate:

| Field | Value |
| --- | --- |
| ID | `p3_contextual_retrieval` |
| Chapter | `p3_contextual_retrieval` |
| Parser | `basic` |
| Chunking | 800 size / 120 overlap |
| Contextual retrieval | enabled |
| Parent | compatible completed P0 only |

P0, P1, and P2 always run with contextualization disabled. P3 runs a server-built contextualizer with a versioned prompt and fixed limits. It fails the run if contextualization cannot be generated; it never silently falls back to uncontextualized chunks, because that would falsely represent P3 as applied.

## Runtime design

`RuntimeCandidate` gains a read-only `contextual_retrieval` declaration. Manifest validation accepts no arbitrary combination: P1 remains structured JSON with contextualization disabled, P2 remains Basic/400/80 with contextualization disabled, and P3 must match the exact fixed shape above.

`ingest.NewVariantService` accepts an explicit contextualizer override in addition to parser and splitter. The tutorial app factory creates these app-lifetime services:

- P0: Basic parser, 800/120 splitter, no contextualizer.
- P1: structured JSON parser, 800/120 splitter, no contextualizer.
- P2: Basic parser, 400/80 splitter, no contextualizer.
- P3: Basic parser, 800/120 splitter, `tutorial_contextual_v1` contextualizer.

The P3 contextualizer uses the configured chat model but fixed request limits and the strict `fail` policy. The model/provider/evaluator snapshot remains in the comparison fingerprint; the candidate flag and prompt version are included in the definition fingerprint only. This permits compatible P0 discovery while ensuring the stored candidate definition cannot be replayed through a different contextualization contract.

## Durable audit and comparison

`ExperimentRun` gains read-only contextualization audit facts:

- `contextual_retrieval_enabled`
- `contextualized_chunk_count`
- `average_context_tokens`

The index stage obtains these values from the actual `ingest.Result.Chunks`: only non-empty `ContextualText` values count; their deterministic text-unit lengths form the average. The repository persists them atomically while the run is `running/index_private_pack`, alongside existing chunk statistics. PostgreSQL migration and in-memory repository behavior match.

P3 remains comparable only when its stored direct parent is a completed P0 with matching shared fingerprint, normal evaluation IDs, P0's exact baseline shape, P3's exact contextual shape, and actual contextual facts. `index_metrics` then includes existing chunk facts plus `contextualized_chunk_count` and `average_context_tokens`. These are index measurements, not a quality, cost, latency, or confidence claim.

## API and Console

The OpenAPI schemas expose the declaration and run facts as read-only fields. The start request still accepts only the Pack-declared `variant` and an idempotency key. The Console derives P3 labels exclusively from `variant.id` and `variant.chapter`, displays whether contextualization was applied, and renders the additional index facts only when returned by the server. It must not let a browser enable, disable, tune, or supply contextualization content.

## Fixture, documentation, and verification

Create immutable `tests/fixtures/tutorial-packs/text-rag/1.0.3/quick` with a computed checksum and P1/P2/P3 declarations. The existing controlled browser harness temporarily maps that fixture to the local test catalog only; it does not publish or overwrite public Pack versions.

Required validation includes manifest validation, direct-P0 and failure-mode unit tests, persistence/comparison coverage, HTTP contract tests, Console tests, a production Console build, and the real PostgreSQL/Qdrant/Playwright walkthrough. The walkthrough must prove a completed P0 followed by P3 with non-empty contextual facts and a comparable response.

## Non-goals

- No P3-on-P2 or P3-on-P1 inheritance.
- No browser-supplied prompt, limits, failure policy, model, or storage coordinates.
- No Benchmark Run, Replay, visual-document, or video Live Run in this increment.
- No claim that mock walkthrough output establishes P3 quality superiority.
