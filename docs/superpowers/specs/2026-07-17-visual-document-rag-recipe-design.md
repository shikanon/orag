# Visual-document RAG Recipe Pack design

**Status:** approved design; implementation has not started

## Purpose and boundary

ORAG will make the `visual-document-rag` tutorial reproducible without
republishing upstream ViDoSeek PDFs, page images, annotations, or derived
corpora. The public artifact is an immutable **Recipe Manifest**. After a
user explicitly accepts the upstream licence, ORAG downloads the pinned
upstream objects directly into that user's project-private storage, verifies
them, creates deterministic page assets, and runs only a declared visual
experiment.

This design is intentionally separate from the existing text Pack contract:
the latter distributes immutable objects through the public Pack catalog and
its runtime declaration is text-only. A visual Recipe must not be accepted as
a text Pack, and a visual run must not silently fall back to text ingestion or
a text retriever.

## Pinned upstream source

The first tutorial release is `visual-document-rag/1.0.0` and uses ViDoSeek
from the following immutable upstream state:

| Field | Value |
| --- | --- |
| Dataset | `Qiuchen-Wang/ViDoSeek` |
| Revision | `e91a92ba5f38690696c7e66be5c5474b54c6e791` |
| Licence | `Apache-2.0` |
| Quick corpus | `vidoseek_pdf_document.zip` |
| Quick annotations | `vidoseek.json` |
| PDF archive size | `758769613` bytes |
| PDF archive SHA-256 | `3b999a798ceab38703118e4cc7d9b852f86538d5bb7caad64eb545251ee00454` |

The public Recipe records these facts and the annotation hash observed at its
publication. It does not contain credentials, a mutable branch name, an
arbitrary URL, or an object-store location. It permits only a fixed HTTPS
Hugging Face resolve URL derived from the dataset ID, revision, and allowlisted
file name. The source is revalidated before every download.

## Public Recipe Manifest

`RecipeManifest` is a new immutable manifest type, distinct from `Manifest`.
It has a template ID, version, tier, source licence, upstream dataset/revision,
bounded source-object declarations, and a visual runtime declaration.

Each source object declares `path`, `bytes`, and `sha256`. Validation rejects
unknown fields, duplicate paths, non-HTTPS sources, a non-40-character Git
revision, an unapproved dataset ID, zero/oversized objects, or an object not
on the tier's allowlist. Recipe objects are source inputs, not public Pack
objects, so the public catalog may expose the Recipe JSON and its checksum but
never the source bytes.

The Quick tier uses only the ViDoSeek archive and annotations. Benchmark is a
separate immutable Recipe version and may add SlideVQA only with its own
declared object hashes and size limits. A later upstream revision always
requires a new tutorial version.

## Private clone and recovery flow

1. The user starts a Clone with the existing explicit licence confirmation and
   idempotency key.
2. The worker obtains and validates the public Recipe, then creates no runtime
   resources until all upstream objects have passed validation.
3. It streams every fixed source object to a private staging location, enforcing
   total-byte, per-object-byte, redirect, timeout, and checksum limits.
4. It safely extracts the fixed ZIP archive into a private staging tree,
   rejecting absolute paths, parent traversal, symlinks, duplicate targets,
   excessive entries, and excess extracted bytes.
5. A deterministic converter creates page assets and stable page evidence IDs;
   annotations create a frozen project dataset. The converter version, recipe
   hash, upstream revision, verified input hashes, and output hashes form an
   immutable runtime-environment fingerprint.
6. Only then are private objects committed and the visual experiment resources
   created. The clone becomes runnable only after both are durable.

Download, checksum, archive, conversion, and resource failures are persisted
as redacted, retryable stage failures. A retry reuses only verified private
inputs and derived outputs whose fingerprints still match; it never trusts a
partial write. API responses, logs, and Console state expose a stage and a
safe error code, never a signed source URL, a private path, or credentials.

## Visual runtime contract and Live Run

`VisualRuntimeManifest` is modality-specific. It declares the fixed page-asset
root, frozen evaluation items, the P0 visual baseline, and the allowed visual
candidate definitions. It does not accept client-selected models, retrieval
parameters, URLs, or storage settings.

P0 uses a declared visual-page retrieval profile. Candidate definitions are
single-variable experiments and encode only server-supported changes, such as
page representation, visual parser mode, multimodal retrieval, or reranking.
Every run records its Recipe fingerprint, visual environment fingerprint,
provider capability identity, dataset fingerprint, candidate ID, and observed
evaluation/index metrics. A visual request sent to the text runtime, an
undeclared candidate, an unavailable visual provider, or an incompatible P0
lineage is rejected before any ingestion or evaluation work starts.

The initial implementation may support deterministic mock visual providers for
test and explicit demo mode. They are marked as mock, cannot be selected by a
production configuration, and never produce an official Replay.

## Official Replay

An official visual Replay is a read-only aggregate snapshot created from a
verified Recipe and a controlled visual environment. It contains the Recipe,
runtime-environment, evaluator, and build fingerprints plus aggregate P0 and
candidate metrics. It does not include upstream PDFs, page images, annotation
text, private object coordinates, prompts, or model credentials. A Replay is
not a claim that an arbitrary user's provider or project will reproduce the
same result.

## Verification and acceptance criteria

- Unit tests reject malformed Recipes, source drift, duplicate or unsafe paths,
  redirects outside the fixed source, oversized downloads, mismatched hashes,
  Zip Slip, symlinks, archive-bomb limits, and cross-modality runtime use.
- Unit tests prove deterministic conversion and environment fingerprints for
  the same verified source inputs.
- Clone tests prove idempotency, safe retry from each persisted stage, tenant
  isolation, and absence of source/private coordinates in public responses.
- Runtime tests prove visual P0/candidate lineage, provider-capability gates,
  and rejection of a text-only fallback.
- A real PostgreSQL + Qdrant browser E2E uses a local fixed-source fixture to
  prove the full private clone and visual P0 path. The production Recipe is
  separately verified against the upstream object metadata and checksums.
- The public documentation explains the licence confirmation, direct upstream
  download, project-private storage, failure recovery, and the distinction
  between a Live Run and an official Replay.

## Explicit non-goals

- ORAG does not mirror, proxy-cache, or publicly host ViDoSeek source or
  derived visual assets.
- This tutorial does not train or fine-tune a model.
- It does not claim that the current text-only runtime supports visual
  documents before the visual runtime contract and adapters are implemented.
- Video RAG remains a separate follow-up design and implementation.
