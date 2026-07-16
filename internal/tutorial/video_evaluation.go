package tutorial

import (
	"context"
	"errors"
	"strings"

	"github.com/shikanon/orag/internal/dataset"
)

var ErrVideoEvaluationInvalid = errors.New("private video evaluation set is invalid")

// VideoEvaluationService freezes an owner-owned project dataset into the
// experiment manifest. The copied set makes a later P0 independent from
// future edits to the mutable source dataset.
type VideoEvaluationService struct {
	repo        CloneRepository
	datasets    *dataset.Service
	initializer RuntimeInitializer
}

func NewVideoEvaluationService(repo CloneRepository, datasets *dataset.Service, initializer RuntimeInitializer) *VideoEvaluationService {
	return &VideoEvaluationService{repo: repo, datasets: datasets, initializer: initializer}
}

func (s *VideoEvaluationService) Activate(ctx context.Context, subject Subject, projectID, sourceDatasetID string) (RuntimeResources, error) {
	if s == nil || s.repo == nil || s.datasets == nil || s.initializer == nil {
		return RuntimeResources{}, ErrVideoEvaluationInvalid
	}
	experiment, found, err := s.repo.GetExperiment(ctx, subject.TenantID, projectID)
	if err != nil || !found || experiment.TemplateID != "video-rag" || experiment.PackStatus != PackStatusInstalled || experiment.PackManifest.VideoProtocol == nil || experiment.PackManifest.VideoSource == nil || len(experiment.PackManifest.TemporalAssets) == 0 {
		return RuntimeResources{}, ErrVideoEvaluationInvalid
	}
	source, found, err := s.datasets.GetInProject(ctx, subject.TenantID, projectID, strings.TrimSpace(sourceDatasetID))
	if err != nil || !found {
		return RuntimeResources{}, ErrVideoEvaluationInvalid
	}
	items, err := s.datasets.Items(ctx, subject.TenantID, source.ID)
	if err != nil || len(items) == 0 {
		return RuntimeResources{}, ErrVideoEvaluationInvalid
	}
	evidence := make(map[string]bool, len(experiment.PackManifest.TemporalSegments))
	for _, segment := range experiment.PackManifest.TemporalSegments {
		evidence[segment.EvidenceID] = true
	}
	runtimeItems := make([]RuntimeDatasetItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Query) == "" || strings.TrimSpace(item.GroundTruth) == "" || len(item.ExpectedEvidence) == 0 {
			return RuntimeResources{}, ErrVideoEvaluationInvalid
		}
		for _, id := range item.ExpectedEvidence {
			if !evidence[id] {
				return RuntimeResources{}, ErrVideoEvaluationInvalid
			}
		}
		runtimeItems = append(runtimeItems, RuntimeDatasetItem{Query: item.Query, GroundTruth: item.GroundTruth, ExpectedEvidence: item.ExpectedEvidence, Split: string(item.Split), Weight: item.Weight})
	}
	manifest := cloneManifest(experiment.PackManifest)
	manifest.Runtime = &RuntimeManifest{Baseline: RuntimeBaseline{Profile: experiment.PackManifest.VideoProtocol.Runtime.Profile, TopK: experiment.PackManifest.VideoProtocol.Runtime.TopK}, Documents: []RuntimeDocument{{ObjectPath: manifest.TemporalAssets[0].Path, Name: "temporal evidence"}}, Dataset: RuntimeDataset{Name: "Private Video RAG evaluation snapshot", Items: runtimeItems}}
	job, found, err := s.repo.GetJob(ctx, subject.TenantID, experiment.CloneJobID)
	if err != nil || !found {
		return RuntimeResources{}, ErrVideoEvaluationInvalid
	}
	resources, err := s.initializer.Ensure(ctx, job, manifest)
	if err != nil {
		return RuntimeResources{}, err
	}
	if err := s.repo.SetExperimentRuntime(ctx, subject.TenantID, projectID, resources, manifest, experiment.UpdatedAt); err != nil {
		return RuntimeResources{}, err
	}
	return resources, nil
}
