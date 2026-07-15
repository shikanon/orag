# Supply-chain dependency pinning plan

**Issue:** [#217](https://github.com/shikanon/orag/issues/217)

**Goal:** Make workflow and container inputs immutable and reduce CI token authority without breaking release or documentation delivery.

## Delivery

- [x] Resolve official release commits for every action used by CI, docs, and release workflows.
- [x] Pin actions to full commit SHAs and retain exact release versions as review comments.
- [x] Pin API and Console base images to verified multi-architecture manifest digests while retaining readable tags.
- [x] Give general CI only `contents: read`; retain narrowly scoped writes for Pages, packages, attestations, OIDC, and GitHub Releases.
- [x] Pass local workflow, documentation, Compose, and repository gates.
- [ ] Pass protected remote checks and merge a ready pull request that closes #217.
- [ ] Confirm the default-branch Scorecard rerun no longer reports the addressed pinned-dependency and token-permission findings.

## Verification

- `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/*.yml`
- `./scripts/build-docs-site.sh`
- `docker compose -f deployments/docker-compose.yml --profile demo config`
- `make agent-gate`
- GitHub protected checks plus the post-merge OpenSSF Scorecard run
