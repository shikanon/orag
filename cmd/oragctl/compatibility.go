package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shikanon/orag/internal/compatibility"
)

type compatibilityExceptionFile struct {
	Exceptions []compatibilityException `json:"exceptions"`
}

type compatibilityException struct {
	Finding   string `json:"finding"`
	Migration string `json:"migration"`
	Release   string `json:"release"`
}

func compatibilityAuditCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("compatibility-audit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	base := fs.String("base", "", "previous published git tag")
	allowFile := fs.String("allow-file", "compatibility-exceptions.json", "JSON file containing exact migration-backed exceptions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*base) == "" {
		return fmt.Errorf("base is required")
	}
	baseOpenAPI, err := gitShow(*base, "api/openapi.yaml")
	if err != nil {
		return err
	}
	currentOpenAPI, err := os.ReadFile(filepath.Join("api", "openapi.yaml"))
	if err != nil {
		return err
	}
	baseSDK, err := gitSDKFiles(*base)
	if err != nil {
		return err
	}
	currentSDK, err := currentSDKFiles()
	if err != nil {
		return err
	}
	findings, err := compatibility.Audit(baseOpenAPI, currentOpenAPI, baseSDK, currentSDK)
	if err != nil {
		return err
	}
	allowed, err := loadCompatibilityExceptions(*allowFile)
	if err != nil {
		return err
	}
	blocking := make([]string, 0, len(findings))
	for _, finding := range findings {
		if _, ok := allowed[finding.ID]; !ok {
			blocking = append(blocking, finding.ID)
		}
	}
	if len(blocking) > 0 {
		return fmt.Errorf("breaking compatibility changes against %s:\n%s", *base, strings.Join(blocking, "\n"))
	}
	_, err = fmt.Fprintf(out, "compatible base=%s findings=%d exceptions=%d\n", *base, len(findings), len(allowed))
	return err
}

func gitShow(ref, path string) ([]byte, error) {
	command := exec.Command("git", "show", ref+":"+path)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", ref, path, err)
	}
	return output, nil
}

func gitSDKFiles(ref string) (map[string][]byte, error) {
	command := exec.Command("git", "ls-tree", "-r", "--name-only", ref)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s: %w", ref, err)
	}
	files := make(map[string][]byte)
	for _, path := range strings.Fields(string(output)) {
		if !isRootSDKFile(path) {
			continue
		}
		body, err := gitShow(ref, path)
		if err != nil {
			return nil, err
		}
		files[path] = body
	}
	return files, nil
}

func currentSDKFiles() (map[string][]byte, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() || !isRootSDKFile(entry.Name()) {
			continue
		}
		body, err := os.ReadFile(entry.Name())
		if err != nil {
			return nil, err
		}
		files[entry.Name()] = body
	}
	return files, nil
}

func isRootSDKFile(path string) bool {
	return filepath.Dir(path) == "." && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

func loadCompatibilityExceptions(path string) (map[string]compatibilityException, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file compatibilityExceptionFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse compatibility exceptions: %w", err)
	}
	allowed := make(map[string]compatibilityException, len(file.Exceptions))
	for _, exception := range file.Exceptions {
		exception.Finding = strings.TrimSpace(exception.Finding)
		exception.Migration = strings.TrimSpace(exception.Migration)
		exception.Release = strings.TrimSpace(exception.Release)
		if exception.Finding == "" || strings.Contains(exception.Finding, "*") || exception.Migration == "" || exception.Release == "" {
			return nil, fmt.Errorf("compatibility exception requires exact finding, migration and release")
		}
		if _, exists := allowed[exception.Finding]; exists {
			return nil, fmt.Errorf("duplicate compatibility exception %q", exception.Finding)
		}
		allowed[exception.Finding] = exception
	}
	return allowed, nil
}

func sortedExceptionFindings(allowed map[string]compatibilityException) []string {
	values := make([]string, 0, len(allowed))
	for finding := range allowed {
		values = append(values, finding)
	}
	sort.Strings(values)
	return values
}
