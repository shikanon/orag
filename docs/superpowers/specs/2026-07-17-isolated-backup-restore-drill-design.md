# Isolated Backup and Restore Drill Design

**Status:** Implemented and integration-verified on 2026-07-17

**Roadmap:** Phase 3 — production-pilot baseline / consistency and execution safety

## Decision

ORAG will provide `make backup-restore-drill`, an opt-in integration runner
that proves a complete isolated recovery of the two retrieval sources of truth:
PostgreSQL metadata and Qdrant primary/semantic-cache collections. It is not a
production backup scheduler and never touches a user deployment.

The runner starts independent source and target Docker Compose projects with
separate named volumes and host ports. It starts a local deterministic-mock API
against the source stack, runs the existing HTTP walkthrough to create a cited
document, trace, dataset and evaluation, then exports a PostgreSQL custom dump
and one Qdrant snapshot per required collection. It creates the existing strict
`orag.backup.v1` manifest and checksum file, verifies them with `oragctl
backup-verify`, and restores all artifacts into an empty target stack.

The target API must then successfully authenticate, execute the original cited
query against the restored knowledge base and fetch its trace. A structured
drill evidence JSON records only non-secret evidence: build revision, backup
manifest fingerprint, source knowledge-base ID, source trace ID, restored
citation count and restored trace ID.

## Why this boundary

The repository currently validates backup artifact shape and documents manual
recovery. That does not prove the dump and vector snapshots can be restored
together, nor that ORAG can retrieve the restored data. A full temporary
source/target exercise closes that verification gap without claiming an
independent production deployment or a disaster-recovery SLA.

## Architecture and data flow

1. A dedicated Compose file runs PostgreSQL 16 and Qdrant 1.11.5 as a source
   or target project. The script chooses two non-overlapping fixed test port
   sets and destroys only its own project-scoped volumes during cleanup.
2. The source API uses explicit mock providers and collection names scoped to
   the runner. `orag-demo` writes the known deterministic walkthrough and a
   summary file identifies the source knowledge base and trace.
3. The runner creates a custom PostgreSQL dump. It requests and downloads
   Qdrant snapshots for the primary and semantic cache collections, archives
   them as `qdrant-snapshots.tgz`, writes SHA-256 checksums and a manifest, and
   verifies that directory before restore.
4. The target receives the PostgreSQL dump, applies bundled migrations, then
   receives each Qdrant collection snapshot through Qdrant's documented
   snapshot-upload endpoint with `priority=snapshot`.
5. The target API uses the recovered stores. It logs in, submits the exact
   known query to the original knowledge base, requires a citation, and reads
   the returned trace. It writes `drill-evidence.json` only after all checks
   pass.

## Failure and safety rules

- The runner requires `docker`, Docker Compose, `curl`, `jq`, `tar`, `shasum`,
  and a Go toolchain; it fails before starting any containers when a dependency
  is absent.
- Source and target passwords are test-only process-local values. No external
  storage, user `.env`, provider credential, server address or object-store
  credential is read.
- Shell traps stop local API processes and remove only Docker Compose projects
  named by the runner. Existing host services and volumes are never stopped or
  removed.
- A failed restore never emits successful evidence. Source artifacts and logs
  remain under a local ignored `.tmp/backup-restore-drill` path for diagnosis.
- The runner validates Qdrant snapshot API responses and requires both
  collections before continuing. It does not copy Qdrant storage directories
  or rely on undocumented internals.

## Test and documentation evidence

A contract test requires the Make entry, script, disaster-recovery runbook and
hosted documentation to expose the isolated drill and its non-production
boundary. The integration runner itself is invoked by a dedicated Make target
and is suitable for scheduled/manual evidence collection rather than every
unit-test invocation. `make agent-gate` remains the required repository gate.

## Non-goals

This does not configure scheduled encrypted off-host backups, test multi-node
Qdrant recovery, exercise a real provider, establish the two independent
reference deployments required by the roadmap, or replace the operator's
production change-freeze and approval process.
