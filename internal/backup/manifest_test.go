package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerify(t *testing.T) {
	d := t.TempDir()
	for n, s := range map[string]string{"postgres.dump": "pg", "qdrant-snapshots.tgz": "qd"} {
		if err := os.WriteFile(filepath.Join(d, n), []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sums := ""
	for _, n := range []string{"postgres.dump", "qdrant-snapshots.tgz"} {
		b, _ := os.ReadFile(filepath.Join(d, n))
		h := sha256.Sum256(b)
		sums += hex.EncodeToString(h[:]) + "  " + n + "\n"
	}
	os.WriteFile(filepath.Join(d, "SHA256SUMS"), []byte(sums), 0600)
	os.WriteFile(filepath.Join(d, "manifest.json"), []byte(`{"schema_version":"orag.backup.v1","created_at":"2026-07-17T00:00:00Z","build_revision":"x","migrations":["000001"],"artifacts":["postgres.dump","qdrant-snapshots.tgz"]}`), 0600)
	if _, err := Verify(d); err != nil {
		t.Fatal(err)
	}
}
