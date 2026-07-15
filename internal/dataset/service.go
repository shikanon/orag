package dataset

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
)

var ErrDatasetNotFound = errors.New("dataset not found")

type Dataset struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	ProjectID string    `json:"project_id,omitempty"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}

type Item struct {
	ID                   string                `json:"id"`
	DatasetID            string                `json:"dataset_id"`
	Query                string                `json:"query"`
	GroundTruth          string                `json:"ground_truth"`
	RelevantDocIDs       []string              `json:"relevant_doc_ids"`
	DiversityAnnotations []DiversityAnnotation `json:"diversity_annotations,omitempty"`
	Split                DatasetSplit          `json:"split,omitempty"`
	Weight               float64               `json:"weight,omitempty"`
	ExpectedEvidence     []string              `json:"expected_evidence,omitempty"`
	HumanScores          map[string]float64    `json:"human_scores,omitempty"`
}

type DatasetSplit string

const (
	DatasetSplitTrain   DatasetSplit = "train"
	DatasetSplitEval    DatasetSplit = "eval"
	DatasetSplitHoldout DatasetSplit = "holdout"
	DatasetSplitGold    DatasetSplit = "gold"
)

type DiversityAnnotation struct {
	Aspect      string   `json:"aspect,omitempty"`
	Subquestion string   `json:"subquestion,omitempty"`
	ChunkID     string   `json:"chunk_id,omitempty"`
	ChunkIDs    []string `json:"chunk_ids,omitempty"`
	DocumentID  string   `json:"document_id,omitempty"`
	DocumentIDs []string `json:"document_ids,omitempty"`
	SourceURI   string   `json:"source_uri,omitempty"`
	SourceURIs  []string `json:"source_uris,omitempty"`
}

type Repository interface {
	CreateDataset(ctx context.Context, ds Dataset) (Dataset, error)
	GetDataset(ctx context.Context, tenantID, id string) (Dataset, bool, error)
	AddDatasetItem(ctx context.Context, tenantID string, item Item) (Item, error)
	DatasetItems(ctx context.Context, tenantID, datasetID string) ([]Item, error)
}

type SplitRepository interface {
	DatasetItemsBySplit(ctx context.Context, tenantID, datasetID string, split DatasetSplit) ([]Item, error)
}

type Service struct {
	repo Repository
}

func NewService(repo ...Repository) *Service {
	if len(repo) > 0 && repo[0] != nil {
		return &Service{repo: repo[0]}
	}
	return &Service{repo: NewMemoryRepository()}
}

func (s *Service) Create(ctx context.Context, tenantID, name, kind string) (Dataset, error) {
	return s.CreateInProject(ctx, tenantID, "", name, kind)
}

func (s *Service) CreateInProject(ctx context.Context, tenantID, projectID, name, kind string) (Dataset, error) {
	ds := Dataset{
		ID:        id.New("ds"),
		TenantID:  tenantID,
		ProjectID: projectID,
		Name:      name,
		Kind:      kind,
		Version:   time.Now().UTC().Format("20060102150405"),
		CreatedAt: time.Now().UTC(),
	}
	return s.repo.CreateDataset(ctx, ds)
}

func (s *Service) AddItem(ctx context.Context, tenantID, datasetID string, item Item) (Item, error) {
	if _, ok, err := s.repo.GetDataset(ctx, tenantID, datasetID); err != nil {
		return Item{}, err
	} else if !ok {
		return Item{}, ErrDatasetNotFound
	}
	item.ID = id.New("dsi")
	item.DatasetID = datasetID
	item = NormalizeItemMetadata(item)
	return s.repo.AddDatasetItem(ctx, tenantID, item)
}

func (s *Service) Items(ctx context.Context, tenantID, datasetID string) ([]Item, error) {
	if _, ok, err := s.repo.GetDataset(ctx, tenantID, datasetID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrDatasetNotFound
	}
	return s.repo.DatasetItems(ctx, tenantID, datasetID)
}

func (s *Service) ItemsBySplit(ctx context.Context, tenantID, datasetID string, split DatasetSplit) ([]Item, error) {
	if _, ok, err := s.repo.GetDataset(ctx, tenantID, datasetID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrDatasetNotFound
	}
	split = NormalizeSplit(split)
	if split == "" {
		return s.repo.DatasetItems(ctx, tenantID, datasetID)
	}
	if repo, ok := s.repo.(SplitRepository); ok {
		return repo.DatasetItemsBySplit(ctx, tenantID, datasetID, split)
	}
	items, err := s.repo.DatasetItems(ctx, tenantID, datasetID)
	if err != nil {
		return nil, err
	}
	return FilterItemsBySplit(items, split), nil
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (Dataset, bool, error) {
	return s.repo.GetDataset(ctx, tenantID, id)
}

type MemoryRepository struct {
	mu       sync.RWMutex
	datasets map[string]Dataset
	items    map[string][]Item
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{datasets: map[string]Dataset{}, items: map[string][]Item{}}
}

func (r *MemoryRepository) CreateDataset(_ context.Context, ds Dataset) (Dataset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.datasets[ds.ID] = ds
	return ds, nil
}

func (r *MemoryRepository) GetDataset(_ context.Context, tenantID, id string) (Dataset, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ds, ok := r.datasets[id]
	return ds, ok && ds.TenantID == tenantID, nil
}

func (r *MemoryRepository) AddDatasetItem(_ context.Context, tenantID string, item Item) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ds, ok := r.datasets[item.DatasetID]
	if !ok || ds.TenantID != tenantID {
		return Item{}, ErrDatasetNotFound
	}
	item = NormalizeItemMetadata(item)
	r.items[item.DatasetID] = append(r.items[item.DatasetID], item)
	return item, nil
}

func (r *MemoryRepository) DatasetItems(_ context.Context, tenantID, datasetID string) ([]Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ds, ok := r.datasets[datasetID]
	if !ok || ds.TenantID != tenantID {
		return nil, ErrDatasetNotFound
	}
	return append([]Item(nil), r.items[datasetID]...), nil
}

func (r *MemoryRepository) DatasetItemsBySplit(_ context.Context, tenantID, datasetID string, split DatasetSplit) ([]Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ds, ok := r.datasets[datasetID]
	if !ok || ds.TenantID != tenantID {
		return nil, ErrDatasetNotFound
	}
	return FilterItemsBySplit(r.items[datasetID], split), nil
}

func NormalizeItemMetadata(item Item) Item {
	if item.Split == "" {
		item.Split = DatasetSplitEval
	}
	if item.Weight <= 0 {
		item.Weight = 1
	}
	if item.ExpectedEvidence == nil {
		item.ExpectedEvidence = []string{}
	}
	if item.HumanScores == nil {
		item.HumanScores = map[string]float64{}
	}
	return item
}

func NormalizeSplit(split DatasetSplit) DatasetSplit {
	return DatasetSplit(strings.TrimSpace(string(split)))
}

func FilterItemsBySplit(items []Item, split DatasetSplit) []Item {
	split = NormalizeSplit(split)
	if split == "" {
		return append([]Item(nil), items...)
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		item = NormalizeItemMetadata(item)
		if item.Split == split {
			out = append(out, item)
		}
	}
	return out
}
