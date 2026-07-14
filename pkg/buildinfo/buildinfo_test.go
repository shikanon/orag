package buildinfo

import "testing"

func TestCurrentReturnsLinkerMetadata(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() { Version, Commit, BuildTime = originalVersion, originalCommit, originalBuildTime })
	Version, Commit, BuildTime = "v0.1.0-beta.1", "abc123", "2026-07-14T00:00:00Z"
	info := Current()
	if info.Version != Version || info.Commit != Commit || info.BuildTime != BuildTime {
		t.Fatalf("Current() = %#v", info)
	}
}
