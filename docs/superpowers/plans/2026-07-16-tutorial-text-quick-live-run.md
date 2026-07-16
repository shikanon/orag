# Tutorial Text Quick Live Run Plan

**Goal:** Extend a verified text Quick Pack clone into a project-scoped, no-key-compatible baseline evaluation. The clone owns private copies; the runtime never reaches back to the public tutorial origin.

**Scope:** This implements one honest, end-to-end baseline slice for `text-rag` Packs that opt into the runtime manifest extension. It creates project-scoped resource roots at clone time, then asynchronously indexes the private text documents and calls the existing evaluation runner. It does not claim P0–P8 candidate ablations, official Replay, visual-document/video execution, or result comparison are complete.

## Design constraints

- Keep the public manifest strict and catalog-bound. Add an optional `runtime` declaration with only relative Pack object paths, a baseline profile, and dataset examples. Old Pack manifests remain installable but are explicitly `runtime_unavailable`.
- The clone creates deterministic project resources (a Knowledge Base and Dataset) only from verified manifest metadata. It never embeds client-supplied content, object locations, credentials, or model settings.
- A Live Run reads verified Pack objects through the configured private store. It uses the existing `ingest.Service` and `eval.Runner`, so the normal document parser, embedding provider, retrieval, trace, and evaluation storage remain the source of truth.
- The client can request only the immutable `baseline` variant. Its profile/top-k/dataset/KB are server-derived, and the result references the persisted standard evaluation run. A project editor starts/cancels; a project viewer reads.
- A durable experiment-run resource returns `202`, has compare-and-swap ownership, restart recovery, redacted failure codes, and a cancellation request. Indexing and evaluation operate under its worker context.
- A fixture Pack with the runtime declaration and deterministic mock providers proves the whole path without real model keys. Production keeps normal provider validation and fails redactedly when the configured runtime cannot execute.

## Delivery steps

1. Extend the Pack manifest/runtime model, strict validation, clone experiment persistence, and clone workflow to create the Knowledge Base/Dataset roots from verified metadata.
2. Add a private Pack reader for local and Aliyun OSS stores; read-only runtime access is derived from the clone job/project and never reaches the browser.
3. Add durable baseline run state and lifecycle worker. It indexes only declared text objects and delegates scoring to `eval.Runner`.
4. Add authenticated HTTP/OpenAPI APIs, Console experiment workbench/progress, and generated client schema.
5. Extend the real mock-backed Compose/Playwright path; run focused, full, SDK, OpenAPI, docs, and Console validation. Update Roadmap wording to record precisely this baseline slice.

## Acceptance criteria

- A text Quick Pack fixture can be cloned, creates a project KB/Dataset, starts a `202` baseline Live Run, indexes private objects, and completes an ordinary evaluation run without a real key.
- An installed Pack without a runtime declaration is readable but rejects Live Run with a stable `runtime_unavailable` code.
- Cross-tenant/project access, browser-supplied resource IDs/profiles, public-origin runtime downloads, and private storage detail leakage are rejected.
- Repeated starts are idempotent per experiment/variant/idempotency key; recovery and cancellation leave terminal state auditable.
