package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Manifest struct {
	SchemaVersion string   `json:"schema_version"`
	CreatedAt     string   `json:"created_at"`
	BuildRevision string   `json:"build_revision"`
	Migrations    []string `json:"migrations"`
	Artifacts     []string `json:"artifacts"`
}

func Verify(dir string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	if strings.Contains(strings.ToLower(string(raw)), "secret") || strings.Contains(strings.ToLower(string(raw)), "access_key") {
		return Manifest{}, fmt.Errorf("manifest contains forbidden credential field")
	}
	var m Manifest
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return Manifest{}, err
	}
	if m.SchemaVersion != "orag.backup.v1" || m.CreatedAt == "" || m.BuildRevision == "" || len(m.Migrations) == 0 {
		return Manifest{}, fmt.Errorf("backup manifest provenance is incomplete")
	}
	need := map[string]bool{"postgres.dump": false, "qdrant-snapshots.tgz": false}
	for _, a := range m.Artifacts {
		if _, ok := need[a]; ok {
			need[a] = true
		}
	}
	for a, ok := range need {
		if !ok {
			return Manifest{}, fmt.Errorf("manifest missing required artifact %q", a)
		}
		if _, err := os.Stat(filepath.Join(dir, a)); err != nil {
			return Manifest{}, err
		}
	}
	sums, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return Manifest{}, err
	}
	for a := range need {
		if !containsSum(string(sums), a, filepath.Join(dir, a)) {
			return Manifest{}, fmt.Errorf("checksum missing or invalid for %s", a)
		}
	}
	return m, nil
}
func containsSum(s, name, path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	got := sha256.Sum256(raw)
	want := hex.EncodeToString(got[:])
	for _, l := range strings.Split(s, "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && strings.TrimPrefix(f[1], "*") == name && f[0] == want {
			return true
		}
	}
	return false
}
