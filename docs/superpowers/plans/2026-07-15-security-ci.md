# Security CI Implementation Plan

> **Issue:** [#213](https://github.com/shikanon/orag/issues/213)

**Goal:** Make source, dependency, secret, container, and repository supply-chain analysis repeatable, least-privilege security controls while keeping Stage 3 explicitly incomplete.

### Task 1: Add pull-request and default-branch gates

- [x] Add SHA-pinned CodeQL analysis for Go and JavaScript/TypeScript.
- [x] Add Go root/consumer `govulncheck` and production Console npm audit.
- [x] Add reachable-history secret scanning without PR comment permissions.
- [x] Build and scan both API and Console images for fixed HIGH/CRITICAL findings.

### Task 2: Publish supply-chain posture

- [x] Add an isolated, least-privilege OpenSSF Scorecard workflow.
- [x] Publish Scorecard results and upload SARIF using only actions permitted by the Scorecard service.
- [x] Verify native provider scanning and push protection; document that non-provider patterns and validity checks are unavailable to a user-owned public repository and cover reachable history with Gitleaks instead.

### Task 3: Validate and publish

- [x] Validate workflow syntax and local scanners; build and scan both images on a GitHub-hosted runner when local Docker Hub access is unavailable.
- [x] Record the security gates in project documentation, CHANGELOG, and both Roadmaps.
- [x] Push `codex/security-ci`, open a ready PR with `Closes #213`, and pass every remote check.
- [x] Add the successful PR security contexts to protected `main`.
- [x] Squash merge, verify the default-branch Scorecard run, sync `main`, and clean this worktree/branch.
