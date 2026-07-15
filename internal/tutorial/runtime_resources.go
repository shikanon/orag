package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/kb"
)

// ResourceInitializer creates only the durable, project-owned roots required
// by a later Live Run. It does not invoke a model or read Pack content; this
// keeps Pack installation usable when a provider is not configured.
type ResourceInitializer struct {
	KnowledgeBases kb.KnowledgeBaseRepository
	Datasets       *dataset.Service
	Now            func() time.Time
}

func (r ResourceInitializer) Ensure(ctx context.Context, job CloneJob, manifest Manifest) (RuntimeResources, error) {
	if manifest.Runtime == nil {
		return RuntimeResources{Status: "runtime_unavailable"}, nil
	}
	if r.KnowledgeBases == nil || r.Datasets == nil {
		return RuntimeResources{}, errors.New("tutorial runtime resource services are unavailable")
	}
	now := r.now()
	knowledgeBaseID := tutorialResourceID("tkb", job.ProjectID, job.TemplateID, job.TemplateVersion)
	if _, found, err := r.KnowledgeBases.GetKnowledgeBase(ctx, job.TenantID, knowledgeBaseID); err != nil {
		return RuntimeResources{}, err
	} else if !found {
		item := kb.KnowledgeBase{
			ID: knowledgeBaseID, TenantID: job.TenantID, ProjectID: job.ProjectID,
			Name: "教程基线知识库", Description: "由已校验教程 Pack 创建的只读运行根。",
			Metadata:  map[string]string{"tutorial_template_id": job.TemplateID, "tutorial_template_version": job.TemplateVersion, "tutorial_pack_tier": job.Tier},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := r.KnowledgeBases.PutKnowledgeBase(ctx, item); err != nil {
			return RuntimeResources{}, err
		}
	}

	datasetID := tutorialResourceID("tds", job.ProjectID, job.TemplateID, job.TemplateVersion)
	if _, err := r.Datasets.EnsureInProject(ctx, job.TenantID, dataset.Dataset{
		ID: datasetID, TenantID: job.TenantID, ProjectID: job.ProjectID,
		Name: manifest.Runtime.Dataset.Name, Kind: "tutorial_baseline", Version: job.TemplateVersion, CreatedAt: now,
	}); err != nil {
		return RuntimeResources{}, err
	}
	for index, item := range manifest.Runtime.Dataset.Items {
		if _, err := r.Datasets.EnsureItem(ctx, job.TenantID, dataset.Item{
			ID: tutorialResourceID("tdi", datasetID, fmt.Sprintf("%d", index), item.Query, item.GroundTruth), DatasetID: datasetID,
			Query: item.Query, GroundTruth: item.GroundTruth, ExpectedEvidence: item.ExpectedEvidence,
			Split: dataset.DatasetSplit(strings.TrimSpace(item.Split)), Weight: item.Weight,
		}); err != nil {
			return RuntimeResources{}, err
		}
	}
	return RuntimeResources{
		Status: "ready", KnowledgeBaseID: knowledgeBaseID, DatasetID: datasetID,
		BaselineProfile: manifest.Runtime.Baseline.Profile, BaselineTopK: manifest.Runtime.Baseline.TopK,
	}, nil
}

func (r ResourceInitializer) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func tutorialResourceID(prefix string, values ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "_" + hex.EncodeToString(sum[:])[:24]
}
