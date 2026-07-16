package tutorial

import (
	"errors"
	"testing"
)

func TestBuildTemporalSegmentsUsesStableEvidenceIDs(t *testing.T) {
	source := VideoSource{
		Alias: "clip-a", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Bytes: 1024, ContentType: "video/mp4", DurationMS: 20_001,
		Subtitles: []TimedSubtitle{{StartMS: 1_000, EndMS: 2_000, Text: "first"}, {StartMS: 12_000, EndMS: 15_000, Text: "second"}},
	}
	protocol, err := ParseVideoProtocol([]byte(validVideoProtocol), videoProtocolTemplate(t), videoProtocolPack(t))
	if err != nil {
		t.Fatal(err)
	}
	first, err := BuildTemporalSegments(source, protocol)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTemporalSegments(source, protocol)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || first[0].EvidenceID != "clip-a@0-10000" || first[0].ID != second[0].ID || first[1].SubtitleText != "second" || first[2].EndMS != 20_001 {
		t.Fatalf("segments=%#v", first)
	}
}

func TestBuildTemporalSegmentsRejectsSourceOverProtocolLimit(t *testing.T) {
	protocol, err := ParseVideoProtocol([]byte(validVideoProtocol), videoProtocolTemplate(t), videoProtocolPack(t))
	if err != nil {
		t.Fatal(err)
	}
	protocol.Sampling.MaxSegments = 1
	_, err = BuildTemporalSegments(VideoSource{Alias: "clip-a", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Bytes: 1, ContentType: "video/mp4", DurationMS: 20_000}, protocol)
	if !errors.Is(err, ErrVideoSourceInvalid) {
		t.Fatalf("err=%v", err)
	}
}
