# Video RAG private benchmark design

**Status:** approved for implementation under the maintainer's standing
autonomy instruction.

## Decision

ORAG will not distribute, mirror, proxy-cache, or download Video-MME media,
annotations, subtitles, or derived frames. The upstream project says that
Video-MME is for academic research and prohibits distribution, publication,
copying, dissemination, and modification without prior approval. The public
artifact is therefore an immutable **Video Benchmark Protocol**, not a Pack.

The `video-rag/1.0.0` tutorial becomes runnable only after the project owner
imports video files and optional subtitle sidecars that they are entitled to
use, then confirms the benchmark licence. ORAG keeps every source, temporal
asset, transcript, event record, and evaluation item private to that project.

## Considered approaches

1. Mirror a curated Video-MME subset. Rejected: it contradicts the upstream
   no-distribution terms.
2. Have Clone download Video-MME directly. Rejected: it still makes ORAG an
   automated copier of restricted media, and the upstream links can change.
3. Publish only a protocol and require an owner-provided private import.
   **Selected:** preserves a useful temporal RAG product without claiming a
   redistribution right ORAG does not have.

## Public protocol

`VideoBenchmarkProtocol` is a strict JSON declaration, pinned to the
Video-MME repository revision and licence URL. It contains only:

- tutorial identity, version and tier;
- benchmark identity and licence acknowledgement text/version;
- bounded temporal sampling parameters;
- a fixed P0 `temporal_page` profile and evaluation schema;
- opaque, project-local source aliases (not URLs, object keys, filenames,
  questions, answers, subtitles, or media hashes).

The protocol has no media URL or upstream file hash because ORAG does not
fetch the restricted corpus. A new upstream benchmark revision requires a new
tutorial version.

## Private import and temporal assets

The project owner imports a private video plus optional timed subtitles through
the existing project-private upload boundary. The server validates media type,
size, source alias, duration metadata, subtitle timestamp order and a
maximum-segment limit before accepting it.

For each accepted video, a server-owned temporal extractor emits deterministic
segments:

```text
segment ID = SHA-256(video digest, start ms, end ms, extractor version)
evidence ID = source alias + "@" + start ms + "-" + end ms
```

Each segment stores only project-private media/frame references, aligned
subtitle text and a server-generated visual description. The extractor never
accepts a browser-supplied timestamp, frame path, provider choice, or storage
coordinate. Segment records include source digest, interval, extractor version
and derived-content digest for retry-safe reuse.

## Runtime and evaluation

`temporal_page` is a modality-specific profile. It creates a project-owned
knowledge base and dataset only after every requested source alias has a
verified private temporal segment set. Its P0 documents are derived segment
descriptions with evidence IDs; they cannot be indexed by the text tutorial
runtime.

The first implementation supports P0 only. It evaluates regular retrieval
quality plus server-calculated temporal evidence facts:

- segment recall at top K;
- absolute timestamp error in milliseconds;
- temporal order accuracy for ordered evidence;
- subtitle alignment coverage.

Candidate IDs, if added later, must be declared by a new protocol version and
may change exactly one of sampling cadence, subtitle inclusion, temporal
retrieval, or reranking. Public Replay is deferred until a controlled run can
publish aggregate metrics without media, transcript, question, answer or
private-coordinate disclosure.

## Failure, privacy and recovery

Import, media validation, subtitle validation, temporal extraction and visual
description failures are durable redacted states. A retry reuses source and
segment outputs only if all persisted digests and extractor version match.
API and Console responses expose source alias, durable stage and safe failure
code; they never reveal an object key, signed URL, local path, media bytes,
subtitle text or model credential.

## Acceptance criteria

- Unit tests reject a Video-MME media URL, source data hash, unknown field,
  mutable revision, invalid tier, oversized sampling interval and non-private
  source declaration.
- Import tests reject unsupported media, invalid subtitle times, overlapping
  non-monotonic segments and alias collisions across projects.
- Extractor tests prove deterministic segment/evidence IDs, subtitle interval
  alignment, source digest reuse and no text-runtime fallback.
- Clone/Run tests prove tenant isolation, idempotency, redacted failures and
  temporal P0 lineage.
- Public documentation identifies the licence boundary and states that users
  must provide their own authorized source copies.

## Non-goals

- ORAG does not claim a right to redistribute Video-MME.
- This tutorial does not train a video model.
- It does not publish a Video-MME Replay until aggregate-only evidence can be
  produced in a controlled, legally authorized environment.
