---
name: orag-self-ops
description: "Generate a dry-run plan before any operational write."
---

# ORAG Self Ops Trae Skill

Generated from `orag.capabilities.v1` version `2026-07-05` with generator `manifest-first.v1` for Trae.

## Purpose
Generate a dry-run plan before any operational write.

## Trigger Conditions
- User asks for a dry-run maintenance plan.
- User explicitly authorizes a low-risk operational action.

## Anti-Triggers
- Do not use for read-only health checks.
- Do not apply actions without explicit approval and fresh precondition checks.

## Mutual Exclusion
- Key: `self-ops`
- Self-ops is the only Skill allowed to enter authorized write workflows.

## Capabilities
- `orag_apply_low_risk_action`: `apply-low-risk-action` via `POST /v1/self-ops/apply-low-risk-action`, input `#/components/schemas/ApplyLowRiskActionRequest`, output `#/components/schemas/ApplyLowRiskActionResult`, risk `high`, side effect `write`
- `orag_create_remediation_issue`: `create-remediation-issue` via `POST /v1/self-ops/remediation-issues`, input `#/components/schemas/CreateRemediationIssueRequest`, output `#/components/schemas/CreateRemediationIssueResult`, risk `medium`, side effect `write`
- `orag_maintenance_plan`: `maintenance-plan` via `POST /v1/self-ops/maintenance-plan`, input `#/components/schemas/MaintenancePlanRequest`, output `#/components/schemas/MaintenancePlan`, risk `medium`, side effect `dry_run`

## Environment
- `ORAG_API_BASE_URL`
- `ORAG_API_TOKEN`
- `ORAG_TENANT_ID`

## Call Steps
1. Generate or read a dry-run plan.
2. Verify snapshot, preconditions, idempotency key, and lock key.
3. Apply only if the capability and user authorization permit writes.

## Example Prompts
- Create a dry-run plan to regenerate stale agent artifacts.

## Example Request: `orag_apply_low_risk_action`
Apply the approved low-risk action from plan plan_20260705_001.

```json
{
  "approved": true,
  "plan_id": "plan_20260705_001"
}
```

## Expected Output Shape: `orag_apply_low_risk_action`
```json
{
  "status": "completed",
  "verdict": "pass"
}
```


## Example Request: `orag_create_remediation_issue`
Create a remediation issue for the failed release readiness check.

```json
{
  "approved": true,
  "finding_id": "release.contract.openapi_failed"
}
```

## Expected Output Shape: `orag_create_remediation_issue`
```json
{
  "issue_id": "orag-remediation-001",
  "verdict": "pass"
}
```


## Example Request: `orag_maintenance_plan`
Create a dry-run maintenance plan to regenerate stale agent artifacts.

```json
{
  "dry_run": true,
  "scope": "agent_artifacts"
}
```

## Expected Output Shape: `orag_maintenance_plan`
```json
{
  "lock_key": "selfops:agent-artifacts",
  "verdict": "pass"
}
```

## Safety Boundaries
- Default to dry-run.
- Block on precondition drift.
- Use single-flight locking and idempotency keys for writes.

## Failure Handling
- Return blocked when authorization is absent.
- Return blocked when snapshot or preconditions drift.

## Trae Usage
- Invoke this Skill only when the user request matches the trigger conditions.
- Keep actions inside the listed safety boundaries and ask before expanding scope.
