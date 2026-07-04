package optimizer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/apperrors"
)

const defaultHarnessTimeout = 5 * time.Minute

type HarnessRunner struct {
	ExecutableAllowlist []string
	BaseWorkingDir      string
	Timeout             time.Duration
	MetricRegistry      eval.MetricRegistry
}

type HarnessRunRequest struct {
	Candidate  HarnessCandidate
	Env        map[string]string
	WorkingDir string
}

type HarnessRunResult struct {
	Kind              string
	ArgvRedacted      []string
	WorkingDir        string
	EnvRedacted       map[string]string
	StdoutRedacted    string
	StderrRedacted    string
	ArtifactManifest  map[string]any
	Metrics           map[string]float64
	ExitCode          int
	Duration          time.Duration
	TimedOut          bool
	ExecutableAllowed bool
}

type harnessOutput struct {
	Metrics          map[string]float64 `json:"metrics"`
	ArtifactManifest map[string]any     `json:"artifact_manifest"`
	Artifacts        map[string]any     `json:"artifacts"`
}

func (r HarnessRunner) Run(ctx context.Context, req HarnessRunRequest) (HarnessRunResult, error) {
	if err := validateHarnessCandidate(req.Candidate); err != nil {
		return HarnessRunResult{}, err
	}
	executable, err := r.resolveExecutable(req.Candidate.Argv[0])
	if err != nil {
		return HarnessRunResult{}, err
	}
	if !r.executableAllowed(req.Candidate.Argv[0], executable) {
		return HarnessRunResult{}, validationError("harness executable %q is not in allowlist", req.Candidate.Argv[0])
	}
	workingDir, err := r.prepareWorkingDir(req.WorkingDir)
	if err != nil {
		return HarnessRunResult{}, err
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultHarnessTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	redactor := newHarnessRedactor(req.Env, req.Candidate.Argv)
	result := HarnessRunResult{
		Kind:              req.Candidate.Kind,
		ArgvRedacted:      redactor.redactArgv(req.Candidate.Argv),
		WorkingDir:        workingDir,
		EnvRedacted:       redactor.redactEnv(req.Env),
		Metrics:           map[string]float64{},
		ArtifactManifest:  map[string]any{},
		ExecutableAllowed: true,
		ExitCode:          -1,
	}

	cmd := exec.CommandContext(runCtx, executable, req.Candidate.Argv[1:]...)
	cmd.Dir = workingDir
	cmd.Env = append(os.Environ(), envPairs(req.Env)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err = cmd.Run()
	result.Duration = time.Since(started)
	result.StdoutRedacted = redactor.redactString(stdout.String())
	result.StderrRedacted = redactor.redactString(stderr.String())
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		return result, apperrors.New(apperrors.CodeUpstreamUnavailable, "external harness timed out")
	}
	if err != nil {
		return result, apperrors.Wrap(apperrors.CodeUpstreamUnavailable, "external harness failed", err)
	}

	parsed, err := parseHarnessOutput(stdout.Bytes())
	if err != nil {
		return result, err
	}
	metrics := parsed.Metrics
	if metrics == nil {
		metrics = map[string]float64{}
	}
	if err := r.metricRegistry().Validate(metrics); err != nil {
		return result, err
	}
	result.Metrics = metrics
	manifest := parsed.ArtifactManifest
	if manifest == nil {
		manifest = parsed.Artifacts
	}
	if manifest == nil {
		manifest = map[string]any{}
	}
	result.ArtifactManifest = redactor.redactMap(manifest)
	return result, nil
}

func validateHarnessCandidate(candidate HarnessCandidate) error {
	if strings.TrimSpace(candidate.Command) != "" {
		return validationError("external harness command strings are not allowed; use argv array")
	}
	if len(candidate.Argv) == 0 {
		return validationError("external harness argv array is required")
	}
	for _, arg := range candidate.Argv {
		if strings.Contains(arg, "${") {
			return validationError("external harness argv interpolation is not allowed")
		}
	}
	return nil
}

func (r HarnessRunner) resolveExecutable(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", validationError("external harness executable is required")
	}
	resolved := name
	var err error
	if !filepath.IsAbs(name) {
		resolved, err = exec.LookPath(name)
		if err != nil {
			return "", apperrors.Wrap(apperrors.CodeValidation, fmt.Sprintf("harness executable %q not found", name), err)
		}
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", apperrors.Wrap(apperrors.CodeValidation, "resolve harness executable", err)
	}
	return filepath.Clean(resolved), nil
}

