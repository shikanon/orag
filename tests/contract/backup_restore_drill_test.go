package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestBackupRestoreDrillIsDocumented(t *testing.T) {
	checks := map[string][]string{
		"../../Makefile": {
			"backup-restore-drill:",
			"./scripts/backup-restore-drill.sh",
		},
		"../../scripts/backup-restore-drill.sh": {
			"pg_dump",
			"snapshots/upload?priority=snapshot",
			"backup-verify",
			"drill-evidence.json",
			"target-knowledge-bases.json",
		},
		"../../docs/operations/disaster-recovery.md": {
			"make backup-restore-drill",
			"Docker Compose, `curl`, `jq`, `tar`, `shasum`, and Go",
			"not evidence of a production RPO/RTO",
		},
		"../../docs-site/disaster-recovery.html": {
			"make backup-restore-drill",
			"cited query plus trace",
			"Production RPO/RTO",
		},
		"../../ROADMAP.md": {
			"本地隔离的 PostgreSQL + Qdrant 完整备份恢复演练",
		},
		"../../ROADMAP_EN.md": {
			"complete isolated PostgreSQL + Qdrant backup/restore drill",
		},
	}

	for path, phrases := range checks {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		for _, phrase := range phrases {
			if !strings.Contains(string(body), phrase) {
				t.Errorf("%s missing %q", path, phrase)
			}
		}
	}
}
