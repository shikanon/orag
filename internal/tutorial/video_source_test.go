package tutorial

import (
	"errors"
	"testing"
)

func TestValidateVideoSourceAcceptsVerifiedPrivateMetadata(t *testing.T) {
	err := ValidateVideoSource(VideoSource{
		Alias: "clip-a", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Bytes: 1024, ContentType: "video/mp4", DurationMS: 20_000,
		Subtitles: []TimedSubtitle{{StartMS: 0, EndMS: 2_000, Text: "opening scene"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateTimedSubtitlesRejectsNonMonotonicIntervals(t *testing.T) {
	err := ValidateTimedSubtitles([]TimedSubtitle{{StartMS: 10, EndMS: 20, Text: "a"}, {StartMS: 15, EndMS: 30, Text: "b"}})
	if !errors.Is(err, ErrVideoSourceInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateVideoSourceRejectsUntrustedCoordinates(t *testing.T) {
	for name, source := range map[string]VideoSource{
		"digest":   {Alias: "clip-a", SHA256: "not-a-digest", Bytes: 1, ContentType: "video/mp4", DurationMS: 1},
		"type":     {Alias: "clip-a", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Bytes: 1, ContentType: "text/plain", DurationMS: 1},
		"duration": {Alias: "clip-a", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Bytes: 1, ContentType: "video/mp4", DurationMS: 0},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateVideoSource(source); !errors.Is(err, ErrVideoSourceInvalid) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
