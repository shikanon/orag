# Continuous Go fuzzing plan

**Issue:** [#219](https://github.com/shikanon/orag/issues/219)

**Goal:** Continuously explore untrusted parser and expression inputs with reproducible native Go fuzzing while preserving least-privilege CI.

## Delivery

- [x] Add deterministic seeds and a native fuzz target for text, HTML, XML, and Office ZIP parsing.
- [x] Add deterministic seeds and a native fuzz target for optimizer expression lexing, parsing, and evaluation.
- [x] Add a SHA-pinned, read-only workflow with short PR/main runs, longer scheduled runs, and crash artifact retention.
- [x] Document local reproduction, sensitive-input handling, changelog impact, and bilingual Stage 3 progress.
- [x] Pass local repository gates and both 15-second native fuzz runs.
- [ ] Pass protected remote checks.
- [ ] Merge a ready pull request that closes #219 and require both fuzz checks on `main`.
- [ ] Confirm the post-merge Scorecard recognizes fuzzing and closes its Fuzzing alert.

## Verification

- 15-second local run of each fuzz target
- `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/*.yml`
- `make agent-gate`
- Protected remote checks plus the post-merge OpenSSF Scorecard run
