package prompt

import (
	"strings"
	"testing"
)

func TestAutoStrategyStableSegmentsFirst(t *testing.T) {
	got := NewStrategy("auto").Apply([]Segment{
		{Name: "question", Content: "Q", Stable: false},
		{Name: "system", Content: "S", Stable: true},
		{Name: "context", Content: "C", Stable: true},
	})
	if !strings.HasPrefix(got, "C\nS\n") {
		t.Fatalf("stable segments not first or deterministic: %q", got)
	}
}

func TestManualStrategyMarksStableSegments(t *testing.T) {
	got := NewStrategy("manual").Apply([]Segment{{Name: "system", Content: "S", Stable: true}})
	if !strings.Contains(got, "cache_control") {
		t.Fatalf("expected manual cache marker, got %q", got)
	}
}
