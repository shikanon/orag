package optimizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/rag"
)

const runConfigRunnerKey = "run_config"

type RunConfig struct {
	DatasetID       string                 `json:"dataset_id,omitempty"`
	KnowledgeBaseID string                 `json:"knowledge_base_id,omitempty"`
	Objective       ObjectiveSpec          `json:"objective,omitempty"`
	SearchSpace     SearchSpace            `json:"search_space,omitempty"`
	Search          SearchSpec             `json:"search,omitempty"`
	Budget          Budget                 `json:"budget,omitempty"`
	Profile         rag.Profile            `json:"profile,omitempty"`
	TopK            int                    `json:"top_k,omitempty"`
	NamespaceTTL    time.Duration          `json:"namespace_ttl,omitempty"`
	SelectionSplit  string                 `json:"selection_split,omitempty"`
	HoldoutSplit    string                 `json:"holdout_split,omitempty"`
	HoldoutGate     eval.HoldoutGateConfig `json:"holdout_gate,omitempty"`
}

func RunConfigFromSubmitRequest(req SubmitRequest) RunConfig {
	return RunConfig{
		DatasetID:       req.DatasetID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Objective:       req.Objective,
		SearchSpace:     req.SearchSpace,
		Search:          req.Search,
		Budget:          req.Budget,
		Profile:         req.Profile,
		TopK:            req.TopK,
		NamespaceTTL:    req.NamespaceTTL,
		SelectionSplit:  req.SelectionSplit,
		HoldoutSplit:    req.HoldoutSplit,
		HoldoutGate:     req.HoldoutGate,
	}
}

func (cfg RunConfig) SubmitRequest(tenantID string, runner map[string]any) SubmitRequest {
	return SubmitRequest{
		TenantID:        tenantID,
		DatasetID:       cfg.DatasetID,
		KnowledgeBaseID: cfg.KnowledgeBaseID,
		Objective:       cfg.Objective,
		SearchSpace:     cfg.SearchSpace,
		Search:          cfg.Search,
		Budget:          cfg.Budget,
		Profile:         cfg.Profile,
		TopK:            cfg.TopK,
		NamespaceTTL:    cfg.NamespaceTTL,
		SelectionSplit:  cfg.SelectionSplit,
		HoldoutSplit:    cfg.HoldoutSplit,
		HoldoutGate:     cfg.HoldoutGate,
		Runner:          cloneRunner(runner),
	}
}

func (cfg RunConfig) IsZero() bool {
	return reflect.DeepEqual(cfg, RunConfig{})
}

func (run OptimizationRun) StoredSubmitRequest() SubmitRequest {
	cfg := run.storedConfig()
	return cfg.SubmitRequest(run.TenantID, run.Runner)
}

func (run OptimizationRun) RunnerWithConfig() map[string]any {
	runner := cloneRunner(run.Runner)
	cfg := run.storedConfig()
	if !cfg.IsZero() {
		runner[runConfigRunnerKey] = cfg
	}
	return runner
}

func (run *OptimizationRun) LoadConfigFromRunner() error {
	if run.Runner == nil {
		return nil
	}
	raw, ok := run.Runner[runConfigRunnerKey]
	if !ok {
		return nil
	}
	cfg, err := decodeRunConfig(raw)
	if err != nil {
		return err
	}
	run.Config = cfg
	delete(run.Runner, runConfigRunnerKey)
	return nil
}

func (run OptimizationRun) storedConfig() RunConfig {
	cfg := run.Config
	if cfg.IsZero() {
		cfg = RunConfig{
			DatasetID:       run.DatasetID,
			KnowledgeBaseID: run.KnowledgeBaseID,
			Objective:       run.Objective,
			SearchSpace:     run.SearchSpace,
		}
		if profile, ok := stringFromRunner(run.Runner, "profile"); ok {
			cfg.Profile = rag.Profile(profile)
		}
		if topK, ok := intFromRunner(run.Runner, "top_k"); ok {
			cfg.TopK = topK
		}
		return cfg
	}
	if cfg.DatasetID == "" {
		cfg.DatasetID = run.DatasetID
	}
	if cfg.KnowledgeBaseID == "" {
		cfg.KnowledgeBaseID = run.KnowledgeBaseID
	}
	return cfg
}

func decodeRunConfig(raw any) (RunConfig, error) {
	if cfg, ok := raw.(RunConfig); ok {
		return cfg, nil
	}
	var payload []byte
	switch v := raw.(type) {
	case []byte:
		payload = v
	case json.RawMessage:
		payload = v
	default:
		var err error
		payload, err = json.Marshal(v)
		if err != nil {
			return RunConfig{}, fmt.Errorf("marshal optimization run config: %w", err)
		}
	}
	var cfg RunConfig
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return RunConfig{}, fmt.Errorf("decode optimization run config: %w", err)
	}
	return cfg, nil
}

func cloneRunner(runner map[string]any) map[string]any {
	if len(runner) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(runner))
	for key, value := range runner {
		if key == runConfigRunnerKey {
			continue
		}
		out[key] = value
	}
	return out
}

func stringFromRunner(runner map[string]any, key string) (string, bool) {
	value, ok := runner[key]
	if !ok {
		return "", false
	}
	switch v := value.(type) {
	case string:
		return v, true
	default:
		return "", false
	}
}

func intFromRunner(runner map[string]any, key string) (int, bool) {
	value, ok := runner[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}
