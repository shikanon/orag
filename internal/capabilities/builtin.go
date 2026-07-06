package capabilities

import "net/http"

// BuiltinManifest returns the first manifest-first capability catalog.
func BuiltinManifest() Manifest {
	return Manifest{
		SchemaVersion:     SchemaVersion,
		CapabilityVersion: "2026-07-05",
		GeneratorVersion:  "manifest-first.v1",
		Generation: GenerationMetadata{
			OpenAPIFacetPath: "api/openapi.yaml",
			MCPToolsPath:     ".mcp/tools",
			SkillTargets:     []string{"codex", "claude-code", "trae"},
			ArtifactPaths: []string{
				".mcp/tools",
				".codex/skills",
				".claude/skills",
				".trae/skills",
			},
		},
		DriftChecks: []DriftCheckMetadata{
			{
				ID:          "agent_sync.generated_artifacts_match",
				Description: "Generated MCP, Skill, and OpenAPI facet artifacts match the capability manifest.",
				Commands:    []string{"make agent-sync-check"},
			},
		},
		Capabilities: []Capability{
			ralphLoopCapability(),
			selfCheckCapability(),
			traceLookupCapability(),
			diagnoseCapability(),
			runbookSuggestCapability(),
			maintenancePlanCapability(),
			applyLowRiskActionCapability(),
			createRemediationIssueCapability(),
		},
	}
}

func MustBuiltinManifest() Manifest {
	manifest := BuiltinManifest()
	if err := Validate(manifest); err != nil {
		panic(err)
	}
	return manifest
}

func ralphLoopCapability() Capability {
	return Capability{
		ID:          "ralph-loop",
		DisplayName: "Ralph Loop",
		Description: "Run or resume a bounded Ralph Loop verification workflow for an ORAG task/spec and return the verdict, artifacts, and trace identifier.",
		Status:      "planned",
		RiskLevel:   RiskLow,
		HTTP: HTTPFacet{
			Kind:            "planned_http",
			Method:          http.MethodPost,
			Path:            "/v1/ralph-loop",
			OperationID:     "runRalphLoop",
			AuthScheme:      "bearerAuth",
			RequestSchema:   "#/components/schemas/RalphLoopRequest",
			ResponseSchema:  "#/components/schemas/RalphLoopResponse",
			ErrorSchema:     "#/components/schemas/ErrorResponse",
			BackingServices: []string{"optimization service", "evaluation service", "trace service"},
		},
		MCP: MCPFacet{
			ToolName:     "ralph_loop_run",
			Description:  "Execute a Ralph Loop task verification workflow through the ORAG API contract.",
			InputSchema:  "#/components/schemas/RalphLoopRequest",
			OutputSchema: "#/components/schemas/RalphLoopResponse",
			Annotations:  traceAnnotations("ralph-loop", "Ralph Loop"),
		},
		Skill: ralphLoopSkill(),
		Operations: OperationsSemantic{
			SideEffect: EffectReadOnly,
			ReadOnly:   true,
			Rollback:   []string{"No rollback is required because the capability is planned-only and does not perform writes."},
		},
		Generation: generatedEverywhere("ralph-loop"),
		Examples: []Example{{
			Name:   "verify-task1",
			Prompt: "Run Ralph Loop for Task 1 in focused mode with at most one round, then report the verdict and trace ID.",
			Input: map[string]any{
				"task_spec_path": ".trae/specs/add-ralph-loop-mcp-skills/tasks.md",
				"task_id":        "Task 1",
				"mode":           "focused",
				"max_rounds":     1,
			},
			ExpectedOutput: map[string]any{
				"status":   "completed",
				"verdict":  "pass",
				"trace_id": "trace_ralph_loop_example",
			},
		}},
	}
}

