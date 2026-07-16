package tutorial

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const temporalIndexPath = "video/temporal/segments.txt"

// WriteTemporalIndex creates the complete, deterministic text representation
// eligible for later retrieval. It contains no media bytes, storage coordinate,
// or source digest; the media remains a separately verified private object.
func WriteTemporalIndex(tempDir string, segments []TemporalSegment) (PackObject, string, error) {
	if len(segments) == 0 {
		return PackObject{}, "", ErrVideoSourceInvalid
	}
	file, err := os.CreateTemp(tempDir, "orag-temporal-index-*.txt")
	if err != nil {
		return PackObject{}, "", err
	}
	path := file.Name()
	failed := true
	defer func() {
		if failed {
			_ = os.Remove(path)
		}
	}()
	hash := sha256.New()
	for index, segment := range segments {
		if index > 0 {
			if _, err := fmt.Fprint(file, "\n\n"); err != nil {
				_ = file.Close()
				return PackObject{}, "", err
			}
			_, _ = hash.Write([]byte("\n\n"))
		}
		text := strings.TrimSpace(segment.IndexText()) + "\n"
		if _, err := file.WriteString(text); err != nil {
			_ = file.Close()
			return PackObject{}, "", err
		}
		_, _ = hash.Write([]byte(text))
	}
	info, err := file.Stat()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return PackObject{}, "", err
	}
	failed = false
	return PackObject{Path: temporalIndexPath, SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: info.Size(), ContentType: "text/plain"}, path, nil
}
