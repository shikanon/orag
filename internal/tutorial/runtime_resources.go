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
	if err := r.ensureKnowledgeBase(ctx, job, knowledgeBaseID, "教程基线知识库", "由已校验教程 Pack 创建的只读运行根。", map[string]string{
		"tutorial_template_id": job.TemplateID, "tutorial_template_version": job.TemplateVersion, "tutorial_pack_tier": job.Tier,
	}, now); err != nil {
		return RuntimeResources{}, err
	}
	for _, candidate := range manifest.Runtime.Candidates {
		candidateID := tutorialCandidateKnowledgeBaseID(job, candidate.ID)
		if err := r.ensureKnowledgeBase(ctx, job, candidateID, "教程 P1 解析候选知识库", "由已校验教程 Pack 创建的独立 P1 解析候选运行根。", map[string]string{
			"tutorial_template_id": job.TemplateID, "tutorial_template_version": job.TemplateVersion, "tutorial_pack_tier": job.Tier,
			"tutorial_variant": candidate.ID, "tutorial_parser_method": candidate.ParserMethod,
		}, now); err != nil {
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

func (r ResourceInitializer) ensureKnowledgeBase(ctx context.Context, job CloneJob, knowledgeBaseID, name, description string, metadata map[string]string, now time.Time) error {
	if _, found, err := r.KnowledgeBases.GetKnowledgeBase(ctx, job.TenantID, knowledgeBaseID); err != nil {
		return err
	} else if found {
		return nil
	}
	return r.KnowledgeBases.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID: knowledgeBaseID, TenantID: job.TenantID, ProjectID: job.ProjectID,
		Name: name, Description: description, Metadata: metadata, CreatedAt: now, UpdatedAt: now,
	})
}

func tutorialCandidateKnowledgeBaseID(job CloneJob, candidateID string) string {
	return tutorialCandidateKnowledgeBaseIDFor(job.ProjectID, job.TemplateID, job.TemplateVersion, candidateID)
}

func tutorialCandidateKnowledgeBaseIDFor(projectID, templateID, templateVersion, candidateID string) string {
	return tutorialResourceID("tkb", projectID, templateID, templateVersion, candidateID)
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