func selfCheckCapability() Capability {
	return Capability{
		ID:          "self-check",
		DisplayName: "ORAG Self Check",
		Description: "Run focused or broad read-only checks for health, contract, agent sync, smoke, storage, config, release, or all scopes.",
		Status:      "planned",
		RiskLevel:   RiskLow,
		HTTP:        httpFacet("planned_http", "/v1/self-check", "runSelfCheck", "#/components/schemas/SelfCheckRequest", "#/components/schemas/SelfCheckResult"),
		MCP:         mcpFacet("self-check", "ORAG Self Check", "orag_check", "Run a read-only ORAG self-check and return stable check IDs, evidence, verdict, and trace metadata.", "#/components/schemas/SelfCheckRequest", "#/components/schemas/SelfCheckResult"),
		Skill: SkillBehavior{
			ManifestName:        "orag-self-check",
			Description:         "Use for read-only ORAG health, contract, agent-sync, smoke, storage, config, and release checks.",
			TriggerConditions:   []string{"User asks to check ORAG health.", "User asks whether MCP or Skills are synchronized.", "User asks for release readiness or CI preflight checks."},
			AntiTriggers:        []string{"Do not perform root-cause diagnosis beyond reporting failed checks.", "Do not propose or apply writes; hand off to orag-self-diagnose or orag-self-ops when needed."},
			CallOrder:           []string{"Discover MCP tools.", "Call orag_check with the requested scope and mode.", "Report PASS, FAIL, or BLOCKED with stable check IDs and evidence."},
			SafetyBoundaries:    []string{"Read-only only.", "For agent_sync, state that make agent-sync-check remains the authoritative release gate."},
			FailureHandling:     []string{"Return blocked when a check cannot complete.", "Preserve partial results and surface the trace ID."},
			ExamplePrompts:      []string{"Check ORAG agent_sync in focused mode and report stale generated artifacts."},
			MutualExclusionKey:  "self-check",
			MutualExclusionNote: "Only for checking state; diagnosis and writes belong to separate Skills.",
		},
		Operations: OperationsSemantic{
			SideEffect: EffectReadOnly,
			ReadOnly:   true,
			Rollback:   []string{"No rollback is required for read-only checks."},
		},
		Generation: generatedEverywhere("orag-self-check"),
		Examples: []Example{{
			Name:   "agent-sync-focused",
			Prompt: "Check whether ORAG MCP and Skill artifacts are synchronized.",
			Input:  map[string]any{"scope": "agent_sync", "mode": "focused"},
			ExpectedOutput: map[string]any{
				"schema_version": "orag.selfops.result.v1",
				"verdict":        "pass",
			},
		}},
	}
}

func traceLookupCapability() Capability {
	return Capability{
		ID:          "trace-lookup",
		DisplayName: "ORAG Trace Lookup",
		Description: "Look up trace evidence for diagnosis without performing writes.",
		Status:      "planned",
		RiskLevel:   RiskLow,
		HTTP:        httpFacet("planned_http", "/v1/diagnostics/traces/{trace_id}", "lookupTrace", "#/components/schemas/TraceLookupRequest", "#/components/schemas/TraceLookupResponse"),
		MCP:         mcpFacet("trace-lookup", "ORAG Trace Lookup", "orag_trace_lookup", "Look up ORAG trace details and return evidence for diagnosis.", "#/components/schemas/TraceLookupRequest", "#/components/schemas/TraceLookupResponse"),
		Skill:       diagnoseSkill("Use trace evidence as read-only input for diagnosis.", "Look up trace trace_req and summarize failing stages."),
		Operations: OperationsSemantic{
			SideEffect: EffectReadOnly,
			ReadOnly:   true,
			Rollback:   []string{"No rollback is required for trace reads."},
		},
		Generation: generatedEverywhere("orag-self-diagnose"),
		Examples: []Example{{
			Name:           "trace-lookup",
			Prompt:         "Look up trace trace_req and summarize the failed stage.",
			Input:          map[string]any{"trace_id": "trace_req"},
			ExpectedOutput: map[string]any{"verdict": "pass", "trace_id": "trace_req"},
		}},
	}
}

