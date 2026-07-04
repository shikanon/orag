package optimizer

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHarnessRunnerRejectsShellCommandAndInterpolation(t *testing.T) {
	runner := HarnessRunner{}
	_, err := runner.Run(t.Context(), HarnessRunRequest{
		Candidate: HarnessCandidate{Command: "codex-cli eval ${DATASET_PATH}"},
	})
	if err == nil || !strings.Contains(err.Error(), "command strings are not allowed") {
		t.Fatalf("Run() error = %v, want command string rejection", err)
	}

	_, err = runner.Run(t.Context(), HarnessRunRequest{
		Candidate: HarnessCandidate{Argv: []string{testExecutable(t), "${DATASET_PATH}"}},
	})
	if err == nil || !strings.Contains(err.Error(), "interpolation is not allowed") {
		t.Fatalf("Run() error = %v, want interpolation rejection", err)
	}
}

func TestHarnessRunnerRequiresExecutableAllowlist(t *testing.T) {
	exe := testExecutable(t)
	runner := HarnessRunner{
		ExecutableAllowlist: []string{"/not/allowed"},
		BaseWorkingDir:      t.TempDir(),
	}
	_, err := runner.Run(t.Context(), HarnessRunRequest{
		Candidate: HarnessCandidate{Argv: []string{exe, "-test.run=TestHarnessHelper", "--", "success"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not in allowlist") {
		t.Fatalf("Run() error = %v, want allowlist rejection", err)
	}
}

func TestHarnessRunnerRunsArgvAndRedactsSensitiveData(t *testing.T) {
	exe := testExecutable(t)
	secret := "super-secret-token"
	runner := HarnessRunner{
		ExecutableAllowlist: []string{exe},
		BaseWorkingDir:      t.TempDir(),
		Timeout:             time.Second,
	}
	result, err := runner.Run(t.Context(), HarnessRunRequest{
		Candidate: HarnessCandidate{
			Kind: "codex-cli",
			Argv: []string{exe, "-test.run=TestHarnessHelper", "--", "success", "--api-key=" + secret},
		},
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"HARNESS_API_TOKEN":      secret,
			"SAFE_VALUE":             "visible",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Metrics["faithfulness"] != 0.9 || result.Metrics["latency_p95_ms"] != 12 {
		t.Fatalf("metrics = %#v, want parsed registered metrics", result.Metrics)
	}
	if result.EnvRedacted["HARNESS_API_TOKEN"] != "[REDACTED]" {
		t.Fatalf("redacted env = %#v, want token redacted", result.EnvRedacted)
	}
	joinedArgv := strings.Join(result.ArgvRedacted, " ")
	if strings.Contains(joinedArgv, secret) || !strings.Contains(joinedArgv, "--api-key=[REDACTED]") {
		t.Fatalf("redacted argv = %q, want secret removed", joinedArgv)
	}
	if strings.Contains(result.StdoutRedacted, secret) || strings.Contains(result.StderrRedacted, secret) {
		t.Fatalf("stdout/stderr leaked secret: stdout=%q stderr=%q", result.StdoutRedacted, result.StderrRedacted)
	}
	if result.ArtifactManifest["token"] != "[REDACTED]" {
		t.Fatalf("artifact manifest = %#v, want token redacted", result.ArtifactManifest)
	}
	nested, ok := result.ArtifactManifest["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested artifact manifest = %#v, want map", result.ArtifactManifest["nested"])
	}
	if strings.Contains(fmt.Sprint(nested["url"]), secret) {
		t.Fatalf("nested artifact url leaked secret: %#v", nested)
	}
}

func TestHarnessRunnerRejectsUnknownMetric(t *testing.T) {
	exe := testExecutable(t)
	runner := HarnessRunner{
		ExecutableAllowlist: []string{exe},
		BaseWorkingDir:      t.TempDir(),
		Timeout:             time.Second,
	}
	_, err := runner.Run(t.Context(), HarnessRunRequest{
		Candidate: HarnessCandidate{Argv: []string{exe, "-test.run=TestHarnessHelper", "--", "unknown-metric"}},
		Env:       map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown evaluation metric") {
		t.Fatalf("Run() error = %v, want unknown metric rejection", err)
	}
}

func TestHarnessRunnerTimeout(t *testing.T) {
	exe := testExecutable(t)
	runner := HarnessRunner{
		ExecutableAllowlist: []string{exe},
		BaseWorkingDir:      t.TempDir(),
		Timeout:             10 * time.Millisecond,
	}
	result, err := runner.Run(t.Context(), HarnessRunRequest{
		Candidate: HarnessCandidate{Argv: []string{exe, "-test.run=TestHarnessHelper", "--", "sleep"}},
		Env:       map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Run() error = %v, want timeout", err)
	}
	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
}

func TestHarnessHelper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	mode := helperMode()
	secret := os.Getenv("HARNESS_API_TOKEN")
	switch mode {
	case "success":
		fmt.Fprintf(os.Stderr, "stderr token=%s\n", secret)
		fmt.Printf(`{"metrics":{"faithfulness":0.9,"latency_p95_ms":12},"artifact_manifest":{"path":"/tmp/out.json","token":"%s","nested":{"url":"https://example.test/?token=%s"}}}`, secret, secret)
	case "unknown-metric":
		fmt.Print(`{"metrics":{"harness_custom":0.42}}`)
	case "sleep":
		time.Sleep(time.Second)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q", mode)
		os.Exit(2)
	}
	os.Exit(0)
}

func helperMode() string {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func testExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	return exe
}
