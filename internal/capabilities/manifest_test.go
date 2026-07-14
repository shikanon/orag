package capabilities

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuiltinManifestValidates(t *testing.T) {
	manifest := BuiltinManifest()
	if err := Validate(manifest); err != nil {
		t.Fatalf("Validate(BuiltinManifest) error = %v", err)
	}

	if manifest.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", manifest.SchemaVersion, SchemaVersion)
	}
	if len(manifest.Capabilities) != 8 {
		t.Fatalf("capability count = %d, want 8", len(manifest.Capabilities))
	}
	requireCapability(t, manifest, "ralph-loop", "ralph_loop_run", "ralph-loop")
	requireCapability(t, manifest, "self-check", "orag_check", "orag-self-check")
	requireCapability(t, manifest, "diagnose", "orag_diagnose", "orag-self-diagnose")
	requireCapability(t, manifest, "apply-low-risk-action", "orag_apply_low_risk_action", "orag-self-ops")
}

func TestLoadJSONValidatesManifest(t *testing.T) {
	body, err := json.Marshal(BuiltinManifest())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("LoadJSON error = %v", err)
	}
	if len(manifest.Capabilities) != len(BuiltinManifest().Capabilities) {
		t.Fatalf("loaded capability count = %d", len(manifest.Capabilities))
	}
}

func TestValidateRejectsMissingRequiredFields(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.Capabilities[0].DisplayName = ""
	manifest.Capabilities[0].HTTP.RequestSchema = ""

	err := Validate(manifest)
	requireValidationError(t, err, "capabilities[0].display_name: is required")
	requireValidationError(t, err, "capabilities[0].http.request_schema: is required")
}

func TestValidateRejectsDuplicateToolName(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.Capabilities[1].MCP.ToolName = manifest.Capabilities[0].MCP.ToolName

	err := Validate(manifest)
	requireValidationError(t, err, "capabilities[1].mcp.tool_name: duplicates ralph-loop")
}

func TestValidateRejectsIncompleteSkillTriggerBoundary(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.Capabilities[1].Skill.TriggerConditions = nil
	manifest.Capabilities[1].Skill.AntiTriggers = nil
	manifest.Capabilities[1].Skill.MutualExclusionKey = ""

	err := Validate(manifest)
	requireValidationError(t, err, "capabilities[1].skill.trigger_conditions: must not be empty")
	requireValidationError(t, err, "capabilities[1].skill.anti_triggers: must define trigger boundaries")
	requireValidationError(t, err, "capabilities[1].skill.mutual_exclusion_key: is required")
}

func TestValidateRejectsInvalidRiskLevel(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.Capabilities[0].RiskLevel = "severe"

	err := Validate(manifest)
	requireValidationError(t, err, "capabilities[0].risk_level: must be one of low, medium, high")
}

func TestValidateRejectsMissingMaturity(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.Capabilities[0].Maturity = ""

	err := Validate(manifest)
	requireValidationError(t, err, "capabilities[0].maturity: is required")
}

func TestValidateRejectsInvalidMaturity(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.Capabilities[0].Maturity = "preview"

	err := Validate(manifest)
	requireValidationError(t, err, "capabilities[0].maturity: must be one of experimental, beta, stable")
}

func TestBuiltinManifestUsesPublishedMaturityValues(t *testing.T) {
	for _, capability := range BuiltinManifest().Capabilities {
		if capability.Maturity != MaturityExperimental {
			t.Errorf("%s maturity = %q, want %q", capability.ID, capability.Maturity, MaturityExperimental)
		}
	}
}

func TestValidateRejectsInvalidOperationsSemantics(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.Capabilities[1].Operations.ReadOnly = false
	manifest.Capabilities[6].Operations.IdempotencyKey = ""
	manifest.Capabilities[6].Operations.LockKey = ""

	err := Validate(manifest)
	requireValidationError(t, err, "capabilities[1].operations.read_only: must be true when side_effect is read_only")
	requireValidationError(t, err, "capabilities[6].operations.idempotency_key: is required for approved operations")
	requireValidationError(t, err, "capabilities[6].operations.lock_key: is required for approved operations")
}

func TestValidateRejectsSchemaVersionMismatch(t *testing.T) {
	manifest := cloneManifest(t, BuiltinManifest())
	manifest.SchemaVersion = "1"

	err := Validate(manifest)
	requireValidationError(t, err, `schema_version: must be "orag.capabilities.v1"`)
}

func requireCapability(t *testing.T, manifest Manifest, id, toolName, skillName string) {
	t.Helper()
	for _, capability := range manifest.Capabilities {
		if capability.ID != id {
			continue
		}
		if capability.MCP.ToolName != toolName {
			t.Fatalf("%s MCP tool = %q, want %q", id, capability.MCP.ToolName, toolName)
		}
		if capability.Skill.ManifestName != skillName {
			t.Fatalf("%s Skill = %q, want %q", id, capability.Skill.ManifestName, skillName)
		}
		return
	}
	t.Fatalf("capability %q not found", id)
}

func requireValidationError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Validate error = nil, want %q", want)
	}
	if !IsValidationError(err) {
		t.Fatalf("Validate error type = %T, want validation error", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Validate error = %q, want to contain %q", err.Error(), want)
	}
}

func cloneManifest(t *testing.T, manifest Manifest) Manifest {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var cloned Manifest
	if err := json.Unmarshal(body, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
