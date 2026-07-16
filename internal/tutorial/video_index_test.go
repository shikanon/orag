package tutorial

import (
	"os"
	"strings"
	"testing"
)

func TestWriteTemporalIndexIsDeterministicAndCoordinateFree(t *testing.T) {
	segments := []TemporalSegment{{ID: "private-id", EvidenceID: "clip@0-1000", StartMS: 0, EndMS: 1000, SubtitleText: "hello"}}
	object, path, err := WriteTemporalIndex(t.TempDir(), segments)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	content, err := os.ReadFile(path)
	if err != nil || object.Path != temporalIndexPath || object.ContentType != "text/plain" || object.Bytes != int64(len(content)) || !strings.Contains(string(content), "evidence=clip@0-1000") || strings.Contains(string(content), "private-id") {
		t.Fatalf("object=%#v content=%q err=%v", object, content, err)
	}
}
