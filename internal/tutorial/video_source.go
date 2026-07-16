package tutorial

import (
	"errors"
	"fmt"
	"strings"
)

const (
	maxVideoSourceBytes   = int64(128 << 30)
	maxVideoDurationMS    = int64(24 * 60 * 60 * 1_000)
	maxTimedSubtitleItems = 100_000
	maxTimedSubtitleChars = 8_000
)

var ErrVideoSourceInvalid = errors.New("private video source is invalid")

// VideoSource is private experiment state. Object coordinates are deliberately
// absent: only the verified digest and source alias travel into temporal
// derivation and neither must be exposed from public protocol endpoints.
type VideoSource struct {
	Alias       string          `json:"alias"`
	SHA256      string          `json:"sha256"`
	Bytes       int64           `json:"bytes"`
	ContentType string          `json:"content_type"`
	DurationMS  int64           `json:"duration_ms"`
	Subtitles   []TimedSubtitle `json:"subtitles,omitempty"`
}

type TimedSubtitle struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

func ValidateVideoSource(source VideoSource) error {
	if strings.TrimSpace(source.Alias) == "" || len(source.Alias) > 200 || !sha256Pattern.MatchString(source.SHA256) {
		return fmt.Errorf("%w: alias or digest", ErrVideoSourceInvalid)
	}
	if source.Bytes <= 0 || source.Bytes > maxVideoSourceBytes || source.DurationMS <= 0 || source.DurationMS > maxVideoDurationMS {
		return fmt.Errorf("%w: bounds", ErrVideoSourceInvalid)
	}
	switch strings.ToLower(strings.TrimSpace(source.ContentType)) {
	case "video/mp4", "video/webm", "video/quicktime":
	default:
		return fmt.Errorf("%w: content type", ErrVideoSourceInvalid)
	}
	return ValidateTimedSubtitles(source.Subtitles)
}

func ValidateTimedSubtitles(items []TimedSubtitle) error {
	if len(items) > maxTimedSubtitleItems {
		return fmt.Errorf("%w: subtitle count", ErrVideoSourceInvalid)
	}
	var previousEnd int64
	for index, item := range items {
		if item.StartMS < 0 || item.EndMS <= item.StartMS || len([]rune(strings.TrimSpace(item.Text))) == 0 || len([]rune(item.Text)) > maxTimedSubtitleChars {
			return fmt.Errorf("%w: subtitle %d", ErrVideoSourceInvalid, index)
		}
		if index > 0 && item.StartMS < previousEnd {
			return fmt.Errorf("%w: subtitle %d overlaps predecessor", ErrVideoSourceInvalid, index)
		}
		previousEnd = item.EndMS
	}
	return nil
}
