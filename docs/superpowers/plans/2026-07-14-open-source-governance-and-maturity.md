# Open-Source Governance and Maturity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish ORAG's public community, security, contribution, dependency-update, and capability-maturity baseline.

**Architecture:** Repository policy lives in standard root and `.github` files. Capability maturity is a required field in the manifest and OpenAPI operations, with Go contract tests preventing invalid values and generated-artifact drift. Mutable GitHub repository settings are applied only after the repository files pass CI and are verified by reading them back through the API.

**Tech Stack:** Markdown, GitHub issue forms, GitHub Actions, Dependabot, Go 1.26, kin-openapi, GitHub REST API.

## Global Constraints

- The only maturity values are `experimental`, `beta`, and `stable`.
- No capability may be marked `stable` before `v1.0.0`.
- The roadmap uses date-free phases rather than calendar milestones.
- Branch protection must not require an external approving reviewer while the repository has one maintainer.
- Generated MCP and Skill artifacts remain derived from the capability manifest.
- Do not commit `.superpowers/`, `.env`, credentials, or runtime state.

---

### Task 1: Community and security policy

**Files:**
- Create: `CONTRIBUTING.md`
- Create: `SECURITY.md`
- Create: `CODE_OF_CONDUCT.md`
- Create: `CHANGELOG.md`
- Create: `docs/compatibility.md`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

**Interfaces:**
- Consumes: existing Make targets and repository layout.
- Produces: public contribution, disclosure, compatibility, release-note, and date-free roadmap contracts.

- [ ] **Step 1: Add policy-content checks**

Create `tests/contract/community_files_test.go` with a table that reads each required file and asserts required phrases such as `make test`, `Private Vulnerability Reporting`, `Contributor Covenant`, `experimental`, `beta`, and `stable`.

```go
func TestCommunityFilesContainRequiredPolicy(t *testing.T) {
    checks := map[string][]string{
        "../../CONTRIBUTING.md": {"make test", "Pull Request"},
        "../../SECURITY.md": {"Private Vulnerability Reporting", "Supported Versions"},
        "../../CODE_OF_CONDUCT.md": {"Contributor Covenant"},
        "../../docs/compatibility.md": {"experimental", "beta", "stable"},
    }
    for path, phrases := range checks {
        body, err := os.ReadFile(path)
        if err != nil {
            t.Errorf("read %s: %v", path, err)
            continue
        }
        for _, phrase := range phrases {
            if !bytes.Contains(body, []byte(phrase)) {
                t.Errorf("%s missing %q", path, phrase)
            }
        }
    }
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./tests/contract -run TestCommunityFilesContainRequiredPolicy -v`

Expected: FAIL because the files do not exist.

- [ ] **Step 3: Write the policy files**

Document setup, test tiers, generated artifacts, PR expectations, security reporting, supported Beta versions, Contributor Covenant 2.1 enforcement, Keep a Changelog sections, and the exact maturity/compatibility policy. Apply the already-approved phase-only roadmap wording.

- [ ] **Step 4: Run the focused test and Markdown checks**

Run: `go test ./tests/contract -run TestCommunityFilesContainRequiredPolicy -v && git diff --check`

Expected: PASS and no whitespace errors.

- [ ] **Step 5: Commit**

```bash
git add CONTRIBUTING.md SECURITY.md CODE_OF_CONDUCT.md CHANGELOG.md docs/compatibility.md ROADMAP.md ROADMAP_EN.md tests/contract/community_files_test.go
git commit -m "docs: add community and security policies"
```

### Task 2: Issue forms and pull-request template

**Files:**
- Create: `.github/ISSUE_TEMPLATE/config.yml`
- Create: `.github/ISSUE_TEMPLATE/bug.yml`
- Create: `.github/ISSUE_TEMPLATE/feature.yml`
- Create: `.github/ISSUE_TEMPLATE/documentation.yml`
- Create: `.github/ISSUE_TEMPLATE/rfc.yml`
- Create: `.github/pull_request_template.md`
- Modify: `tests/contract/community_files_test.go`

**Interfaces:**
- Consumes: contribution and security policies from Task 1.
- Produces: structured GitHub intake with secret warnings and acceptance criteria.

- [ ] **Step 1: Extend the contract test**

