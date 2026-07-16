package tutorial

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type TemporalSegment struct {
	ID           string `json:"id"`
	EvidenceID   string `json:"evidence_id"`
	StartMS      int64  `json:"start_ms"`
	EndMS        int64  `json:"end_ms"`
	SubtitleText string `json:"subtitle_text,omitempty"`
}

// BuildTemporalSegments uses the protocol cadence only. It never accepts
// timestamps, paths, provider settings, or frame selection from a browser.
func BuildTemporalSegments(source VideoSource, protocol VideoBenchmarkProtocol) ([]TemporalSegment, error) {
	if err := ValidateVideoSource(source); err != nil {
		return nil, err
	}
	if protocol.Runtime.Profile != "temporal_page" || protocol.Sampling.SegmentMilliseconds <= 0 {
		return nil, fmt.Errorf("%w: temporal runtime", ErrVideoProtocolInvalid)
	}
	count := int((source.DurationMS + protocol.Sampling.SegmentMilliseconds - 1) / protocol.Sampling.SegmentMilliseconds)
	if count > protocol.Sampling.MaxSegments {
		return nil, fmt.Errorf("%w: source exceeds protocol segment cap", ErrVideoSourceInvalid)
	}
	segments := make([]TemporalSegment, 0, count)
	for start := int64(0); start < source.DurationMS; start += protocol.Sampling.SegmentMilliseconds {
		end := start + protocol.Sampling.SegmentMilliseconds
		if end > source.DurationMS {
			end = source.DurationMS
		}
		evidenceID, err := protocol.EvidenceID(source.Alias, start, end)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", source.SHA256, start, end, protocol.Runtime.ExtractorVersion)))
		segments = append(segments, TemporalSegment{
			ID:           hex.EncodeToString(digest[:]),
			EvidenceID:   evidenceID,
			StartMS:      start,
			EndMS:        end,
			SubtitleText: subtitlesOverlapping(source.Subtitles, start, end),
		})
	}
	return segments, nil
}

func subtitlesOverlapping(items []TimedSubtitle, startMS, endMS int64) string {
	text := make([]string, 0)
	for _, item := range items {
		if item.EndMS <= startMS {
			continue
		}
		if item.StartMS >= endMS {
			break
		}
		text = append(text, strings.TrimSpace(item.Text))
	}
	return strings.Join(text, "\n")
}
