package tutorial

import (
	"errors"
	"strings"
	"testing"
)

func TestCatalogLoadsValidatedOfficialTextReplay(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	replay, err := catalog.Replay("text-rag")
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != "text-rag/1.0.0/benchmark/replay-v1" || replay.Fingerprint == "" || replay.Baseline.ContextPackTopN != TutorialBaselineContextPackTopN || replay.Candidate.ContextPackTopN != TutorialP8ContextPackTopN {
		t.Fatalf("replay = %#v", replay)
	}
	items := catalog.List()
	for _, item := range items {
		if item.ID == "text-rag" && !item.ReplayAvailable {
			t.Fatal("text-rag replay is not advertised")
		}
		if item.ID != "text-rag" && item.ReplayAvailable {
			t.Fatalf("unpublished replay advertised for %q", item.ID)
		}
	}
	if _, err := catalog.Replay("video-rag"); !errors.Is(err, ErrReplayNotFound) {
		t.Fatalf("video replay error = %v", err)
	}
}

func TestParseReplayRejectsUnknownAndSensitiveFields(t *testing.T) {
	unknown := strings.Replace(string(embeddedTextRAGReplay), "\n}", ",\n  \"unexpected\": true\n}", 1)
	if _, err := parseReplay([]byte(unknown)); err == nil {
		t.Fatal("unknown field accepted")
	}
	sensitive := strings.Replace(string(embeddedTextRAGReplay), "\n}", ",\n  \"access_key\": \"nope\"\n}", 1)
	if _, err := parseReplay([]byte(sensitive)); err == nil {
		t.Fatal("sensitive field accepted")
	}
}

func TestParseReplayRejectsInvalidP8Contract(t *testing.T) {
	invalid := strings.Replace(string(embeddedTextRAGReplay), "\"context_pack_top_n\": 3", "\"context_pack_top_n\": 4", 1)
	if _, err := parseReplay([]byte(invalid)); err == nil {
		t.Fatal("invalid P8 contract accepted")
	}
}

func TestParseReplayRejectsInvalidSHA256(t *testing.T) {
	invalid := strings.Replace(string(embeddedTextRAGReplay), "d3b293d5478d03ec7cc37bd0fbc837c1a9a017764c457c4a1001ece25d890e26", "not-a-sha", 1)
	if _, err := parseReplay([]byte(invalid)); err == nil {
		t.Fatal("invalid SHA-256 accepted")
	}
}
