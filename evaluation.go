package orag

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/rag"
)

// DatasetSplit identifies the intended use of an evaluation item.
type DatasetSplit string

const (
	DatasetSplitTrain   DatasetSplit = "train"
	DatasetSplitEval    DatasetSplit = "eval"
	DatasetSplitHoldout DatasetSplit = "holdout"
	DatasetSplitGold    DatasetSplit = "gold"
)

// Dataset is a versioned collection of evaluation examples.
type Dataset struct {
	ID        string
	TenantID  string
	ProjectID string
	Name      string
	Kind      string
	Version   string
	CreatedAt time.Time
}

type DatasetItem struct {
	ID               string
	DatasetID        string
	Query            string
	GroundTruth      string
	RelevantDocIDs   []string
	Split            DatasetSplit
	Weight           float64
	ExpectedEvidence []string
	HumanScores      map[string]float64
}

type CreateDatasetRequest struct {
	TenantID  string
	ProjectID string
	Name      string
	Kind      string
}

type AddDatasetItemRequest struct {
	TenantID         string
	DatasetID        string
	Query            string
	GroundTruth      string
	RelevantDocIDs   []string
	Split            DatasetSplit
	Weight           float64
	ExpectedEvidence []string
	HumanScores      map[string]float64
}

type RunEvaluationRequest struct {
	TenantID        string
	DatasetID       string
	KnowledgeBaseID string
	Profile         string
	TopK            int
	Split           DatasetSplit
}

type GetEvaluationRequest struct {
	TenantID string
	ID       string
}

type EvaluationSplitSummary struct {
	UnweightedSampleCount int
	WeightedSampleCount   float64
}

// EvaluationRun contains deterministic evaluation metrics. Model-judge
// configuration remains an HTTP control-plane capability in the beta release.
type EvaluationRun struct {
	ID                    string
	DatasetID             string
	Profile               string
	Total                 int
	HitRate               float64
	Accuracy              float64
	WeightedSampleCount   float64
	UnweightedSampleCount int
	Split                 DatasetSplit
	SplitSummary          map[string]EvaluationSplitSummary
	MissingSplit          bool
	Metrics               map[string]float64
	CreatedAt             time.Time
}

func (c *Client) CreateDataset(ctx context.Context, req CreateDatasetRequest) (Dataset, error) {
	if err := c.requireOpen("create_dataset"); err != nil {
		return Dataset{}, err
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Kind) == "" {
		return Dataset{}, newError(CodeInvalidArgument, "create_dataset", "dataset", "", false, errors.New("name and kind are required"))
	}
	tenantID := c.tenant(req.TenantID)
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID != "" {
		if _, err := c.app.Projects.Get(ctx, tenantID, projectID); err != nil {
			return Dataset{}, controlPlaneError("create_dataset", projectID, err)
		}
	}
	item, err := c.app.Datasets.CreateInProject(ctx, tenantID, projectID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Kind))
	if err != nil {
		return Dataset{}, wrapError("create_dataset", "dataset", "", err)
	}
	return fromDataset(item), nil
}

func (c *Client) AddDatasetItem(ctx context.Context, req AddDatasetItemRequest) (DatasetItem, error) {
	if err := c.requireOpen("add_dataset_item"); err != nil {
		return DatasetItem{}, err
	}
	if strings.TrimSpace(req.DatasetID) == "" || strings.TrimSpace(req.Query) == "" {
		return DatasetItem{}, newError(CodeInvalidArgument, "add_dataset_item", req.DatasetID, "", false, errors.New("dataset_id and query are required"))
	}
	item, err := c.app.Datasets.AddItem(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.DatasetID), dataset.Item{
		Query:            strings.TrimSpace(req.Query),
		GroundTruth:      req.GroundTruth,
		RelevantDocIDs:   append([]string(nil), req.RelevantDocIDs...),
		Split:            dataset.DatasetSplit(req.Split),
		Weight:           req.Weight,
		ExpectedEvidence: append([]string(nil), req.ExpectedEvidence...),
		HumanScores:      cloneFloats(req.HumanScores),
	})
	if err != nil {
		if errors.Is(err, dataset.ErrDatasetNotFound) {
			return DatasetItem{}, newError(CodeNotFound, "add_dataset_item", req.DatasetID, "", false, err)
		}
		return DatasetItem{}, wrapError("add_dataset_item", req.DatasetID, "", err)
	}
	return fromDatasetItem(item), nil
}

