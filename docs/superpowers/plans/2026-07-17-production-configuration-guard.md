# Production Configuration Guard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent an explicitly production-configured ORAG process from starting with local demo credentials, mock providers, debug mode, mock object storage, or an unsafe public URL.

**Architecture:** `ORAG_ENV` becomes a validated server field. A focused pure validation helper runs only for production intent and checks a fixed allowlist of deployment safety invariants without reading files, DNS, or external services.

**Tech Stack:** Go configuration loader and tests, Compose/YAML documentation, Go contract tests.

## Global Constraints

- `development` remains the default environment.
- Only `development` and `production` are accepted values for `ORAG_ENV`.
- Production errors name settings but never disclose secret values.
- Production rejects debug/mock execution and weak/default bootstrap credentials.
- Production URL validation requires absolute HTTPS and rejects localhost, IP literals and `.local` hosts.

---

### Task 1: Add failing production configuration tests

**Files:**
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Produces expected `Load()` failures for unsafe production settings and a
  complete accepted production configuration.

- [ ] Add a helper that sets the four real providers, safe secrets, strong
  bootstrap password, HTTPS public URL, and `ORAG_ENV=production`.
- [ ] Add table-driven mutations for short/default/equal auth values, debug,
  mock provider, mock storage, HTTP/localhost/IP/local URL and invalid
  environment name.
- [ ] Run `go test ./internal/config -run 'TestProduction|TestEnvironment' -count=1`; expected failure before implementation.

### Task 2: Implement explicit production intent and guard

**Files:**
- Modify: `internal/config/config.go`

**Interfaces:**
- Produces `ServerConfig.Environment string`, `ServerConfig.IsProduction() bool`,
  and `Config.validateProductionConfiguration() error`.

- [ ] Load `ORAG_ENV` as lowercase trimmed `development` by default.
- [ ] Validate the two allowed environment values before dependent production
  checks.
- [ ] Implement production validation using `net/url` and `net/netip` for URL
  shape/host restrictions, fixed demo-secret comparisons, and the explicit
  model/object-storage/debug checks.
- [ ] Run the focused config suite; expected PASS.

### Task 3: Document and lock deployment intent

**Files:**
- Modify: `.env.example`
- Modify: `docs/operations/server-deployment.md`
- Modify: `docs/operations/README.md`
- Create: `tests/contract/production_configuration_test.go`

**Interfaces:**
- Produces a documented `ORAG_ENV=production` requirement for server
  deployment and a drift test for the template and runbook.

- [ ] Add `ORAG_ENV=development` to the local template with an explicit
  warning that production must set `production`.
- [ ] Add `ORAG_ENV=production` to the server-only environment example and
  document its exact rejected categories.
- [ ] Add a contract test requiring the environment variable, production
  guard and non-disclosure wording.
- [ ] Run `go test ./tests/contract -run TestProductionConfigurationDocumentation -v`; expected PASS.

### Task 4: Verify, publish, and deploy

**Files:**
- Modify: `ROADMAP.md`, `ROADMAP_EN.md`

**Interfaces:**
- Produces an accurate roadmap note that a startup production guard is
  available without claiming a completed production pilot or secret rotation.

- [ ] Run focused config and contract tests, `git diff --check`, and `make agent-gate`.
- [ ] Commit, push, create and merge a PR after all required checks pass.
- [ ] Sync `main`, build/deploy documentation, and verify the hosted guide is
  reachable over HTTPS.
