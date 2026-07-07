# Multimodal Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a stable multimodal fixture scenario using the approved remote image, BGM, video, long-video, and docx test URLs.

**Architecture:** The scenario is a Go manifest demo under `examples/scenarios/multimodal-assets`. It validates URL shape and prints metadata without downloading assets, keeping normal demo runs fast and avoiding large-file side effects. Contract tests pin the exact URLs so the fixture set cannot drift silently.

**Tech Stack:** Go standard library, Markdown scenario docs, Go contract tests.

---

### Task 1: Scenario Manifest

**Files:**
- Create: `examples/scenarios/multimodal-assets/main.go`
- Create: `examples/scenarios/multimodal-assets/demo-data.md`
- Create: `examples/scenarios/multimodal-assets/sample-input.md`
- Create: `examples/scenarios/multimodal-assets/expected-output.md`
- Create: `examples/scenarios/multimodal-assets/README.md`

- [x] **Step 1: Add Go manifest**

Store all seven provided HTTPS URLs in `main.go`, classify media kind, print filename and extension, and mark `TestLongVideo.mp4` as `large_file_only=true`.

- [x] **Step 2: Add scenario docs**

Document role, usage dimensions, asset table, expected output, and the rule that long video is upload-only.

### Task 2: Example Index And Contract

**Files:**
- Modify: `examples/README.md`
- Modify: `tests/contract/examples_test.go`

- [x] **Step 1: Link the scenario**

Add `go run ./examples/scenarios/multimodal-assets` to the top-level examples index.

- [x] **Step 2: Protect the fixture URLs**

Extend contract tests to require the scenario files and the exact seven user-provided URLs.

### Task 3: Validation

**Files:**
- Test: `examples/scenarios/multimodal-assets/main.go`
- Test: `tests/contract/examples_test.go`

- [x] **Step 1: Run focused demo**

Run:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/scenarios/multimodal-assets
```

Expected: output includes `scenario=multimodal-assets`, `asset_count=7`, and `large_file_only=true`.

- [x] **Step 2: Run examples and contract tests**

Run:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./examples/... -v
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -v
```

Expected: PASS.
