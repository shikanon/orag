package capabilities

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	SchemaVersion = "orag.capabilities.v1"

	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"

	EffectReadOnly = "read_only"
	EffectDryRun   = "dry_run"
	EffectWrite    = "write"
)

var (
	allowedRiskLevels = map[string]struct{}{
		RiskLow:    {},
		RiskMedium: {},
		RiskHigh:   {},
	}
	allowedSideEffects = map[string]struct{}{
		EffectReadOnly: {},
		EffectDryRun:   {},
		EffectWrite:    {},
	}
)

// Manifest is the single source of truth for agent-facing ORAG capabilities.
type Manifest struct {
	SchemaVersion     string               `json:"schema_version"`
	CapabilityVersion string               `json:"capability_version"`
	GeneratorVersion  string               `json:"generator_version"`
	Generation        GenerationMetadata   `json:"generation"`
	Capabilities      []Capability         `json:"capabilities"`
	DriftChecks       []DriftCheckMetadata `json:"drift_checks"`
}

type GenerationMetadata struct {
	OpenAPIFacetPath string   `json:"openapi_facet_path"`
	MCPToolsPath     string   `json:"mcp_tools_path"`
	SkillTargets     []string `json:"skill_targets"`
	ArtifactPaths    []string `json:"artifact_paths"`
}

type DriftCheckMetadata struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
}

type Capability struct {
	ID          string             `json:"id"`
	DisplayName string             `json:"display_name"`
	Description string             `json:"description"`
	Status      string             `json:"status"`
	RiskLevel   string             `json:"risk_level"`
	HTTP        HTTPFacet          `json:"http"`
	MCP         MCPFacet           `json:"mcp"`
	Skill       SkillBehavior      `json:"skill"`
	Operations  OperationsSemantic `json:"operations"`
	Generation  CapabilityArtifact `json:"generation"`
	Examples    []Example          `json:"examples"`
}

type HTTPFacet struct {
	Kind            string   `json:"kind"`
	Method          string   `json:"method"`
	Path            string   `json:"path"`
	OperationID     string   `json:"operation_id"`
	AuthScheme      string   `json:"auth_scheme"`
	RequestSchema   string   `json:"request_schema"`
	ResponseSchema  string   `json:"response_schema"`
	ErrorSchema     string   `json:"error_schema"`
	BackingServices []string `json:"backing_services"`
}

type MCPFacet struct {
	ToolName     string         `json:"tool_name"`
	Description  string         `json:"description"`
	InputSchema  string         `json:"input_schema"`
	OutputSchema string         `json:"output_schema"`
	Annotations  map[string]any `json:"annotations"`
}

type SkillBehavior struct {
	ManifestName        string   `json:"manifest_name"`
	Description         string   `json:"description"`
	TriggerConditions   []string `json:"trigger_conditions"`
	AntiTriggers        []string `json:"anti_triggers"`
	CallOrder           []string `json:"call_order"`
	SafetyBoundaries    []string `json:"safety_boundaries"`
	FailureHandling     []string `json:"failure_handling"`
	ExamplePrompts      []string `json:"example_prompts"`
	MutualExclusionKey  string   `json:"mutual_exclusion_key"`
	MutualExclusionNote string   `json:"mutual_exclusion_note"`
}

type OperationsSemantic struct {
	SideEffect       string   `json:"side_effect"`
	ReadOnly         bool     `json:"read_only"`
	DryRunSupported  bool     `json:"dry_run_supported"`
	RequiresApproval bool     `json:"requires_approval"`
	IdempotencyKey   string   `json:"idempotency_key,omitempty"`
	LockKey          string   `json:"lock_key,omitempty"`
	Rollback         []string `json:"rollback"`
}

type CapabilityArtifact struct {
	OpenAPIFacet   string   `json:"openapi_facet"`
	MCPArtifact    string   `json:"mcp_artifact"`
	SkillArtifacts []string `json:"skill_artifacts"`
}

type Example struct {
	Name           string         `json:"name"`
	Prompt         string         `json:"prompt"`
	Input          map[string]any `json:"input"`
	ExpectedOutput map[string]any `json:"expected_output"`
}