func diagnoseCapability() Capability {
	return Capability{
		ID:          "diagnose",
		DisplayName: "ORAG Diagnose",
		Description: "Diagnose symptoms, failed commands, or trace evidence and recommend verification actions without writes.",
		Status:      "planned",
		RiskLevel:   RiskLow,
		HTTP:        httpFacet("planned_http", "/v1/diagnostics/diagnose", "diagnoseSymptom", "#/components/schemas/DiagnoseRequest", "#/components/schemas/DiagnoseResult"),
		MCP:         mcpFacet("diagnose", "ORAG Diagnose", "orag_diagnose", "Diagnose ORAG symptoms from trace IDs, command evidence, and user-provided symptoms without performing writes.", "#/components/schemas/DiagnoseRequest", "#/components/schemas/DiagnoseResult"),
		Skill:       diagnoseSkill("Use symptoms, failed commands, and trace evidence to produce findings and recommended actions.", "Diagnose this MCP agent_sync failure and suggest verification commands."),
		Operations: OperationsSemantic{
			SideEffect: EffectReadOnly,
			ReadOnly:   true,
			Rollback:   []string{"No rollback is required because diagnosis is read-only."},
		},
		Generation: generatedEverywhere("orag-self-diagnose"),
		Examples: []Example{{
			Name:           "diagnose-failed-command",
			Prompt:         "Diagnose why make agent-sync-check failed and recommend the next verification command.",
			Input:          map[string]any{"scope": "mcp", "symptom": "make agent-sync-check failed", "allow_commands": false},
			ExpectedOutput: map[string]any{"verdict": "pass", "severity": "warning"},
		}},
	}
}

func runbookSuggestCapability() Capability {
	return Capability{
		ID:          "runbook-suggest",
		DisplayName: "ORAG Runbook Suggest",
		Description: "Suggest a read-only runbook and verification commands for a diagnostic scope.",
		Status:      "planned",
		RiskLevel:   RiskLow,
		HTTP:        httpFacet("planned_http", "/v1/diagnostics/runbooks/suggest", "suggestRunbook", "#/components/schemas/RunbookSuggestRequest", "#/components/schemas/RunbookSuggestResponse"),
		MCP:         mcpFacet("runbook-suggest", "ORAG Runbook Suggest", "orag_runbook_suggest", "Suggest ORAG runbooks and verification commands without executing writes.", "#/components/schemas/RunbookSuggestRequest", "#/components/schemas/RunbookSuggestResponse"),
		Skill:       diagnoseSkill("Use diagnostic findings to choose a runbook and verification path.", "Suggest a runbook for storage readiness failures."),
		Operations: OperationsSemantic{
			SideEffect: EffectReadOnly,
			ReadOnly:   true,
			Rollback:   []string{"No rollback is required for runbook suggestions."},
		},
		Generation: generatedEverywhere("orag-self-diagnose"),
		Examples: []Example{{
			Name:           "storage-runbook",
			Prompt:         "Suggest a runbook for storage readiness failures.",
			Input:          map[string]any{"scope": "storage", "verdict": "blocked"},
			ExpectedOutput: map[string]any{"verdict": "pass", "runbook": "docs/operations/troubleshooting.md"},
		}},
	}
}

func maintenancePlanCapability() Capability {
	return Capability{
		ID:          "maintenance-plan",
		DisplayName: "ORAG Maintenance Plan",
		Description: "Create a dry-run maintenance plan with snapshot, preconditions, idempotency key, lock key, rollback, and verification commands.",
		Status:      "planned",
		RiskLevel:   RiskMedium,
		HTTP:        httpFacet("planned_http", "/v1/self-ops/maintenance-plan", "createMaintenancePlan", "#/components/schemas/MaintenancePlanRequest", "#/components/schemas/MaintenancePlan"),
		MCP:         mcpFacet("maintenance-plan", "ORAG Maintenance Plan", "orag_maintenance_plan", "Create a dry-run ORAG maintenance plan without applying writes.", "#/components/schemas/MaintenancePlanRequest", "#/components/schemas/MaintenancePlan"),
		Skill:       opsSkill("Generate a dry-run plan before any operational write.", "Create a dry-run plan to regenerate stale agent artifacts."),
		Operations: OperationsSemantic{
			SideEffect:      EffectDryRun,
			ReadOnly:        true,
			DryRunSupported: true,
			Rollback:        []string{"Discard the dry-run plan if preconditions drift or authorization is not granted."},
		},
		Generation: generatedEverywhere("orag-self-ops"),
		Examples: []Example{{
			Name:           "dry-run-agent-artifacts",
			Prompt:         "Create a dry-run maintenance plan to regenerate stale agent artifacts.",
			Input:          map[string]any{"scope": "agent_artifacts", "dry_run": true},
			ExpectedOutput: map[string]any{"verdict": "pass", "lock_key": "selfops:agent-artifacts"},
		}},
	}
}

