package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompatibilityAuditRequiresBase(t *testing.T) {
	if err := compatibilityAuditCmd(nil, os.Stdout); err == nil || err.Error() != "base is required" {
		t.Fatalf("compatibilityAuditCmd() error=%v", err)
	}
}

func TestLoadCompatibilityExceptionsRequiresPreciseMigration(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	if err := os.WriteFile(valid, []byte(`{"exceptions":[{"finding":"sdk.symbol_removed:method Client.Query","migration":"docs/migration.md","release":"v0.1.0-beta.3"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	allowed, err := loadCompatibilityExceptions(valid)
	if err != nil || len(allowed) != 1 {
		t.Fatalf("load valid exceptions allowed=%v err=%v", allowed, err)
	}
	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"exceptions":[{"finding":"sdk.*","migration":"","release":"v0.1.0-beta.3"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCompatibilityExceptions(invalid); err == nil {
		t.Fatal("loadCompatibilityExceptions() accepted wildcard exception")
	}
}