Assert every issue form has `name`, `description`, `body`, and at least one required checkbox or textarea; assert the PR template contains `Testing`, `Documentation`, `Security`, `Compatibility`, and `Maturity`.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./tests/contract -run TestCommunity -v`

Expected: FAIL because `.github/ISSUE_TEMPLATE` does not exist.

- [ ] **Step 3: Add structured templates**

Use GitHub issue-form YAML. Disable blank issues, link security reports to the repository security advisory page, and give each form a default label. RFCs must collect motivation, proposed contract, alternatives, compatibility, security, observability, migration, and acceptance criteria.

- [ ] **Step 4: Validate templates**

Run: `go test ./tests/contract -run TestCommunity -v && git diff --check`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add .github/ISSUE_TEMPLATE .github/pull_request_template.md tests/contract/community_files_test.go
git commit -m "chore: add issue and pull request templates"
```

### Task 3: Dependabot coverage

**Files:**
- Create: `.github/dependabot.yml`
- Modify: `tests/contract/community_files_test.go`

**Interfaces:**
- Consumes: repository ecosystems at `/`, `/console`, `.github/workflows`, and `deployments`.
- Produces: weekly dependency update coverage for Go, npm, Actions, and Docker.

- [ ] **Step 1: Add a Dependabot contract test**

Read `.github/dependabot.yml` and assert it contains `gomod`, `npm`, `github-actions`, and two `docker` entries for API and Console Dockerfile locations once the Console Dockerfile exists. For this PR, use `/deployments` as the Docker location and leave the second Docker entry for PR 3.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./tests/contract -run TestDependabot -v`

Expected: FAIL because the configuration does not exist.

- [ ] **Step 3: Add `.github/dependabot.yml`**

Configure weekly updates with explicit directories, `open-pull-requests-limit: 5`, grouped non-major development updates, and conventional commit prefixes. Do not group major or security updates.

- [ ] **Step 4: Run the test**

Run: `go test ./tests/contract -run TestDependabot -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add .github/dependabot.yml tests/contract/community_files_test.go
git commit -m "chore: configure dependabot"
```

### Task 4: Manifest maturity enum

**Files:**
- Modify: `internal/capabilities/manifest.go`
- Modify: `internal/capabilities/manifest_test.go`
- Modify: `internal/capabilities/builtin.go`

**Interfaces:**
- Consumes: `capabilities.Capability` and `Validate(Manifest)`.
- Produces: `MaturityExperimental`, `MaturityBeta`, `MaturityStable`, a required `Capability.Maturity string`, and validation errors at `capabilities[i].maturity`.

- [ ] **Step 1: Write failing maturity tests**

```go
func TestValidateRejectsInvalidMaturity(t *testing.T) {
    manifest := cloneManifest(t, BuiltinManifest())
    manifest.Capabilities[0].Maturity = "preview"
    requireValidationError(t, Validate(manifest), "capabilities[0].maturity: must be one of experimental, beta, stable")
}

func TestBuiltinManifestUsesPublishedMaturityValues(t *testing.T) {
    for _, capability := range BuiltinManifest().Capabilities {
        if capability.Maturity != MaturityExperimental {
            t.Fatalf("%s maturity = %q", capability.ID, capability.Maturity)
        }
    }
}
```

- [ ] **Step 2: Run the tests and confirm compilation fails**

Run: `go test ./internal/capabilities -run Maturity -v`

Expected: FAIL because `Maturity` and constants do not exist.

- [ ] **Step 3: Implement the enum and validation**

Add constants, an allowed-value map, `Maturity string \`json:"maturity"\`` to `Capability`, require it in `validateCapability`, and set all current builtins to `experimental`. Keep existing `Status` because it expresses runtime availability (`planned` versus implemented), not contract stability.

- [ ] **Step 4: Run capability tests**

Run: `go test ./internal/capabilities -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/capabilities/manifest.go internal/capabilities/manifest_test.go internal/capabilities/builtin.go
git commit -m "feat: add capability maturity contract"
```

### Task 5: Propagate maturity to generated artifacts

**Files:**
- Modify: `internal/agentsync/generator.go`
- Modify: `internal/agentsync/generator_test.go`
- Modify: `internal/agentskills/generator.go`
- Modify: `internal/agentskills/generator_test.go`
- Regenerate: `agent/mcp/**`
- Regenerate: `agent/skills/**`

**Interfaces:**
- Consumes: `Capability.Maturity` from Task 4.
- Produces: generated MCP JSON field `maturity` and generated Skill front matter/body maturity notice.

- [ ] **Step 1: Add failing generator assertions**

Assert rendered MCP tool documents include `"maturity":"experimental"` per tool and generated Skill content includes `Maturity: experimental`.

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `go test ./internal/agentsync ./internal/agentskills -run Maturity -v`