func (r HarnessRunner) executableAllowed(raw, resolved string) bool {
	if len(r.ExecutableAllowlist) == 0 {
		return false
	}
	resolved = filepath.Clean(resolved)
	for _, allowed := range r.ExecutableAllowlist {
		if allowed == raw || filepath.Clean(allowed) == resolved {
			return true
		}
		if !filepath.IsAbs(allowed) && filepath.Base(resolved) == allowed {
			return true
		}
	}
	return false
}

func (r HarnessRunner) prepareWorkingDir(requested string) (string, error) {
	base := r.BaseWorkingDir
	if base == "" {
		base = os.TempDir()
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", apperrors.Wrap(apperrors.CodeValidation, "resolve harness base working dir", err)
	}
	if requested == "" {
		return os.MkdirTemp(baseAbs, "orag-harness-*")
	}
	workingDir, err := filepath.Abs(requested)
	if err != nil {
		return "", apperrors.Wrap(apperrors.CodeValidation, "resolve harness working dir", err)
	}
	if !pathWithin(baseAbs, workingDir) {
		return "", validationError("external harness working dir must be inside base working dir")
	}
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		return "", apperrors.Wrap(apperrors.CodeInternal, "create harness working dir", err)
	}
	return filepath.Clean(workingDir), nil
}

func (r HarnessRunner) metricRegistry() eval.MetricRegistry {
	if len(r.MetricRegistry.Names()) == 0 {
		return eval.DefaultMetricRegistry
	}
	return r.MetricRegistry
}

func pathWithin(base, path string) bool {
	base = filepath.Clean(base)
	path = filepath.Clean(path)
	if path == base {
		return true
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func envPairs(env map[string]string) []string {
	pairs := make([]string, 0, len(env))
	for key, value := range env {
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}

func parseHarnessOutput(stdout []byte) (harnessOutput, error) {
	if len(bytes.TrimSpace(stdout)) == 0 {
		return harnessOutput{}, nil
	}
	var parsed harnessOutput
	if err := json.Unmarshal(stdout, &parsed); err != nil {
		return harnessOutput{}, apperrors.Wrap(apperrors.CodeValidation, "parse external harness metrics JSON", err)
	}
	return parsed, nil
}

type harnessRedactor struct {
	sensitiveValues []string
}

var (
	sensitiveKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|authorization|bearer|credential|password|secret|token)`)
	inlineSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|api[_-]?key=|token=|password=|secret=)([^\s"',;]+)`)
)

func newHarnessRedactor(env map[string]string, argv []string) harnessRedactor {
	seen := map[string]struct{}{}
	var values []string
	addValue := func(value string) {
		value = strings.TrimSpace(value)
		if len(value) < 4 {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	for key, value := range env {
		if sensitiveKeyPattern.MatchString(key) || sensitiveKeyPattern.MatchString(value) {
			addValue(value)
		}
	}
	for _, arg := range argv {
		if sensitiveKeyPattern.MatchString(arg) {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				addValue(parts[1])
			}
		}
	}
	return harnessRedactor{sensitiveValues: values}
}

func (r harnessRedactor) redactEnv(env map[string]string) map[string]string {
	redacted := make(map[string]string, len(env))
	for key, value := range env {
		if sensitiveKeyPattern.MatchString(key) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = r.redactString(value)
	}
	return redacted
}

func (r harnessRedactor) redactArgv(argv []string) []string {
	redacted := make([]string, len(argv))
	for i, arg := range argv {
		if sensitiveKeyPattern.MatchString(arg) {
			if strings.Contains(arg, "=") {
				redacted[i] = strings.SplitN(arg, "=", 2)[0] + "=[REDACTED]"
				continue
			}
			redacted[i] = "[REDACTED]"
			continue
		}
		redacted[i] = r.redactString(arg)
	}
	return redacted
}

func (r harnessRedactor) redactMap(input map[string]any) map[string]any {
	redacted := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveKeyPattern.MatchString(key) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = r.redactValue(value)
	}
	return redacted
}

func (r harnessRedactor) redactValue(value any) any {
	switch typed := value.(type) {
	case string:
		return r.redactString(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = r.redactValue(item)
		}
		return out
	case map[string]any:
		return r.redactMap(typed)
	default:
		return typed
	}
}

func (r harnessRedactor) redactString(input string) string {
	out := input
	for _, value := range r.sensitiveValues {
		out = strings.ReplaceAll(out, value, "[REDACTED]")
	}
	out = inlineSecretPattern.ReplaceAllString(out, "${1}[REDACTED]")
	return out
}
