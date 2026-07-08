package offlineknowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrTenantMismatch          = errors.New("offline knowledge item tenant mismatch")
	ErrKBMismatch              = errors.New("offline knowledge item knowledge base mismatch")
	ErrLowConfidence           = errors.New("offline knowledge item confidence below threshold")
	ErrMissingEvidence         = errors.New("offline knowledge item missing evidence")
	ErrMissingQuote            = errors.New("offline knowledge evidence missing quote")
	ErrMissingFingerprint      = errors.New("offline knowledge item missing source fingerprint")
	ErrSourceNotFound          = errors.New("offline knowledge source chunk not found")
	ErrStaleFingerprint        = errors.New("offline knowledge source fingerprint is stale")
	ErrQuoteNotContained       = errors.New("offline knowledge quote is not contained in source chunk")
	ErrConclusionRejected      = errors.New("offline knowledge conclusion judge rejected item")
	ErrConclusionDisabled      = errors.New("offline knowledge conclusion judge is disabled")
	ErrConclusionUnavailable   = errors.New("offline knowledge conclusion judge is unavailable")
	ErrSourceReaderUnavailable = errors.New("offline knowledge source reader is unavailable")
)

type ValidatorOptions struct {
	MinConfidence float64
}

type Validator struct {
	sourceReader  SourceReader
	judge         ConclusionJudge
	minConfidence float64
}

func NewValidator(sourceReader SourceReader, judge ConclusionJudge, opts ValidatorOptions) *Validator {
	minConfidence := opts.MinConfidence
	if minConfidence == 0 {
		minConfidence = 0.8
	}
	return &Validator{
		sourceReader:  sourceReader,
		judge:         judge,
		minConfidence: minConfidence,
	}
}

func (v *Validator) ValidateItem(ctx context.Context, tenantID, kbID string, item OptimizationItem) error {
	if item.TenantID != tenantID {
		return fmt.Errorf("%w: item tenant %q, expected %q", ErrTenantMismatch, item.TenantID, tenantID)
	}
	if item.KBID != kbID {
		return fmt.Errorf("%w: item kb %q, expected %q", ErrKBMismatch, item.KBID, kbID)
	}
	if item.Confidence < v.minConfidence {
		return fmt.Errorf("%w: item confidence %.4f, threshold %.4f", ErrLowConfidence, item.Confidence, v.minConfidence)
	}
	if len(item.Evidence) == 0 {
		return ErrMissingEvidence
	}
	if len(item.SourceFingerprints) == 0 {
		return ErrMissingFingerprint
	}
	fingerprints := make(map[string]SourceFingerprint, len(item.SourceFingerprints))
	for _, fingerprint := range item.SourceFingerprints {
		if fingerprint.ChunkID != "" {
			fingerprints[fingerprint.ChunkID] = fingerprint
		}
	}
	for _, evidence := range item.Evidence {
		if strings.TrimSpace(evidence.Quote) == "" {
			return fmt.Errorf("%w: chunk %q", ErrMissingQuote, evidence.ChunkID)
		}
		fingerprint, ok := fingerprints[evidence.ChunkID]
		if !ok {
			return fmt.Errorf("%w: chunk %q", ErrMissingFingerprint, evidence.ChunkID)
		}
		if v.sourceReader == nil {
			return ErrSourceReaderUnavailable
		}
		chunk, found, err := v.sourceReader.ReadSourceChunk(ctx, tenantID, kbID, evidence.ChunkID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: chunk %q", ErrSourceNotFound, evidence.ChunkID)
		}
		if err := validateSourceFingerprint(tenantID, kbID, evidence, fingerprint, chunk); err != nil {
			return err
		}
		if !strings.Contains(chunk.Text, evidence.Quote) {
			return fmt.Errorf("%w: quote for chunk %q", ErrQuoteNotContained, evidence.ChunkID)
		}
	}
	if item.ItemType == ItemTypeAnswer {
		if v.judge == nil {
			return ErrConclusionUnavailable
		}
		accepted, err := v.judge.JudgeConclusion(ctx, item, item.Evidence)
		if err != nil {
			return err
		}
		if !accepted {
			return ErrConclusionRejected
		}
	}
	return nil
}

func validateSourceFingerprint(tenantID, kbID string, evidence Evidence, fingerprint SourceFingerprint, chunk SourceChunk) error {
	if chunk.TenantID != tenantID {
		return fmt.Errorf("%w: source chunk %q belongs to tenant %q", ErrTenantMismatch, chunk.ChunkID, chunk.TenantID)
	}
	if chunk.KBID != kbID {
		return fmt.Errorf("%w: source chunk %q belongs to kb %q", ErrKBMismatch, chunk.ChunkID, chunk.KBID)
	}
	if evidence.DocID != "" && chunk.DocID != evidence.DocID {
		return fmt.Errorf("%w: evidence doc %q, source doc %q", ErrStaleFingerprint, evidence.DocID, chunk.DocID)
	}
	if fingerprint.DocID != "" && chunk.DocID != fingerprint.DocID {
		return fmt.Errorf("%w: fingerprint doc %q, source doc %q", ErrStaleFingerprint, fingerprint.DocID, chunk.DocID)
	}
	if fingerprint.DocVersion != chunk.DocVersion {
		return fmt.Errorf("%w: doc version %q, current %q", ErrStaleFingerprint, fingerprint.DocVersion, chunk.DocVersion)
	}
	if fingerprint.ChunkContentHash != chunk.ChunkContentHash {
		return fmt.Errorf("%w: chunk hash %q, current %q", ErrStaleFingerprint, fingerprint.ChunkContentHash, chunk.ChunkContentHash)
	}
	return nil
}
