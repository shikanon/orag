# RAG Span Timeline Verification Plan

> **Issue:** [#163](https://github.com/shikanon/orag/issues/163)

**Goal:** Close the remaining observability evidence gap by proving graph-emitted spans carry real, ordered execution windows before trace-store fallback normalization.

**Current state:** Commit `6a600b6` already populates `Sequence`, `StartedAt`, and `EndedAt` in `NodeSet.withSpan`. The missing deliverable is a focused regression test and verified compatibility coverage.

### Task 1: Add graph timeline regression coverage

- [x] Add `TestRAGGraphPersistsSpanTimelineMetadata` in `internal/graph/rag_graph_test.go`.
- [x] Assert non-empty multi-node spans, contiguous 1-based sequence, UTC/non-zero timestamps, ordered non-overlapping execution windows, and latency consistent with the measured window.
- [x] Run `go test ./internal/graph -run TestRAGGraphPersistsSpanTimelineMetadata -count=1`.

### Task 2: Preserve trace-store fallback compatibility

- [x] Run graph, PostgreSQL trace, and app memory-trace tests.
- [x] Confirm `normalizedSpanTimes` and `memoryTraceSpanTimes` remain unchanged for legacy/manual zero-valued spans.
- [x] Run the full optimizer-independent repository gate with `make agent-gate`.

### Task 3: Record and publish Roadmap completion

- [x] Update `CHANGELOG.md`, `ROADMAP.md`, and `ROADMAP_EN.md` with #163 and the already-verified #166 diagnostic trace closure.
- [ ] Commit, push `codex/trace-span-timeline`, and open a ready PR containing `Closes #163`.
- [ ] Wait for required checks, squash merge, sync local `main`, and remove only this worktree/branch.
