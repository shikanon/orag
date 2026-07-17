# Isolated Backup and Restore Drill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove that an ORAG PostgreSQL dump and Qdrant snapshots restore together into an isolated stack that can serve a cited query and trace.

**Architecture:** A dedicated two-service Compose definition can be started twice under isolated project names. A shell runner owns source seeding, backup, manifest verification, target restore and API verification; it writes only local ignored diagnostics/evidence.

**Tech Stack:** Docker Compose, PostgreSQL 16, Qdrant 1.11.5 REST snapshots, existing `orag-api`, `orag-demo`, `oragctl backup-verify`, Bash, jq.

## Global Constraints

- Use explicit deterministic mock providers; never read user `.env` or real provider credentials.
- Restore through Qdrant's supported snapshot-upload API, never by copying Qdrant storage internals.
- Remove only runner-named Compose projects and volumes in cleanup.
- Do not claim the runner proves production RPO/RTO or independent deployment evidence.

---

### Task 1: Add isolated dependency topology and drill runner

**Files:**
- Create: `deployments/docker-compose.restore-drill.yml`
- Create: `scripts/backup-restore-drill.sh`
- Modify: `Makefile`

**Interfaces:**
- Produces: `make backup-restore-drill`.
- Consumes: `docker compose`, `cmd/orag-api`, `cmd/orag-demo`, `cmd/oragctl backup-verify`, Qdrant collection snapshot endpoints.

- [ ] **Step 1: Add the Compose dependency definition**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment: { POSTGRES_USER: orag, POSTGRES_PASSWORD: orag, POSTGRES_DB: orag }
  qdrant:
    image: qdrant/qdrant:v1.11.5
```

- [ ] **Step 2: Implement source backup and manifest verification**

```bash
source_compose=(docker compose -p orag-backup-drill-source -f deployments/docker-compose.restore-drill.yml)
"${source_compose[@]}" exec -T postgres pg_dump -U orag -d orag --format=custom --no-owner --no-acl > "$backup/postgres.dump"
curl -fsS -X POST "$source_qdrant/collections/$collection/snapshots"
go run ./cmd/oragctl backup-verify --dir "$backup"
```

- [ ] **Step 3: Implement target restore and HTTP evidence checks**

```bash
"${target_compose[@]}" exec -T postgres pg_restore -U orag -d orag --no-owner --no-acl --exit-on-error < "$backup/postgres.dump"
curl -fsS -X POST "$target_qdrant/collections/$collection/snapshots/upload?priority=snapshot" -F "snapshot=@$snapshot"
curl -fsS -X POST "$target_api/v1/query" -H "Authorization: Bearer $token" -d "$query"
```

- [ ] **Step 4: Add the guarded Make target**

```make
backup-restore-drill:
	./scripts/backup-restore-drill.sh
```

### Task 2: Document and protect the drill contract

**Files:**
- Modify: `docs/operations/disaster-recovery.md`
- Modify: `docs-site/performance-baseline.html` or create `docs-site/disaster-recovery.html`
- Modify: `docs-site/index.html`
- Create: `tests/contract/backup_restore_drill_test.go`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

**Interfaces:**
- Consumes: the Make target and runner path.
- Produces: a documented repeatable drill with truthful scope language.

- [ ] **Step 1: Add a failing contract test**

```go
func TestBackupRestoreDrillIsDocumented(t *testing.T) {
    // Assert Makefile, runbook, hosted page and Roadmaps mention backup-restore-drill and isolated restore.
}
```

- [ ] **Step 2: Update operator and hosted documentation**

State exact prerequisites, the command, artifact/evidence location, cleanup behavior, and that the result does not prove a production deployment.

- [ ] **Step 3: Run focused contracts and build the docs site**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestBackupRestoreDrillIsDocumented -v && ./scripts/build-docs-site.sh`

Expected: PASS and the hosted recovery page is generated.

### Task 3: Execute the real drill and complete repository validation

**Files:**
- Test: `scripts/backup-restore-drill.sh`

- [ ] **Step 1: Execute the integration drill**

Run: `make backup-restore-drill`

Expected: backup manifest verifies, the target serves a citation and trace, and `.tmp/backup-restore-drill/drill-evidence.json` exists.

- [ ] **Step 2: Run complete quality gate**

Run: `make agent-gate`

Expected: PASS.

- [ ] **Step 3: Commit, push, open PR, wait for required checks, squash merge, synchronize `main`, deploy docs, and verify the hosted recovery page returns HTTP 200**

Run: repository protected-main workflow and the configured documentation server key.