Expected: FAIL because generators omit maturity.

- [ ] **Step 3: Add maturity to generated representations**

Extend only generator-owned DTOs and templates; do not alter MCP protocol annotations with an unknown standard field. Put maturity in ORAG-owned metadata and generated documentation.

- [ ] **Step 4: Regenerate and verify drift**

Run: `make agent-sync && make agent-sync-check && go test ./internal/agentsync ./internal/agentskills -v`

Expected: PASS with deterministic generated files.

- [ ] **Step 5: Commit**

```bash
git add internal/agentsync internal/agentskills agent/mcp agent/skills
git commit -m "feat: publish maturity in agent artifacts"
```

### Task 6: OpenAPI maturity contract

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `tests/contract/openapi_test.go`

**Interfaces:**
- Consumes: the maturity enum from Task 4 and existing OpenAPI operations.
- Produces: required `x-orag-maturity` on every operation and matching maturity on `x-orag-agent-capabilities` entries.

- [ ] **Step 1: Write the failing OpenAPI test**

Iterate every operation and require an extension value in `{experimental,beta,stable}`. Compare agent capability entries by operation ID to the builtin manifest when the operation exists.

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./tests/contract -run TestOpenAPIMaturity -v`

Expected: FAIL on the first operation missing `x-orag-maturity`.

- [ ] **Step 3: Annotate OpenAPI**

Set current system endpoints and all `/v1` operations to `experimental`; add `maturity: experimental` to agent capability extension records. Preserve `status: planned` separately.

- [ ] **Step 4: Run contract and full agent gates**

Run: `make openapi-validate agent-gate`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/openapi.yaml tests/contract/openapi_test.go
git commit -m "docs: annotate openapi capability maturity"
```

### Task 7: Public maturity documentation and full validation

**Files:**
- Modify: `README.md`
- Modify: `README_EN.md`
- Modify: `docs/README.md`
- Modify: `docs/compatibility.md`

**Interfaces:**
- Consumes: manifest and OpenAPI maturity contracts.
- Produces: bilingual discoverable maturity guidance without claiming stable APIs.

- [ ] **Step 1: Add documentation assertions**

Extend `community_files_test.go` to require both READMEs to link `docs/compatibility.md` and describe the three maturity values.

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./tests/contract -run TestCommunity -v`

Expected: FAIL because README links are absent.

- [ ] **Step 3: Update documentation**

Add a compact maturity section and link it from the docs index. State that all current capabilities are experimental and `v0.1.0-beta.1` is a Beta distribution, not a claim that every feature is Beta.

- [ ] **Step 4: Run full repository validation**

Run: `make test vet openapi-validate agent-gate && npm --prefix console test -- --run && npm --prefix console run build && git diff --check`

Expected: all commands exit 0.

- [ ] **Step 5: Commit**

```bash
git add README.md README_EN.md docs/README.md docs/compatibility.md tests/contract/community_files_test.go
git commit -m "docs: publish capability maturity policy"
```

### Task 8: Publish PR and apply repository settings

**Files:**
- No source file changes after validation unless CI reports an actionable defect.

**Interfaces:**
- Consumes: green PR checks and exact required check names.
- Produces: merged governance baseline plus verified GitHub Discussions, Topics, Private Vulnerability Reporting, delete-branch-on-merge, and `main` protection.

- [ ] **Step 1: Rebase and push the feature branch**

Run: `git fetch origin && git rebase origin/main && git push -u origin codex/open-source-governance`

Expected: push succeeds and the branch contains only governance/maturity changes.

- [ ] **Step 2: Create a ready-for-review PR**

Use `gh pr create` with the validation evidence, maturity decision, repository-setting follow-up, and no auto-generated secrets.

- [ ] **Step 3: Wait for required checks and merge**

Run: `gh pr checks --watch <PR>` followed by `gh pr merge <PR> --squash --delete-branch` only when every required check passes.

- [ ] **Step 4: Apply mutable repository settings**

Use `gh repo edit` for Discussions, Topics, and delete-branch-on-merge; enable private vulnerability reporting through `PUT /repos/shikanon/orag/private-vulnerability-reporting`; apply `main` protection with required status checks, strict mode, conversation resolution, force-push disabled, deletion disabled, and no required approving review count.

- [ ] **Step 5: Verify settings**

Read back repository metadata, vulnerability reporting, and branch protection through `gh api`. Confirm `main...origin/main` is `0 0` in a clean checkout before starting PR 2.
