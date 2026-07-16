package tutorial

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	videoProtocolBenchmarkID       = "Video-MME"
	videoProtocolPrivateImportMode = "owner_authorized_private_import"
	videoProtocolExtractorVersion  = "temporal-v1"
	minVideoSegmentMilliseconds    = int64(1_000)
	maxVideoSegmentMilliseconds    = int64(10 * 60 * 1_000)
	maxVideoProtocolSegments       = 10_000
)

var ErrVideoProtocolInvalid = errors.New("video benchmark protocol is invalid")

// VideoBenchmarkProtocol is the public, immutable contract for an evaluation
// method. It has no media, annotation, subtitle, source digest, or storage
// coordinate: owners import authorized data into their private workspace.
type VideoBenchmarkProtocol struct {
	TemplateID string          `json:"template_id"`
	Version    string          `json:"version"`
	Tier       string          `json:"tier"`
	Benchmark  VideoBenchmark  `json:"benchmark"`
	Sampling   VideoSampling   `json:"sampling"`
	Runtime    TemporalRuntime `json:"runtime"`
}

type VideoBenchmark struct {
	ID         string `json:"id"`
	SourceURL  string `json:"source_url"`
	ImportMode string `json:"import_mode"`
}

type VideoSampling struct {
	SegmentMilliseconds int64 `json:"segment_milliseconds"`
	MaxSegments         int   `json:"max_segments"`
}

type TemporalRuntime struct {
	Profile          string `json:"profile"`
	TopK             int    `json:"top_k"`
	ExtractorVersion string `json:"extractor_version"`
}

// ParseVideoProtocol verifies the public contract before a private import can
// be associated with it. Decoding is strict so a protocol cannot smuggle a
// media URL or any undisclosed data field into an otherwise valid release.
func ParseVideoProtocol(raw []byte, template Template, pack PackRef) (VideoBenchmarkProtocol, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var protocol VideoBenchmarkProtocol
	if err := decoder.Decode(&protocol); err != nil {
		return VideoBenchmarkProtocol{}, fmt.Errorf("%w: decode: %v", ErrVideoProtocolInvalid, err)
	}
	if decoder.More() {
		return VideoBenchmarkProtocol{}, fmt.Errorf("%w: multiple JSON values", ErrVideoProtocolInvalid)
	}
	if template.Modality != ModalityVideo || protocol.TemplateID != template.ID || protocol.Version != template.Version || protocol.Tier != pack.Tier {
		return VideoBenchmarkProtocol{}, fmt.Errorf("%w: template identity", ErrVideoProtocolInvalid)
	}
	if protocol.Benchmark.ID != videoProtocolBenchmarkID || protocol.Benchmark.SourceURL != template.SourceURL || protocol.Benchmark.ImportMode != videoProtocolPrivateImportMode {
		return VideoBenchmarkProtocol{}, fmt.Errorf("%w: benchmark identity", ErrVideoProtocolInvalid)
	}
	if protocol.Sampling.SegmentMilliseconds < minVideoSegmentMilliseconds || protocol.Sampling.SegmentMilliseconds > maxVideoSegmentMilliseconds || protocol.Sampling.MaxSegments < 1 || protocol.Sampling.MaxSegments > maxVideoProtocolSegments {
		return VideoBenchmarkProtocol{}, fmt.Errorf("%w: sampling limits", ErrVideoProtocolInvalid)
	}
	if protocol.Runtime.Profile != "temporal_page" || protocol.Runtime.TopK < 1 || protocol.Runtime.TopK > 100 || protocol.Runtime.ExtractorVersion != videoProtocolExtractorVersion {
		return VideoBenchmarkProtocol{}, fmt.Errorf("%w: temporal runtime", ErrVideoProtocolInvalid)
	}
	return protocol, nil
}

func (p VideoBenchmarkProtocol) EvidenceID(sourceAlias string, startMS, endMS int64) (string, error) {
	if strings.TrimSpace(sourceAlias) == "" || startMS < 0 || endMS <= startMS {
		return "", fmt.Errorf("%w: evidence coordinates", ErrVideoProtocolInvalid)
	}
	return fmt.Sprintf("%s@%d-%d", sourceAlias, startMS, endMS), nil
}
