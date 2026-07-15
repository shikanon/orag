# Eino Jinja filesystem-boundary upgrade plan

**Issue:** [#221](https://github.com/shikanon/orag/issues/221)

**Goal:** Land Eino's Jinja filesystem-access fix independently from unrelated dependency upgrades and preserve verifiable vulnerability evidence.

## Delivery

- [x] Upgrade only Eino to 0.9.12 and update the root plus standalone SDK consumer module graphs.
- [x] Add a canary-file contract test proving Jinja `file` and `fileset` filters fail closed.
- [x] Verify `golang.org/x/crypto/openpgp` is not imported and document GO-2026-5932 as an unreachable module-level advisory with no fixed release.
- [x] Pass full unit/vet, race, public SDK, agent, OpenAPI, and real PostgreSQL + Qdrant integration gates.
- [ ] Pass protected remote checks and merge a ready pull request that closes #221.
- [ ] Re-run Scorecard, record the remaining module-level advisory behavior, and close or supersede the Eino portion of Dependabot PR #190.

## Verification

- `go test ./tests/contract -run TestEinoJinjaDisablesFilesystemFilters -v`
- `GOTOOLCHAIN=go1.26.5 govulncheck -show verbose ./...`
- `CGO_ENABLED=1 GOFLAGS=-tags=stdjson,gjson go test -race ./...`
- `make agent-gate`
- `make test-integration`
- Protected remote checks and post-merge Scorecard
