package dataset

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
)

var ErrDatasetNotFound = errors.New("dataset not found")

type Dataset struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}

type Item struct {
	ID             string   `json:"id"`
	DatasetID      string   `json:"dataset_id"`
	Query          string   `json:"query"`
	GroundTruth    string   `json:"ground_truth"`
	RelevantDocIDs []string `json:"relevant_doc_ids"`
}

type Repository interface {
	CreateDataset(ctx context.Context, ds Dataset) (Dataset, error)
	GetDataset(ctx context.Context, tenantID, id string) (Dataset, bool, error)
	AddDatasetItem(ctx context.Context, tenantID string, item Item) (Item, error)
	DatasetItems(ctx context.Context, tenantID, datasetID string) ([]Item, error)
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
	ds := Dataset{
		ID:        id.New("ds"),
		TenantID:  tenantID,
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
	if ds, ok := r.datasets[item.DatasetID]; !ok || ds.TenantID != tenantID {
		return Item{}, ErrDatasetNotFound
	}
	r.items[item.DatasetID] = append(r.items[item.DatasetID], item)
	return item, nil
}

func (r *MemoryRepository) DatasetItems(_ context.Context, tenantID, datasetID string) ([]Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ds, ok := r.datasets[datasetID]; !ok || ds.TenantID != tenantID {
		return nil, ErrDatasetNotFound
	}
	return append([]Item(nil), r.items[datasetID]...), nil
}