func applyLowRiskActionCapability() Capability {
	return Capability{
		ID:          "apply-low-risk-action",
		DisplayName: "ORAG Apply Low Risk Action",
		Description: "Apply an explicitly authorized low-risk action after revalidating snapshot preconditions and single-flight lock boundaries.",
		Status:      "planned",
		RiskLevel:   RiskHigh,
		HTTP:        httpFacet("planned_http", "/v1/self-ops/apply-low-risk-action", "applyLowRiskAction", "#/components/schemas/ApplyLowRiskActionRequest", "#/components/schemas/ApplyLowRiskActionResult"),
		MCP:         mcpFacet("apply-low-risk-action", "ORAG Apply Low Risk Action", "orag_apply_low_risk_action", "Apply an authorized low-risk ORAG maintenance action after TOCTOU precondition checks.", "#/components/schemas/ApplyLowRiskActionRequest", "#/components/schemas/ApplyLowRiskActionResult"),
		Skill:       opsSkill("Apply only explicitly authorized low-risk actions after dry-run planning.", "Apply the approved low-risk action from plan plan_20260705_001."),
		Operations: OperationsSemantic{
			SideEffect:       EffectWrite,
			ReadOnly:         false,
			DryRunSupported:  false,
			RequiresApproval: true,
			IdempotencyKey:   "selfops:{action}:{snapshot_hash}",
			LockKey:          "selfops:{scope}",
			Rollback:         []string{"Use the plan rollback commands and report blocked if preconditions drift."},
		},
		Generation: generatedEverywhere("orag-self-ops"),
		Examples: []Example{{
			Name:           "apply-approved-action",
			Prompt:         "Apply the approved low-risk action from plan plan_20260705_001.",
			Input:          map[string]any{"plan_id": "plan_20260705_001", "approved": true},
			ExpectedOutput: map[string]any{"verdict": "pass", "status": "completed"},
		}},
	}
}

func createRemediationIssueCapability() Capability {
	return Capability{
		ID:          "create-remediation-issue",
		DisplayName: "ORAG Create Remediation Issue",
		Description: "Create an explicitly authorized remediation issue from self-check or diagnosis findings.",
		Status:      "planned",
		RiskLevel:   RiskMedium,
		HTTP:        httpFacet("planned_http", "/v1/self-ops/remediation-issues", "createRemediationIssue", "#/components/schemas/CreateRemediationIssueRequest", "#/components/schemas/CreateRemediationIssueResult"),
		MCP:         mcpFacet("create-remediation-issue", "ORAG Create Remediation Issue", "orag_create_remediation_issue", "Create an authorized remediation issue from ORAG findings.", "#/components/schemas/CreateRemediationIssueRequest", "#/components/schemas/CreateRemediationIssueResult"),
		Skill:       opsSkill("Create a remediation issue only after user approval.", "Create a remediation issue for the failed release readiness check."),
		Operations: OperationsSemantic{
			SideEffect:       EffectWrite,
			ReadOnly:         false,
			DryRunSupported:  true,
			RequiresApproval: true,
			IdempotencyKey:   "selfops:remediation-issue:{finding_hash}",
			LockKey:          "selfops:remediation-issue",
			Rollback:         []string{"Close the generated issue if it was created from stale findings."},
		},
		Generation: generatedEverywhere("orag-self-ops"),
		Examples: []Example{{
			Name:           "create-remediation-issue",
			Prompt:         "Create a remediation issue for the failed release readiness check.",
			Input:          map[string]any{"finding_id": "release.contract.openapi_failed", "approved": true},
			ExpectedOutput: map[string]any{"verdict": "pass", "issue_id": "orag-remediation-001"},
		}},
	}
}

func httpFacet(kind, path, operationID, requestSchema, responseSchema string) HTTPFacet {
	return HTTPFacet{
		Kind:            kind,
		Method:          http.MethodPost,
		Path:            path,
		OperationID:     operationID,
		AuthScheme:      "bearerAuth",
		RequestSchema:   requestSchema,
		ResponseSchema:  responseSchema,
		ErrorSchema:     "#/components/schemas/ErrorResponse",
		BackingServices: []string{"trace service", "agent artifact service"},
	}
}

func mcpFacet(id, displayName, toolName, description, inputSchema, outputSchema string) MCPFacet {
	return MCPFacet{
		ToolName:     toolName,
		Description:  description,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Annotations:  traceAnnotations(id, displayName),
	}
}

