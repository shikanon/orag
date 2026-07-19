package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/pkg/buildinfo"
)

// DatasetSnapshot freezes the exact cases used by an evaluation run. Dataset
// roots may remain editable; historical runs always reference this content.
type DatasetSnapshot struct {
	DatasetID   string               `json:"dataset_id"`
	Version     string               `json:"version,omitempty"`
	Split       dataset.DatasetSplit `json:"split,omitempty"`
	Items       []dataset.Item       `json:"items,omitempty"`
	ContentHash string               `json:"content_hash"`
	ItemCount   int                  `json:"item_count"`
}

// EvaluationManifest records the server-effective inputs needed to explain a
// run. It deliberately contains only durable identifiers and configuration,
// never credentials or raw model provider responses.
type EvaluationManifest struct {
	SchemaVersion      string          `json:"schema_version"`
	CodeVersion        string          `json:"code_version,omitempty"`
	CodeCommit         string          `json:"code_commit,omitempty"`
	Dataset            DatasetSnapshot `json:"dataset"`
	KnowledgeBaseID    string          `json:"knowledge_base_id"`
	ProjectID          string          `json:"project_id,omitempty"`
	Profile            rag.Profile     `json:"profile,omitempty"`
	TopK               int             `json:"top_k,omitempty"`
	ScopedShadowItemID string          `json:"scoped_shadow_item_id,omitempty"`
	JudgeConfigHash    string          `json:"judge_config_hash,omitempty"`
	QAGConfigHash      string          `json:"qag_config_hash,omitempty"`
	PairwiseConfigHash string          `json:"pairwise_config_hash,omitempty"`
}

func snapshotDataset(ds dataset.Dataset, split dataset.DatasetSplit, items []dataset.Item) DatasetSnapshot {
	copyItems := append([]dataset.Item(nil), items...)
	sort.Slice(copyItems, func(i, j int) bool { return copyItems[i].ID < copyItems[j].ID })
	for i := range copyItems {
		copyItems[i] = dataset.NormalizeItemMetadata(copyItems[i])
	}
	return DatasetSnapshot{
		DatasetID:   ds.ID,
		Version:     ds.Version,
		Split:       split,
		Items:       copyItems,
		ContentHash: stableJSONHash(copyItems),
		ItemCount:   len(copyItems),
	}
}

func evaluationManifest(ds dataset.Dataset, snapshot DatasetSnapshot, req RunRequest) EvaluationManifest {
	build := buildinfo.Current()
	manifest := EvaluationManifest{
		SchemaVersion:      "1.0",
		CodeVersion:        build.Version,
		CodeCommit:         build.Commit,
		Dataset:            snapshot,
		KnowledgeBaseID:    strings.TrimSpace(req.KnowledgeBaseID),
		ProjectID:          strings.TrimSpace(req.ProjectID),
		Profile:            req.Profile,
		TopK:               req.TopK,
		ScopedShadowItemID: strings.TrimSpace(req.ScopedShadowItemID),
	}
	if req.Judge != nil {
		manifest.JudgeConfigHash = HashJudgeConfig(*req.Judge)
	}
	if req.QAG != nil {
		manifest.QAGConfigHash = HashJudgeConfig(*req.QAG)
	}
	if req.Pairwise != nil {
		manifest.PairwiseConfigHash = HashJudgeConfig(*req.Pairwise)
	}
	return manifest
}

func stableJSONHash(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(digest[:])
}