func (c *Client) RunEvaluation(ctx context.Context, req RunEvaluationRequest) (EvaluationRun, error) {
	if err := c.requireOpen("run_evaluation"); err != nil {
		return EvaluationRun{}, err
	}
	if strings.TrimSpace(req.DatasetID) == "" || strings.TrimSpace(req.KnowledgeBaseID) == "" {
		return EvaluationRun{}, newError(CodeInvalidArgument, "run_evaluation", req.DatasetID, "", false, errors.New("dataset_id and knowledge_base_id are required"))
	}
	result, err := c.app.Eval.Run(ctx, eval.RunRequest{
		TenantID:        c.tenant(req.TenantID),
		DatasetID:       strings.TrimSpace(req.DatasetID),
		KnowledgeBaseID: strings.TrimSpace(req.KnowledgeBaseID),
		Profile:         rag.Profile(req.Profile),
		TopK:            req.TopK,
		Split:           dataset.DatasetSplit(req.Split),
	})
	if err != nil {
		return EvaluationRun{}, wrapError("run_evaluation", req.DatasetID, "", err)
	}
	return fromEvaluationRun(result), nil
}

func (c *Client) GetEvaluation(ctx context.Context, req GetEvaluationRequest) (EvaluationRun, bool, error) {
	if err := c.requireOpen("get_evaluation"); err != nil {
		return EvaluationRun{}, false, err
	}
	if strings.TrimSpace(req.ID) == "" {
		return EvaluationRun{}, false, newError(CodeInvalidArgument, "get_evaluation", req.ID, "", false, errors.New("id is required"))
	}
	result, found, err := c.app.Eval.Get(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return EvaluationRun{}, false, wrapError("get_evaluation", req.ID, "", err)
	}
	if !found {
		return EvaluationRun{}, false, nil
	}
	return fromEvaluationRun(result), true, nil
}

func fromDataset(item dataset.Dataset) Dataset {
	return Dataset{ID: item.ID, TenantID: item.TenantID, ProjectID: item.ProjectID, Name: item.Name, Kind: item.Kind, Version: item.Version, CreatedAt: item.CreatedAt}
}

func fromDatasetItem(item dataset.Item) DatasetItem {
	return DatasetItem{ID: item.ID, DatasetID: item.DatasetID, Query: item.Query, GroundTruth: item.GroundTruth, RelevantDocIDs: append([]string(nil), item.RelevantDocIDs...), Split: DatasetSplit(item.Split), Weight: item.Weight, ExpectedEvidence: append([]string(nil), item.ExpectedEvidence...), HumanScores: cloneFloats(item.HumanScores)}
}

func fromEvaluationRun(item eval.RunResult) EvaluationRun {
	summary := make(map[string]EvaluationSplitSummary, len(item.SplitSummary))
	for name, value := range item.SplitSummary {
		summary[name] = EvaluationSplitSummary{UnweightedSampleCount: value.UnweightedSampleCount, WeightedSampleCount: value.WeightedSampleCount}
	}
	return EvaluationRun{ID: item.ID, DatasetID: item.DatasetID, Profile: item.Profile, Total: item.Total, HitRate: item.HitRate, Accuracy: item.Accuracy, WeightedSampleCount: item.WeightedSampleCount, UnweightedSampleCount: item.UnweightedSampleCount, Split: DatasetSplit(item.Split), SplitSummary: summary, MissingSplit: item.MissingSplit, Metrics: cloneFloats(item.Metrics), CreatedAt: item.CreatedAt}
}

func cloneFloats(values map[string]float64) map[string]float64 {
	if values == nil {
		return nil
	}
	result := make(map[string]float64, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
