package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

var ErrVideoImportUnavailable = errors.New("video tutorial import is unavailable")

type VideoImportService struct {
	repo    CloneRepository
	private PrivateStore
	tempDir string
}

func NewVideoImportService(repo CloneRepository, private PrivateStore, tempDir string) *VideoImportService {
	return &VideoImportService{repo: repo, private: private, tempDir: tempDir}
}

// Import accepts an owner-authorized private stream. It never accepts an object
// key or URL and persists only verified metadata and deterministic segments.
func (s *VideoImportService) Import(ctx context.Context, subject Subject, projectID string, source VideoSource, body io.Reader) (VideoSource, []TemporalSegment, error) {
	if s == nil || s.repo == nil || s.private == nil || body == nil {
		return VideoSource{}, nil, ErrVideoImportUnavailable
	}
	experiment, found, err := s.repo.GetExperiment(ctx, subject.TenantID, projectID)
	if err != nil || !found || experiment.TemplateID != "video-rag" || experiment.PackStatus != PackStatusInstalled || experiment.PackManifest.VideoProtocol == nil {
		return VideoSource{}, nil, ErrVideoImportUnavailable
	}
	if err := ValidateVideoSource(source); err != nil {
		return VideoSource{}, nil, err
	}
	file, err := os.CreateTemp(s.tempDir, "orag-video-*")
	if err != nil {
		return VideoSource{}, nil, err
	}
	path := file.Name()
	defer os.Remove(path)
	h := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, h), io.LimitReader(body, source.Bytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written != source.Bytes || hex.EncodeToString(h.Sum(nil)) != source.SHA256 {
		return VideoSource{}, nil, ErrVideoSourceInvalid
	}
	segments, err := BuildTemporalSegments(source, *experiment.PackManifest.VideoProtocol)
	if err != nil {
		return VideoSource{}, nil, err
	}
	object := VerifiedObject{PackObject: PackObject{Path: "video/" + source.Alias, SHA256: source.SHA256, Bytes: source.Bytes, ContentType: source.ContentType}, TempPath: path}
	if err := s.private.PutVerified(ctx, PrivateObject{TenantID: experiment.TenantID, ProjectID: experiment.ProjectID, JobID: experiment.CloneJobID, Object: object}); err != nil {
		return VideoSource{}, nil, err
	}
	temporalObject, temporalPath, err := WriteTemporalIndex(s.tempDir, segments)
	if err != nil {
		return VideoSource{}, nil, err
	}
	defer os.Remove(temporalPath)
	if err := s.private.PutVerified(ctx, PrivateObject{TenantID: experiment.TenantID, ProjectID: experiment.ProjectID, JobID: experiment.CloneJobID, Object: VerifiedObject{PackObject: temporalObject, TempPath: temporalPath}}); err != nil {
		return VideoSource{}, nil, err
	}
	manifest := cloneManifest(experiment.PackManifest)
	manifest.VideoSource = &source
	manifest.TemporalSegments = segments
	manifest.TemporalAssets = []PackObject{temporalObject}
	// A video import makes deterministic retrieval input available, but never
	// fabricates benchmark questions or answers. Evaluation becomes runnable only
	// after the owner supplies an authorized private evaluation set.
	resources := RuntimeResources{Status: "temporal_index_pending_evaluation", KnowledgeBaseID: experiment.KnowledgeBaseID, DatasetID: experiment.DatasetID, BaselineProfile: experiment.PackManifest.VideoProtocol.Runtime.Profile, BaselineTopK: experiment.PackManifest.VideoProtocol.Runtime.TopK}
	if err := s.repo.SetExperimentRuntime(ctx, experiment.TenantID, experiment.ProjectID, resources, manifest, experiment.UpdatedAt); err != nil {
		return VideoSource{}, nil, err
	}
	return source, segments, nil
}