type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, err := range e {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

// LoadJSON reads a capability manifest from JSON and validates it.
func LoadJSON(r io.Reader) (Manifest, error) {
	var manifest Manifest
	if err := json.NewDecoder(r).Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode capability manifest: %w", err)
	}
	if err := Validate(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Validate(manifest Manifest) error {
	var errs ValidationErrors
	add := func(path, message string) {
		errs = append(errs, ValidationError{Path: path, Message: message})
	}

	if strings.TrimSpace(manifest.SchemaVersion) != SchemaVersion {
		add("schema_version", fmt.Sprintf("must be %q", SchemaVersion))
	}
	if strings.TrimSpace(manifest.CapabilityVersion) == "" {
		add("capability_version", "is required")
	}
	if strings.TrimSpace(manifest.GeneratorVersion) == "" {
		add("generator_version", "is required")
	}
	if strings.TrimSpace(manifest.Generation.OpenAPIFacetPath) == "" {
		add("generation.openapi_facet_path", "is required")
	}
	if strings.TrimSpace(manifest.Generation.MCPToolsPath) == "" {
		add("generation.mcp_tools_path", "is required")
	}
	if len(manifest.Generation.SkillTargets) == 0 {
		add("generation.skill_targets", "must not be empty")
	}
	if len(manifest.Capabilities) == 0 {
		add("capabilities", "must not be empty")
	}

	seenTools := map[string]string{}
	for i, capability := range manifest.Capabilities {
		validateCapability(fmt.Sprintf("capabilities[%d]", i), capability, seenTools, add)
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validateCapability(path string, capability Capability, seenTools map[string]string, add func(string, string)) {
	required := map[string]string{
		"id":           capability.ID,
		"display_name": capability.DisplayName,
		"description":  capability.Description,
		"status":       capability.Status,
		"risk_level":   capability.RiskLevel,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			add(path+"."+field, "is required")
		}
	}
	if _, ok := allowedRiskLevels[capability.RiskLevel]; capability.RiskLevel != "" && !ok {
		add(path+".risk_level", "must be one of low, medium, high")
	}

	validateHTTPFacet(path+".http", capability.HTTP, add)
	validateMCPFacet(path+".mcp", capability, seenTools, add)
	validateSkillBehavior(path+".skill", capability.Skill, add)
	validateOperations(path+".operations", capability.Operations, add)
	validateGeneration(path+".generation", capability.Generation, add)
	if len(capability.Examples) == 0 {
		add(path+".examples", "must not be empty")
	}
}

func validateHTTPFacet(path string, facet HTTPFacet, add func(string, string)) {
	for field, value := range map[string]string{
		"kind":            facet.Kind,
		"method":          facet.Method,
		"path":            facet.Path,
		"operation_id":    facet.OperationID,
		"request_schema":  facet.RequestSchema,
		"response_schema": facet.ResponseSchema,
		"error_schema":    facet.ErrorSchema,
	} {
		if strings.TrimSpace(value) == "" {
			add(path+"."+field, "is required")
		}
	}
	if facet.Method != "" && facet.Method != http.MethodGet && facet.Method != http.MethodPost {
		add(path+".method", "must be GET or POST for agent-facing capabilities")
	}
}

func validateMCPFacet(path string, capability Capability, seenTools map[string]string, add func(string, string)) {
	facet := capability.MCP
	for field, value := range map[string]string{
		"tool_name":     facet.ToolName,
		"description":   facet.Description,
		"input_schema":  facet.InputSchema,
		"output_schema": facet.OutputSchema,
	} {
		if strings.TrimSpace(value) == "" {
			add(path+"."+field, "is required")
		}
	}
	if facet.ToolName != "" {
		if other, exists := seenTools[facet.ToolName]; exists {
			add(path+".tool_name", fmt.Sprintf("duplicates %s", other))
		} else {
			seenTools[facet.ToolName] = capability.ID
		}
	}
}

func validateSkillBehavior(path string, skill SkillBehavior, add func(string, string)) {
	for field, value := range map[string]string{
		"manifest_name":        skill.ManifestName,
		"description":          skill.Description,
		"mutual_exclusion_key": skill.MutualExclusionKey,
	} {
		if strings.TrimSpace(value) == "" {
			add(path+"."+field, "is required")
		}
	}
	if len(skill.TriggerConditions) == 0 {
		add(path+".trigger_conditions", "must not be empty")
	}
	if len(skill.AntiTriggers) == 0 {
		add(path+".anti_triggers", "must define trigger boundaries")
	}
	if len(skill.CallOrder) == 0 {
		add(path+".call_order", "must not be empty")
	}
	if len(skill.SafetyBoundaries) == 0 {
		add(path+".safety_boundaries", "must not be empty")
	}
	if len(skill.FailureHandling) == 0 {
		add(path+".failure_handling", "must not be empty")
	}
	if len(skill.ExamplePrompts) == 0 {
		add(path+".example_prompts", "must not be empty")
	}
}

func validateOperations(path string, ops OperationsSemantic, add func(string, string)) {
	if _, ok := allowedSideEffects[ops.SideEffect]; !ok {
		add(path+".side_effect", "must be one of read_only, dry_run, write")
	}
	if ops.SideEffect == EffectReadOnly && !ops.ReadOnly {
		add(path+".read_only", "must be true when side_effect is read_only")
	}
	if ops.SideEffect == EffectDryRun && !ops.DryRunSupported {
		add(path+".dry_run_supported", "must be true when side_effect is dry_run")
	}
	if ops.SideEffect == EffectWrite && !ops.RequiresApproval {
		add(path+".requires_approval", "must be true when side_effect is write")
	}
	if ops.RequiresApproval && strings.TrimSpace(ops.IdempotencyKey) == "" {
		add(path+".idempotency_key", "is required for approved operations")
	}
	if ops.RequiresApproval && strings.TrimSpace(ops.LockKey) == "" {
		add(path+".lock_key", "is required for approved operations")
	}
}

func validateGeneration(path string, generation CapabilityArtifact, add func(string, string)) {
	if strings.TrimSpace(generation.OpenAPIFacet) == "" {
		add(path+".openapi_facet", "is required")
	}
	if strings.TrimSpace(generation.MCPArtifact) == "" {
		add(path+".mcp_artifact", "is required")
	}
	if len(generation.SkillArtifacts) == 0 {
		add(path+".skill_artifacts", "must not be empty")
	}
}

func IsValidationError(err error) bool {
	var validationErrors ValidationErrors
	return errors.As(err, &validationErrors)
}