func traceAnnotations(id, displayName string) map[string]any {
	return map[string]any{
		"orag_capability_id": id,
		"orag_display_name":  displayName,
		"trace": map[string]any{
			"request_header":  "X-Trace-ID",
			"response_header": "X-Trace-ID",
			"response_field":  "trace_id",
		},
	}
}

func generatedEverywhere(skillName string) CapabilityArtifact {
	return CapabilityArtifact{
		OpenAPIFacet: "api/openapi.yaml",
		MCPArtifact:  ".mcp/tools/" + skillName + ".json",
		SkillArtifacts: []string{
			".codex/skills/" + skillName + "/SKILL.md",
			".claude/skills/" + skillName + "/SKILL.md",
			".trae/skills/" + skillName + "/SKILL.md",
		},
	}
}

func ralphLoopSkill() SkillBehavior {
	return SkillBehavior{
		ManifestName:        "ralph-loop",
		Description:         "Use when an agent needs to run bounded ORAG Ralph Loop verification from a spec/task path and report a PASS/FAIL verdict with trace evidence.",
		TriggerConditions:   []string{"User asks to run Ralph Loop verification for an ORAG task or spec.", "Expected answer must include PASS/FAIL verdict, artifacts, and trace evidence."},
		AntiTriggers:        []string{"Do not use for general RAG queries.", "Do not use for unbounded autonomous implementation work."},
		CallOrder:           []string{"Read the task or spec path.", "Call ralph_loop_run with a bounded max_rounds value.", "Report verdict, summary, artifacts, and trace ID."},
		SafetyBoundaries:    []string{"Planned-only runtime boundary.", "Never print bearer tokens or tenant secrets."},
		FailureHandling:     []string{"Surface API or MCP errors without retrying unboundedly.", "Return blocked when task scope is ambiguous."},
		ExamplePrompts:      []string{"Run Ralph Loop for Task 1 in focused mode with at most one round."},
		MutualExclusionKey:  "ralph-loop",
		MutualExclusionNote: "Ralph Loop verification is separate from self-check, diagnosis, and self-ops Skills.",
	}
}

func diagnoseSkill(description, prompt string) SkillBehavior {
	return SkillBehavior{
		ManifestName:        "orag-self-diagnose",
		Description:         description,
		TriggerConditions:   []string{"User provides symptoms, trace IDs, logs, or failed command evidence.", "User asks for root-cause analysis or recommended verification actions."},
		AntiTriggers:        []string{"Do not execute write operations.", "Do not claim release readiness; use orag-self-check for check-only requests."},
		CallOrder:           []string{"Collect symptom, trace, and command evidence.", "Call the diagnostic MCP tool.", "Report findings, severity, recommended actions, and verification commands."},
		SafetyBoundaries:    []string{"Read-only only.", "If a write is required, recommend switching to orag-self-ops dry-run planning."},
		FailureHandling:     []string{"Return blocked when evidence is insufficient.", "Preserve trace IDs and failed command output as evidence."},
		ExamplePrompts:      []string{prompt},
		MutualExclusionKey:  "self-diagnose",
		MutualExclusionNote: "Diagnosis interprets evidence; self-check only gathers status, and self-ops handles authorized write plans.",
	}
}

func opsSkill(description, prompt string) SkillBehavior {
	return SkillBehavior{
		ManifestName:        "orag-self-ops",
		Description:         description,
		TriggerConditions:   []string{"User asks for a dry-run maintenance plan.", "User explicitly authorizes a low-risk operational action."},
		AntiTriggers:        []string{"Do not use for read-only health checks.", "Do not apply actions without explicit approval and fresh precondition checks."},
		CallOrder:           []string{"Generate or read a dry-run plan.", "Verify snapshot, preconditions, idempotency key, and lock key.", "Apply only if the capability and user authorization permit writes."},
		SafetyBoundaries:    []string{"Default to dry-run.", "Block on precondition drift.", "Use single-flight locking and idempotency keys for writes."},
		FailureHandling:     []string{"Return blocked when authorization is absent.", "Return blocked when snapshot or preconditions drift."},
		ExamplePrompts:      []string{prompt},
		MutualExclusionKey:  "self-ops",
		MutualExclusionNote: "Self-ops is the only Skill allowed to enter authorized write workflows.",
	}
}
