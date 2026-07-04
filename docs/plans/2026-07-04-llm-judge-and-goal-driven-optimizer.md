# LLM Judge And Goal Driven Optimizer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a complete LLM-as-Judge evaluation layer and a goal-driven optimizer that can tune prompts, chunking, retrieval, reranking, indexing, model choices, and module orchestration using internal LLM runners or external harness tools.

**Architecture:** Extend the current rule-based evaluation runner with pluggable judges, metric schemas, objective functions, experiment snapshots, candidate generators, and runner backends. Keep deterministic metrics as the CI-safe baseline, add optional LLM-as-Judge as an enhancement, and persist every candidate, score, prompt, config snapshot, trace, and harness artifact for reproducibility.

**Tech Stack:** Go, Hertz, Eino Graph, PostgreSQL JSONB, Qdrant, Ark/Doubao-compatible LLM clients, external process harness adapters, OpenAPI, contract tests.

---

## Current Project Status After Merge

Merge status on 2026-07-04:

- The LLM-as-Judge and goal-driven optimizer design has been implemented and merged into the main project line.
- This document now serves as the implementation design record and capability reference, not as a list of missing features.

Current evaluation implementation:

- `internal/eval.Runner` runs the existing production RAG query path over all dataset items.
- Evaluation input supports `dataset_id`, `knowledge_base_id`, `profile`, `top_k`, optional `judge`, and optional `qag` configuration.
- The runner validates `dataset_id` and `knowledge_base_id`, verifies dataset existence under the current tenant, and rejects empty datasets before running RAG queries.
- The runner keeps deterministic metrics as the default baseline and, when requested, executes LLM-as-Judge, QAG Score, pairwise judge, judge ensemble/repeat aggregation, and gold-set calibration support.
- Current run-level metrics include `answer_accuracy`, `accuracy`, `hit_rate`, `pairwise_accuracy`, `latency_p95_ms`, `cache_hit_rate`, aggregated retrieval/diversity metrics, and optional Judge/QAG token/cost metrics.
- Current item-level metrics include deterministic answer/retrieval/diversity metrics plus optional Judge/QAG results, findings, rationales, raw/parsed responses, pairwise details, and token/cost usage.
- `pairwise_accuracy` remains available as a deterministic fallback when pairwise judge is not requested; with pairwise judge enabled, it represents pairwise preference outcomes.

Current dataset implementation:

- Dataset items contain `query`, `ground_truth`, `relevant_doc_ids`, optional `diversity_annotations`, and evaluation metadata such as `split`, `weight`, `expected_evidence`, and `human_scores`.
- Dataset repository methods are tenant-aware: adding and listing items checks that the dataset belongs to the current tenant.
- `dataset.ErrDatasetNotFound` is used to distinguish missing datasets from internal failures.
- Dataset splits such as `train`, `eval`, `holdout`, and `gold` are supported for optimizer selection, holdout reporting, and judge calibration.

Current optimizer implementation:

- The goal-driven optimizer runs asynchronously through `POST /v1/optimizations`, persists run/candidate state, and supports polling, cancel, resume, checkpoint, budget controls, and holdout re-evaluation.
- Candidate scoring uses objective expressions, constraints, tie-breakers, fixed-budget normalization, pairwise win-rate, and bootstrap-style promotion checks.
- Search supports deterministic candidate IDs, grid, seeded random, successive halving plans, dependency-aware pruning, and internal RAG candidate overlays.
- External harness execution uses argv arrays only, executable allowlists, timeout/workdir isolation, redaction, and metric registry validation.
- Legacy `profiles × top_ks` requests remain accepted and are mapped into the newer search-space path.

Current persistence/API implementation:

- PostgreSQL persistence stores `evaluation_runs`, `evaluation_results`, `judge_runs`, `judge_results`, `pairwise_judge_results`, `judge_calibration_runs`, `optimization_runs`, `optimization_candidates`, and `harness_runs`.
- HTTP APIs expose dataset creation, tenant-checked dataset item append, evaluation run, evaluation detail query, asynchronous optimization submit/status/cancel/resume, and legacy optimization compatibility.
- `GET /v1/evaluations/{id}` defaults to run-level summary and supports `include_items`, `include_judge`, and `include_pairwise` for item-level and Judge/QAG details.

This plan records the architecture and migration path from the deterministic evaluation and simple optimizer baseline to the merged LLM-as-Judge and goal-driven optimizer system.

---

## Target Architecture

### Evaluation Layers

1. **Dataset layer**
   - Keep existing `datasets` and `dataset_items`.
   - Extend item metadata with richer expected evidence, rubric tags, difficulty, locale, and custom judge inputs.
   - Add optional per-item weights and metric overrides.
   - Add explicit dataset splits: `train`, `eval`, `holdout`, and optional `gold`.
   - Optimizer may use only `train` and `eval`; final winner must be re-evaluated on untouched `holdout`.
   - Maintain a small human-labeled `gold` set for judge calibration. A few dozen to one or two hundred examples is enough to track judge-human agreement.

2. **Execution layer**
   - Continue running the same RAG query path used by production.
   - Capture `answer`, `citations`, `retrieved_chunks`, `trace_id`, `latency_ms`, `cache_status`, warnings, and prompt/config snapshot.
   - Record token usage separately for RAG generation and judge calls. Candidate-level cost must aggregate prompt tokens, completion tokens, model prices, judge ensemble fan-out, pairwise order swaps, repeat count, and external harness cost when available.
   - Support two execution modes:
     - `internal_rag`: run current Go RAG service in-process.
     - `external_harness`: call tools such as `codex-cli`, scripts, benchmark harnesses, or remote runners with argv-array execution only. Shell templates and `${VAR}` interpolation are forbidden.
   - Redact secrets from env, argv, stdout, stderr, parsed metrics, and artifacts before persistence. External tools often echo config into stdout/stderr.

3. **Metric layer**
   - Preserve deterministic metrics: `accuracy`, `hit_rate`, `context_recall`, `citation_precision`, `latency_p95_ms`, `cache_hit_rate`.
   - Add evidence-grounded LLM-as-Judge metrics:
     - `faithfulness`
     - `groundedness`
     - `answer_relevance`
     - `hallucination`
     - `completeness`
     - `citation_support`
     - `qag_score`
     - `instruction_following`
     - `safety`
   - Prefer pairwise comparison for candidate ranking: ask judge whether answer A or B is better for the same query and evidence, instead of relying primarily on fragile absolute scores.
   - Add QAG Score for RAG faithfulness: generate verification questions from the answer, answer those questions from the source context, and check whether the generated answer is supported by the original evidence.
   - Use absolute scores only for evidence-checkable dimensions or coarse buckets such as `good`, `mixed`, `bad`; avoid treating `0.82` vs `0.85` as meaningful unless backed by repeated sampling and confidence intervals.
   - Normalize numeric summaries to `[0, 1]`, but persist the original pairwise decisions, coarse labels, rationales, confidence intervals, raw responses, model, rubric hash, prompt hash, and calibration metadata.
   - Compare judge metrics with deterministic anchor metrics. If judge quality improves while `citation_precision`, `context_recall`, or `hit_rate` collapse, flag the result as suspicious.
   - Metric names must be validated against a central whitelist before scoring, aggregation, objective evaluation, or persistence. Do not accept arbitrary `map[string]float64` keys from judge or harness outputs.

4. **Objective layer**
   - Let users define optimization goals as weighted formulas, constraints, and gates.
   - Support pairwise objectives such as win-rate, Bradley-Terry/Elo ranking score, or majority preference over a baseline candidate.
   - Support absolute metrics only as aggregates with confidence intervals, not single-run naked means.
   - Use a restricted expression engine for formulas. Allowed variables are whitelisted metric names only; allowed operators are `+`, `-`, `*`, `/`, and parentheses. Function calls, property access, indexing, assignment, and user-defined symbols are forbidden.
   - Normalize latency and cost against explicit objective constraints or configured budgets, not batch min-max. Example: `normalized_latency = min(latency_p95_ms / latency_p95_limit_ms, 1)`.
   - Example:
     ```json
     {
       "maximize": "0.35*groundedness + 0.25*answer_relevance + 0.2*completeness + 0.1*context_recall + 0.1*(1-normalized_latency)",
       "constraints": [
         "faithfulness >= 0.85",
         "hallucination <= 0.05",
         "latency_p95_ms <= 2500",
         "cost_usd <= 2.0"
       ],
       "tie_breakers": ["latency_p95_ms asc", "cost_usd asc", "created_at asc"]
     }
     ```

5. **Optimizer layer**
   - Generate candidate configurations from a user-provided search space.
   - Evaluate candidates using internal or external runners.
   - Score candidates with the objective function.
   - Support staged optimization:
     - coarse grid/random search
     - focused local search around top candidates
     - optional LLM-proposed candidate generation
     - optional external harness validation
   - Persist every optimization run and candidate for auditability.
   - Promote candidates only when they are significantly better than the baseline or incumbent using paired tests or bootstrap confidence intervals.
   - Always report final best candidate on holdout separately from train/eval selection scores.

---

## Core Concepts

### Evaluation Mode

```go
type EvaluationMode string

const (
    EvaluationModeRuleBased EvaluationMode = "rule_based"
    EvaluationModeLLMJudge  EvaluationMode = "llm_judge"
    EvaluationModeHybrid    EvaluationMode = "hybrid"
)
```

### Judge Metric

```go
type JudgeMetric string

const (
    JudgeMetricFaithfulness       JudgeMetric = "faithfulness"
    JudgeMetricGroundedness       JudgeMetric = "groundedness"
    JudgeMetricAnswerRelevance    JudgeMetric = "answer_relevance"
    JudgeMetricHallucination      JudgeMetric = "hallucination"
    JudgeMetricCompleteness       JudgeMetric = "completeness"
    JudgeMetricCitationSupport    JudgeMetric = "citation_support"
    JudgeMetricInstructionFollow  JudgeMetric = "instruction_following"
)
```

### Judge Input

```go
type JudgeInput struct {
    TenantID          string
    DatasetID         string
    DatasetItemID     string
    Query             string
    GroundTruth       string
    ExpectedEvidence  []string
    RelevantDocIDs    []string
    Answer            string
    Citations         []rag.Citation
    RetrievedChunks   []kb.SearchResult
    TraceID           string
    CandidateID       string
    Rubric            JudgeRubric
}
```

### Judge Output

```go
type JudgeOutput struct {
    Scores       map[string]float64
    Labels       map[string]string
    Confidence   map[string]ConfidenceInterval
    Pass         bool
    Rationale    string
    Findings     []JudgeFinding
    RawResponse   string
    JudgeModel    string
    PromptVersion string
    RubricHash    string
    CreatedAt     time.Time
}
```

### Pairwise Judge Output

```go
type PairwiseJudgeInput struct {
    TenantID         string
    DatasetID        string
    DatasetItemID    string
    Query            string
    GroundTruth      string
    ExpectedEvidence []string
    RelevantDocIDs   []string
    AnswerA          CandidateAnswer
    AnswerB          CandidateAnswer
    Rubric           JudgeRubric
}

type PairwiseJudgeOutput struct {
    Winner        string // "A", "B", or "tie"
    Preference    string // "A_much_better", "A_better", "tie", "B_better", "B_much_better"
    Reasons       []JudgeFinding
    RawResponse   string
    JudgeModel    string
    PromptVersion string
    RubricHash    string
    CreatedAt     time.Time
}
```

### Confidence Interval

```go
type ConfidenceInterval struct {
    Mean  float64
    Low   float64
    High  float64
    N     int
    Method string
}
```

### Objective

```go
type ObjectiveSpec struct {
    Name        string
    Direction   string
    Formula     string
    Weights     map[string]float64
    Pairwise    PairwiseObjective
    Constraints []ObjectiveConstraint
    TieBreakers []TieBreaker
}
```

### Dataset Split

```go
type DatasetSplit string

const (
    DatasetSplitTrain   DatasetSplit = "train"
    DatasetSplitEval    DatasetSplit = "eval"
    DatasetSplitHoldout DatasetSplit = "holdout"
    DatasetSplitGold    DatasetSplit = "gold"
)
```

### Candidate Config

```go
type CandidateConfig struct {
    Prompt          PromptCandidate
    Chunking        ChunkingCandidate
    Embedding       EmbeddingCandidate
    Reranker        RerankerCandidate
    Retrieval       RetrievalCandidate
    Indexing        IndexingCandidate
    Graph           GraphCandidate
    Harness         HarnessCandidate
}
```

---

## Database Design

### New Tables

```sql
ALTER TABLE dataset_items
    ADD COLUMN IF NOT EXISTS split TEXT NOT NULL DEFAULT 'eval',
    ADD COLUMN IF NOT EXISTS weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS expected_evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS human_scores JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS judge_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    evaluation_run_id TEXT NOT NULL REFERENCES evaluation_runs(id),
    judge_provider TEXT NOT NULL,
    judge_model TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    rubric_hash TEXT NOT NULL,
    prompt_hash TEXT NOT NULL,
    judge_params_hash TEXT NOT NULL,
    mode TEXT NOT NULL,
    comparison_mode TEXT NOT NULL DEFAULT 'absolute',
    rubric JSONB NOT NULL DEFAULT '{}'::jsonb,
    judge_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    ensemble JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS judge_results (
    id TEXT PRIMARY KEY,
    judge_run_id TEXT NOT NULL REFERENCES judge_runs(id),
    dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
    candidate_id TEXT NOT NULL DEFAULT '',
    scores JSONB NOT NULL DEFAULT '{}'::jsonb,
    pass BOOLEAN NOT NULL DEFAULT false,
    rationale TEXT NOT NULL DEFAULT '',
    findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    raw_response TEXT NOT NULL DEFAULT '',
    parsed_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    confidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pairwise_judge_results (
    id TEXT PRIMARY KEY,
    judge_run_id TEXT NOT NULL REFERENCES judge_runs(id),
    dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
    candidate_a_id TEXT NOT NULL,
    candidate_b_id TEXT NOT NULL,
    winner TEXT NOT NULL,
    preference TEXT NOT NULL,
    reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    raw_response TEXT NOT NULL DEFAULT '',
    parsed_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS judge_calibration_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    dataset_id TEXT NOT NULL REFERENCES datasets(id),
    judge_config_hash TEXT NOT NULL,
    human_score_version TEXT NOT NULL,
    spearman DOUBLE PRECISION NOT NULL DEFAULT 0,
    cohen_kappa DOUBLE PRECISION NOT NULL DEFAULT 0,
    sample_count INT NOT NULL DEFAULT 0,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS optimization_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    dataset_id TEXT NOT NULL REFERENCES datasets(id),
    knowledge_base_id TEXT NOT NULL REFERENCES knowledge_bases(id),
    objective JSONB NOT NULL DEFAULT '{}'::jsonb,
    search_space JSONB NOT NULL DEFAULT '{}'::jsonb,
    runner JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    status_reason TEXT NOT NULL DEFAULT '',
    best_candidate_id TEXT NOT NULL DEFAULT '',
    holdout_candidate_id TEXT NOT NULL DEFAULT '',
    sampling_strategy TEXT NOT NULL DEFAULT 'random',
    search_space_size BIGINT NOT NULL DEFAULT 0,
    sampled_candidate_count INT NOT NULL DEFAULT 0,
    completed_candidate_count INT NOT NULL DEFAULT 0,
    checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    cost_budget_usd DOUBLE PRECISION,
    cancel_requested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS optimization_candidates (
    id TEXT PRIMARY KEY,
    optimization_run_id TEXT NOT NULL REFERENCES optimization_runs(id),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    evaluation_run_id TEXT NOT NULL DEFAULT '',
    judge_run_id TEXT NOT NULL DEFAULT '',
    objective_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    holdout_score DOUBLE PRECISION,
    confidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    artifacts JSONB NOT NULL DEFAULT '{}'::jsonb,
    temp_namespaces JSONB NOT NULL DEFAULT '[]'::jsonb,
    cleanup_status TEXT NOT NULL DEFAULT 'not_required',
    expires_at TIMESTAMPTZ,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS harness_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    candidate_id TEXT NOT NULL,
    harness_type TEXT NOT NULL,
    argv JSONB NOT NULL DEFAULT '[]'::jsonb,
    working_dir TEXT NOT NULL DEFAULT '',
    env_redacted JSONB NOT NULL DEFAULT '{}'::jsonb,
    stdout_redacted TEXT NOT NULL DEFAULT '',
    stderr_redacted TEXT NOT NULL DEFAULT '',
    parsed_metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    exit_code INT NOT NULL DEFAULT 0,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    artifacts JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ
);
```

### Indexes

```sql
CREATE INDEX IF NOT EXISTS judge_runs_eval_idx ON judge_runs (tenant_id, evaluation_run_id);
CREATE INDEX IF NOT EXISTS judge_runs_hash_idx ON judge_runs (tenant_id, rubric_hash, prompt_hash, judge_params_hash);
CREATE INDEX IF NOT EXISTS judge_results_run_idx ON judge_results (judge_run_id);
CREATE INDEX IF NOT EXISTS pairwise_judge_results_run_idx ON pairwise_judge_results (judge_run_id);
CREATE INDEX IF NOT EXISTS judge_calibration_runs_hash_idx ON judge_calibration_runs (tenant_id, judge_config_hash, created_at DESC);
CREATE INDEX IF NOT EXISTS optimization_runs_tenant_status_idx ON optimization_runs (tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS optimization_candidates_run_score_idx ON optimization_candidates (optimization_run_id, objective_score DESC);
CREATE INDEX IF NOT EXISTS optimization_candidates_run_status_idx ON optimization_candidates (optimization_run_id, status, created_at);
CREATE INDEX IF NOT EXISTS optimization_candidates_cleanup_idx ON optimization_candidates (cleanup_status, expires_at);
CREATE INDEX IF NOT EXISTS harness_runs_candidate_idx ON harness_runs (candidate_id);
```

### Migration Rollback

Schema rollout must include both `up` and `down` migrations. Do not place all schema changes in an irreversible one-way file.

Required rollback order:

```sql
DROP INDEX IF EXISTS harness_runs_candidate_idx;
DROP INDEX IF EXISTS optimization_candidates_cleanup_idx;
DROP INDEX IF EXISTS optimization_candidates_run_status_idx;
DROP INDEX IF EXISTS optimization_candidates_run_score_idx;
DROP INDEX IF EXISTS optimization_runs_tenant_status_idx;
DROP INDEX IF EXISTS judge_calibration_runs_hash_idx;
DROP INDEX IF EXISTS pairwise_judge_results_run_idx;
DROP INDEX IF EXISTS judge_results_run_idx;
DROP INDEX IF EXISTS judge_runs_hash_idx;
DROP INDEX IF EXISTS judge_runs_eval_idx;

DROP TABLE IF EXISTS harness_runs;
DROP TABLE IF EXISTS optimization_candidates;
DROP TABLE IF EXISTS optimization_runs;
DROP TABLE IF EXISTS judge_calibration_runs;
DROP TABLE IF EXISTS pairwise_judge_results;
DROP TABLE IF EXISTS judge_results;
DROP TABLE IF EXISTS judge_runs;

ALTER TABLE dataset_items
    DROP COLUMN IF EXISTS human_scores,
    DROP COLUMN IF EXISTS expected_evidence,
    DROP COLUMN IF EXISTS weight,
    DROP COLUMN IF EXISTS split;
```

For safer deployment, split the schema into smaller migrations:

- `000006_dataset_eval_metadata`
- `000007_judge_results`
- `000008_optimizer_runs`
- `000009_harness_runs`

Each migration must have a matching down script and must be safe to run before behavior changes are enabled.

---

## API Design

### Run Evaluation With Judge

```http
POST /v1/evaluations
```

```json
{
  "dataset_id": "ds_xxx",
  "knowledge_base_id": "kb_xxx",
  "profile": "realtime",
  "top_k": 8,
  "mode": "hybrid",
  "judge": {
    "provider": "ark",
    "comparison_mode": "pairwise_preferred",
    "model": "judge-model-not-equal-to-answer-model",
    "ensemble": [
      {"provider": "ark", "model": "doubao-seed-2-1-pro-260628"},
      {"provider": "openai_compatible", "model": "third-party-judge-model"}
    ],
    "prompt_version": "judge-v1",
    "repeat_count": 3,
    "metrics": ["faithfulness", "groundedness", "answer_relevance", "hallucination", "completeness"],
    "rubric": {
      "faithfulness": "Answer must only make claims supported by retrieved context.",
      "groundedness": "Each material claim should be linked to context evidence.",
      "answer_relevance": "Answer should directly address the user query.",
      "hallucination": "Penalize unsupported facts, invented entities, or contradicted facts.",
      "completeness": "Answer should cover all required facts from ground truth."
    },
    "calibration": {
      "gold_split": "gold",
      "report_spearman": true,
      "report_cohen_kappa": true
    }
  }
}
```

### Get Evaluation Detail

```http
GET /v1/evaluations/{id}?include_items=true&include_judge=true
```

### Run Goal-Driven Optimization

```http
POST /v1/optimizations
```

Optimization is asynchronous. The submit API returns `202 Accepted` with a run ID immediately. Heavy work continues in the background because pairwise judging, A/B swaps, ensemble judges, repeats, 50 candidates, and holdout re-evaluation can take minutes to hours.

```json
{
  "dataset_id": "ds_xxx",
  "knowledge_base_id": "kb_xxx",
  "objective": {
    "name": "high_grounded_quality_low_latency",
    "primary": {
      "type": "pairwise_win_rate",
      "baseline": "current_profile",
      "minimum_significant_delta": 0.03
    },
    "fallback_formula": "0.35*groundedness + 0.25*answer_relevance + 0.2*completeness + 0.1*context_recall + 0.1*(1-normalized_latency)",
    "normalization": {
      "latency_p95_limit_ms": 2500,
      "cost_budget_usd": 2.0
    },
    "constraints": [
      {"metric": "faithfulness", "op": ">=", "value": 0.85},
      {"metric": "hallucination", "op": "<=", "value": 0.05},
      {"metric": "latency_p95_ms", "op": "<=", "value": 2500}
    ],
    "tie_breakers": [
      {"metric": "latency_p95_ms", "direction": "asc"},
      {"metric": "cost_usd", "direction": "asc"}
    ]
  },
  "search_space": {
    "prompts": [
      {"name": "strict_context_v1", "system": "只基于上下文回答。"},
      {"name": "cite_every_claim_v1", "system": "每个关键事实必须引用证据。"}
    ],
    "chunking": {
      "enabled": false,
      "reason": "Re-chunking and re-embedding are expensive. Enable only in dedicated offline experiments.",
      "size_tokens": [500, 800, 1200],
      "overlap_tokens": [80, 120, 200],
      "parser_method": ["basic", "mineru", "docling"]
    },
    "embedding": {
      "enabled": false,
      "reason": "Embedding model changes require isolated re-indexing and cleanup.",
      "models": ["doubao-embedding-vision-251215"],
      "dimensions": [1024]
    },
    "reranker": {
      "providers": ["volcengine", "aliyun"],
      "models": ["m3-v2-rerank", "qwen3-rerank"],
      "top_n": [4, 8, 12]
    },
    "retrieval": {
      "dense_top_k": [20, 50, 80],
      "sparse_top_k": [20, 50, 80],
      "rrf_k": [30, 60, 90],
      "semantic_cache_threshold": [0.88, 0.92, 0.96]
    },
    "graph": {
      "query_rewrite_enabled": [true, false],
      "hyde_enabled": [true, false],
      "multi_query_count": [1, 3, 5],
      "modules": [
        ["semantic_cache_lookup", "hybrid_retrieve", "ark_rerank", "context_pack", "ark_generate"],
        ["query_rewrite", "multi_query", "hybrid_retrieve", "ark_rerank", "context_pack", "ark_generate"]
      ]
    }
  },
  "runner": {
    "type": "internal_rag",
    "strategy": "random_successive_halving",
    "max_candidates": 50,
    "seed": 20260704,
    "parallelism": 2,
    "budget": {
      "max_judge_calls": 1500,
      "max_cost_usd": 2.0,
      "max_wall_time_seconds": 14400
    },
    "rate_limits": {
      "ark": {"max_concurrent": 2, "qps": 1, "retry_max": 5},
      "openai_compatible": {"max_concurrent": 1, "qps": 0.5, "retry_max": 5}
    },
    "splits": {
      "selection": ["train", "eval"],
      "final_report": "holdout"
    },
    "statistics": {
      "confidence_level": 0.95,
      "bootstrap_iterations": 1000,
      "promotion_rule": "significantly_better"
    },
    "judge": {
      "mode": "hybrid",
      "provider": "ark",
      "comparison_mode": "pairwise_preferred",
      "model": "judge-model-not-equal-to-answer-model"
    }
  }
}
```

Submit response:

```json
{
  "id": "opt_xxx",
  "status": "queued",
  "poll_url": "/v1/optimizations/opt_xxx",
  "cancel_url": "/v1/optimizations/opt_xxx:cancel",
  "resume_url": "/v1/optimizations/opt_xxx:resume"
}
```

### Get Optimization Status

```http
GET /v1/optimizations/{id}
```

The status response includes run status, candidate progress, checkpoint, best-so-far, cost, token usage, temporary namespaces, and cleanup status.

### Cancel Optimization

```http
POST /v1/optimizations/{id}:cancel
```

Cancellation is cooperative. The worker stops scheduling new candidates, waits for in-flight safe points, persists checkpoint state, and runs cleanup for temporary namespaces that are no longer needed.

### Resume Optimization

```http
POST /v1/optimizations/{id}:resume
```

Resume uses persisted checkpoints. Completed candidates are not repeated. Queued or running candidates without completed artifacts are retried according to idempotency keys and candidate status.

### External Harness Runner

```json
{
  "runner": {
    "type": "external_harness",
    "harness": {
      "kind": "codex-cli",
      "argv": [
        "codex-cli",
        "eval",
        "--dataset",
        "/workspace/.orag/runs/opt_xxx/dataset.jsonl",
        "--config",
        "/workspace/.orag/runs/opt_xxx/candidate.json",
        "--output",
        "/workspace/.orag/runs/opt_xxx/output"
      ],
      "working_dir": "/workspace/orag",
      "timeout_seconds": 1800,
      "output_format": "json",
      "metrics_path": "metrics.json",
      "artifact_paths": ["reports/", "traces/"],
      "redaction": {
        "redact_env": true,
        "redact_argv": true,
        "redact_stdout": true,
        "redact_stderr": true
      }
    }
  }
}
```

Harness execution must use `exec.CommandContext(ctx, argv[0], argv[1:]...)` or equivalent argv-array execution. Shell execution, command strings, and variable interpolation are not allowed.

---

## Judge Prompt Contract

### Pairwise Output

Pairwise is the default ranking contract. The judge compares two answers for the same query, ground truth, retrieved evidence, and rubric. This is more reliable than asking for a fragile absolute score.

Judge prompts must require strict JSON output:

```json
{
  "winner": "A",
  "preference": "A_better",
  "findings": [
    {
      "severity": "minor",
      "metric": "citation_support",
      "message": "Answer B cites a chunk that does not support the claim.",
      "evidence": "chunk_12"
    }
  ],
  "rationale": "Answer A is better grounded in the provided evidence and misses fewer required facts."
}
```

Allowed `winner` values are `A`, `B`, and `tie`. Allowed `preference` values are `A_much_better`, `A_better`, `tie`, `B_better`, and `B_much_better`.

To reduce position bias, pairwise judging must support A/B order swapping:

1. Run judge with original order: `A=candidate`, `B=baseline`.
2. Run judge with swapped order: `A=baseline`, `B=candidate`.
3. Convert both decisions back to canonical candidate IDs.
4. Aggregate by majority vote or average preference strength.
5. Mark the pair as unstable if the two orders contradict each other.

Prompt wording should ask the judge to reason against the rubric before emitting the final JSON. The persisted response stores only the final structured rationale and findings; raw chain-of-thought should not be exposed in public API unless explicitly configured for internal debugging.

### QAG Score Output

QAG Score is an evidence-grounded faithfulness check for RAG:

1. Generate verification questions from the answer's material claims.
2. Answer each verification question using only retrieved/source context.
3. Compare the context-derived answer with the original answer claim.
4. Score how many claims are supported, contradicted, or unverifiable.

```json
{
  "qag_score": 0.86,
  "questions": [
    {
      "question": "ORAG 默认使用什么向量数据库？",
      "claim": "ORAG 默认使用 Qdrant 作为向量数据库。",
      "context_answer": "Qdrant",
      "support": "supported",
      "evidence": ["chunk_1"]
    },
    {
      "question": "ORAG 是否默认启动 Neo4j？",
      "claim": "ORAG 默认启动 Neo4j。",
      "context_answer": "默认不启动 ES/Neo4j。",
      "support": "contradicted",
      "evidence": ["chunk_3"]
    }
  ],
  "summary": {
    "supported": 1,
    "contradicted": 1,
    "unverifiable": 0
  }
}
```

Allowed `support` values are `supported`, `contradicted`, and `unverifiable`.

QAG is not treated as automatically trustworthy. It adds two LLM-dependent steps: question generation and context-only answering. Quality checks must include:

- Human spot checks for whether generated questions cover the answer's key claims.
- Coverage metrics such as `qag_claim_coverage`, `qag_question_count`, and `qag_unverifiable_rate`.
- Gold-set calibration for both support labels and claim coverage.
- A warning when QAG misses material claims that human reviewers mark as important.

### Absolute Evidence-Check Output

Absolute scoring is allowed only for evidence-checkable metrics or coarse buckets. It is not the primary ranking signal unless pairwise comparison is impossible.

```json
{
  "labels": {
    "faithfulness": "good",
    "groundedness": "mixed",
    "answer_relevance": "good",
    "hallucination": "good",
    "completeness": "mixed",
    "citation_support": "good"
  },
  "scores": {
    "faithfulness": 0.9,
    "groundedness": 0.67,
    "answer_relevance": 0.9,
    "hallucination": 0.05,
    "completeness": 0.67,
    "citation_support": 0.9
  },
  "confidence": {
    "faithfulness": {"mean": 0.9, "low": 0.82, "high": 0.95, "n": 3, "method": "repeated_judge"},
    "groundedness": {"mean": 0.67, "low": 0.52, "high": 0.78, "n": 3, "method": "repeated_judge"}
  },
  "pass": true,
  "findings": [],
  "rationale": "All material claims are supported, but one citation is weaker than the rest."
}
```

Coarse labels should be `good`, `mixed`, or `bad`. Numeric scores are summaries for reporting and objectives, not precise truth. If absolute scoring is used, run the judge `k` times or with a judge ensemble and report confidence intervals.

### Judge Versioning

Judge reproducibility must be anchored by a content hash, not by a manually supplied version string alone:

```text
judge_config_hash = sha256(prompt_template + rubric + judge_model + judge_params + output_schema)
```

Changing the rubric without changing `prompt_version` is still a new judge configuration because `rubric_hash` and `judge_config_hash` change.

### Judge Calibration

- Maintain a human-labeled `gold` split.
- Use 50-100 human-labeled samples as the minimum calibration set before trusting judge-driven optimization.
- Periodically run judge outputs against human labels.
- Report Spearman correlation for ordinal/numeric quality scores.
- Report Cohen's kappa for coarse labels or pass/fail decisions.
- Use dimension-specific agreement thresholds instead of one global `κ >= 0.8` gate:
  - Evidence-checkable dimensions such as `faithfulness`, `citation_support`, and QAG support labels target `κ >= 0.75`.
  - More subjective dimensions such as `completeness` and `answer_relevance` target `κ >= 0.6`.
  - Below-threshold dimensions can still be reported, but cannot drive automatic promotion unless an explicit human-review waiver is recorded.
- Prefer judge models that are different from the tested generation model.
- When budget allows, use a 2-3 model ensemble and aggregate by median score or majority pairwise vote.
- Require human spot checks before key releases, especially when the optimizer changes prompts, retrieval strategy, or model selection.

### Judge Call Control

- All judge providers must use a provider-specific concurrency gate.
- 429, 503, and transient network failures use exponential backoff with jitter.
- Each judge call has a timeout and a retry cap.
- Provider-level circuit breakers stop calls after repeated failures.
- `max_judge_calls`, `max_cost_usd`, and `max_wall_time_seconds` are hard stop conditions.
- The optimizer must persist call counters and cost in checkpoints so resumed runs do not exceed budgets.

### Metric Semantics

- `faithfulness`: whether answer claims are supported by retrieved evidence.
- `groundedness`: whether evidence is actually used and citations align with claims.
- `answer_relevance`: whether the response directly addresses the query.
- `hallucination`: unsupported, contradicted, or invented facts; lower is better.
- `completeness`: whether expected facts and constraints are covered.
- `citation_support`: whether citations point to documents/chunks that support the answer.
- `qag_score`: whether answer-derived verification questions can be answered consistently from the retrieved/source context.
- `instruction_following`: whether system/user constraints are obeyed.

### Reporting Rules

- Report mean and confidence interval for every metric.
- Candidate comparisons must use paired tests or bootstrap on per-item differences.
- Promotion requires "significantly better" rather than a higher naked mean.
- Report anchor metrics and judge metrics together. Large divergence requires warning.
- Final conclusion must include holdout performance that was not used by candidate selection.
- Report pairwise A/B order-swap stability.
- Report QAG support counts: supported, contradicted, and unverifiable.
- Report token usage and cost separately for RAG generation, judge calls, QAG generation, QAG context answering, and external harnesses.

---

## Optimizer Strategy

### Candidate Sources

- User-provided static search space.
- Built-in generators for common RAG dimensions.
- LLM-proposed prompt variants with guardrails.
- Historical best candidates from previous runs.
- External harness-provided candidates.

### Search Algorithms

Before sampling, compute and report `search_space_size`. If the Cartesian product is larger than `max_candidates`, return a warning that grid coverage is low and switch to seeded random or Bayesian sampling unless the user explicitly requests grid.

1. **Seeded random search**
   - Default baseline for non-trivial spaces.
   - Handles large spaces better than grid when only 50-200 candidates are affordable.
   - Supports reproducibility through a stable seed.

2. **Dependency-aware sampling**
   - Validate dependencies before candidate creation.
   - Example: `reranker.provider=aliyun` may require `reranker.model=qwen3-rerank`.
   - Example: `embedding.enabled=false` disables embedding dimensions and index namespace dimensions.
   - Example: `chunking.enabled=false` removes chunk size, overlap, parser, and re-index candidates.

3. **Bayesian or bandit search**
   - Optional after random search is stable.
   - Useful when objective is expensive and metric noise is high.

4. **Successive halving**
   - Evaluate many candidates on a small dataset slice.
   - Promote top candidates to larger slices.
   - Promotion must use paired bootstrap or pairwise win-rate significance, not naked mean.

5. **LLM-guided mutation**
   - Use judge findings to propose prompt/config changes.
   - Must generate candidate diffs, not mutate production config directly.
   - Must not see holdout labels or holdout item results.

6. **External harness validation**
   - Run final top candidates through codex-cli or other harness tools.
   - Parse metrics and artifacts back into ORAG.
   - Treat external harness metrics as another evidence source in the objective.

### Checkpoint And Resume

- Persist a checkpoint after every candidate state transition: `queued`, `running`, `evaluated`, `judged`, `scored`, `promoted`, `holdout_evaluated`, `cleanup_done`, or `failed`.
- Candidate IDs must be deterministic from `optimization_run_id + candidate_config_hash` so retry does not duplicate work.
- Resume must skip completed candidates and continue from the first incomplete checkpoint.
- Running candidates from an interrupted worker are reconciled as `retryable` if no terminal artifact exists.
- Judge call counters, token usage, cost, temporary namespaces, and harness artifacts are part of the checkpoint.
- Cancellation uses the same checkpoint mechanism and records `cancel_requested_at` plus `status_reason`.

### Objective Expression Safety

- Use a restricted expression engine such as `expr-lang/expr` in safe mode, or a small internal parser.
- Variables are limited to the central metric whitelist and derived budget variables such as `normalized_latency` and `normalized_cost`.
- Operators are limited to `+`, `-`, `*`, `/`, and parentheses.
- Function calls, property access, indexing, reflection, assignments, string interpolation, and unknown identifiers are rejected at parse time.
- `normalized_latency` uses the configured `latency_p95_limit_ms`, not batch min-max.
- All metric names from judge, QAG, deterministic scoring, and harness output are validated before objective evaluation.

### Expensive Dimensions

- Re-chunking, re-embedding, and rebuilding vector indexes are slow and expensive. Default them to disabled.
- Enable chunking or embedding search only in explicit offline experiments with a budget, a unique namespace, and cleanup policy.
- Prefer optimizing cheap knobs first: prompt variants, rerank top-n, dense/sparse top-k, RRF k, query rewrite, HyDE, multi-query count, context packing, and module order.
- If index-affecting candidates are enabled, create candidate-specific collection/index namespaces and persist them in `optimization_candidates.temp_namespaces`.

### Dataset Split Policy

- `train`: used for fast exploration and LLM-guided mutation.
- `eval`: used for candidate selection and pairwise comparisons.
- `holdout`: never used for proposal, mutation, search, or selection.
- `gold`: used only for judge calibration against human labels.
- Final report must include holdout results for the selected candidate and baseline.
- If holdout ranking disagrees with eval ranking, mark the optimization as inconclusive unless the user explicitly accepts the risk.

### Safety Rules

- Never mutate production config during optimization.
- Always run candidates against isolated config snapshots.
- Do not rebuild global indexes unless candidate uses a unique index namespace.
- Every temporary namespace must have an owner candidate ID, `expires_at`, and cleanup status.
- Cleanup must run at optimization completion and through a periodic GC fallback.
- Persist namespace cleanup artifacts so interrupted runs can be resumed or garbage-collected.
- Redact env vars, argv, stdout, stderr, parsed metrics, and artifact manifests in persisted harness records.
- Enforce max candidates, timeout, judge call budget, cost budget, wall-time budget, provider rate limits, and concurrency.
- Require explicit allowlist for external executable paths and forbid shell execution.
- Validate metric names against the central metric whitelist before storing any score map.

---

## Implementation Tasks

### Task 1: Add Evaluation Mode And Judge Types

**Files:**
- Modify: `internal/eval/service.go`
- Create: `internal/eval/judge_types.go`
- Create: `internal/eval/metrics_registry.go`
- Test: `internal/eval/judge_types_test.go`

**Step 1: Write failing tests**

Create tests for JSON round-trip of `EvaluationMode`, `JudgeMetric`, `JudgeConfig`, `JudgeInput`, `JudgeOutput`, `PairwiseJudgeInput`, `PairwiseJudgeOutput`, `ConfidenceInterval`, dataset split fields, and metric whitelist validation.

**Step 2: Implement types**

Add typed constants for evaluation modes, comparison modes, judge metrics, and dataset splits. Add a central metric registry that validates all score map keys before aggregation or persistence. Add request fields to `RunRequest` without changing current behavior when omitted.

**Step 3: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/eval -run TestJudgeTypes -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/eval/service.go internal/eval/judge_types.go internal/eval/judge_types_test.go
git commit -m "feat(eval): add judge evaluation types"
```

### Task 2: Add Judge Interface And Rule-Based Adapter

**Files:**
- Create: `internal/eval/judge.go`
- Modify: `internal/eval/metrics.go`
- Test: `internal/eval/judge_test.go`

**Step 1: Write failing tests**

Test that `RuleBasedJudge` maps existing metrics into the new judge output format and preserves deterministic behavior.

**Step 2: Implement interface**

```go
type Judge interface {
    Judge(ctx context.Context, input JudgeInput) (JudgeOutput, error)
}

type PairwiseJudge interface {
    Compare(ctx context.Context, input PairwiseJudgeInput) (PairwiseJudgeOutput, error)
}

type QAGJudge interface {
    ScoreQAG(ctx context.Context, input JudgeInput) (QAGOutput, error)
}
```

**Step 3: Adapt current metrics**

Wrap `ScoreItem` as a rule-based judge so hybrid evaluation can reuse old and new scoring.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/eval -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/eval/judge.go internal/eval/metrics.go internal/eval/judge_test.go
git commit -m "feat(eval): add pluggable judge interface"
```

### Task 3: Implement LLM Judge Client

**Files:**
- Create: `internal/eval/llm_judge.go`
- Create: `internal/eval/judge_prompt.go`
- Test: `internal/eval/llm_judge_test.go`
- Modify: `internal/llm/ark/client.go` only if a narrow chat abstraction is needed.

**Step 1: Write failing tests**

Use a fake chat model that returns strict JSON judge output. Test pairwise parsing, A/B order swapping, contradiction detection between swapped runs, QAG parsing, coarse label parsing, score normalization, confidence intervals, malformed JSON handling, rubric hash generation, judge config hash generation, and raw response persistence.

**Step 2: Implement prompt renderer**

Render query, answer A/B, ground truth, citations, retrieved chunks, and rubric for pairwise comparison. Render QAG prompts that first generate claim-verification questions and then answer from source context. Render absolute evidence-check prompts only when requested. Require strict JSON output and use internal "reason before final JSON" instructions without exposing raw chain-of-thought in public APIs.

**Step 3: Implement LLM judge**

Call one or more judge models, parse JSON, clamp scores to `[0, 1]`, aggregate ensemble outputs by median or majority vote, support repeat count `k`, run swapped A/B order when pairwise mode is enabled, and return `JudgeOutput`, `PairwiseJudgeOutput`, or `QAGOutput`.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/eval -run 'TestLLMJudge|TestJudgePrompt' -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/eval/llm_judge.go internal/eval/judge_prompt.go internal/eval/llm_judge_test.go
git commit -m "feat(eval): add llm judge"
```

### Task 4: Persist Judge Runs And Results

**Files:**
- Create: `migrations/000006_dataset_eval_metadata.up.sql`
- Create: `migrations/000006_dataset_eval_metadata.down.sql`
- Create: `migrations/000007_judge_results.up.sql`
- Create: `migrations/000007_judge_results.down.sql`
- Create: `migrations/000008_optimizer_runs.up.sql`
- Create: `migrations/000008_optimizer_runs.down.sql`
- Create: `migrations/000009_harness_runs.up.sql`
- Create: `migrations/000009_harness_runs.down.sql`
- Modify: `internal/eval/service.go`
- Modify: `internal/storage/postgres/eval.go`
- Test: `internal/storage/postgres/repository_test.go`

**Step 1: Write failing repository tests**

Test storing and retrieving judge run metadata, judge item results, pairwise judge results, calibration runs, rubric hashes, prompt hashes, confidence intervals, raw text responses, parsed JSON responses, token usage, cost, and dataset split fields.

**Step 2: Add migration**

Add dataset split columns plus `judge_runs`, `judge_results`, `pairwise_judge_results`, `judge_calibration_runs`, `optimization_runs`, `optimization_candidates`, and `harness_runs`. Include down migrations with `DROP` and `ALTER TABLE DROP COLUMN` rollback paths.

**Step 3: Extend repository interfaces**

Add `StoreJudgeRun`, `StoreJudgeResult`, `StorePairwiseJudgeResult`, `StoreJudgeCalibrationRun`, and detail retrieval methods.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add migrations/000006_judge_and_optimizer.sql internal/eval/service.go internal/storage/postgres/eval.go internal/storage/postgres/repository_test.go
git commit -m "feat(eval): persist judge results"
```

### Task 5: Extend Evaluation Runner

**Files:**
- Modify: `internal/eval/service.go`
- Modify: `internal/app/app.go`
- Test: `internal/eval/service_test.go`
- Test: `internal/app/app_test.go`

**Step 1: Write failing tests**

Test:
- `rule_based` mode preserves existing response.
- `hybrid` mode includes judge metrics.
- `pairwise_preferred` mode produces pairwise comparisons when a baseline or candidate pair exists.
- pairwise mode runs swapped A/B order and reports stability.
- QAG mode reports supported, contradicted, and unverifiable counts.
- `gold` split calibration reports Spearman correlation and Cohen's kappa.
- calibration agreement below `0.8` blocks optimizer-driven promotion.
- LLM judge failure records a warning or returns a controlled error based on request policy.

**Step 2: Add judge orchestration**

After each RAG response, build `JudgeInput`, run configured judges, run QAG when requested, merge metrics, persist judge outputs, and keep deterministic anchor metrics separate from judge metrics.

**Step 3: Keep backwards compatibility**

When `mode` and `judge` are omitted, behavior must match current API.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/eval ./internal/app -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/eval/service.go internal/app/app.go internal/eval/service_test.go internal/app/app_test.go
git commit -m "feat(eval): run llm judge during evaluation"
```

### Task 6: Add Evaluation Detail API

**Files:**
- Modify: `internal/http/router.go`
- Modify: `api/openapi.yaml`
- Modify: `docs/api.md`
- Test: `internal/http/router_test.go`
- Test: `tests/contract/openapi_test.go`

**Step 1: Write failing route tests**

Test `GET /v1/evaluations/{id}?include_items=true&include_judge=true`.

**Step 2: Implement response model**

Return run summary plus optional item-level answer, rule metrics, judge metrics, pairwise preferences, confidence intervals, calibration summary, rationale, and findings.

**Step 3: Update OpenAPI and docs**

Document `mode`, `judge`, `comparison_mode`, `include_items`, `include_judge`, `include_pairwise`, confidence intervals, and calibration fields.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/http ./tests/contract -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/http/router.go api/openapi.yaml docs/api.md internal/http/router_test.go tests/contract/openapi_test.go
git commit -m "feat(api): expose evaluation judge details"
```

### Task 7: Add Objective Specification

**Files:**
- Create: `internal/optimizer/objective.go`
- Create: `internal/optimizer/objective_test.go`
- Create: `internal/optimizer/expression.go`
- Modify: `internal/eval/optimizer.go` or move optimizer into `internal/optimizer`.

**Step 1: Write failing tests**

Test formula scoring, restricted expression parsing, unknown metric rejection, function-call rejection, property-access rejection, pairwise win-rate objectives, constraints, tie breakers, missing metrics, inverse metrics such as hallucination, fixed-budget latency normalization, confidence intervals, paired bootstrap promotion, and deterministic ordering.

**Step 2: Implement objective evaluator**

Support:
- weighted metrics
- pairwise win-rate and baseline comparison
- explicit formula subset
- metric whitelist validation
- restricted expression engine
- fixed-budget normalization for latency and cost
- constraints
- tie breakers
- confidence-aware promotion
- score explanations

**Step 3: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/optimizer -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/optimizer/objective.go internal/optimizer/objective_test.go internal/eval/optimizer.go
git commit -m "feat(optimizer): add objective scoring"
```

### Task 8: Add Candidate Config And Search Space

**Files:**
- Create: `internal/optimizer/candidate.go`
- Create: `internal/optimizer/search_space.go`
- Test: `internal/optimizer/search_space_test.go`

**Step 1: Write failing tests**

Test expanding prompt, chunking, retrieval, reranker, graph, and harness search spaces into stable candidate IDs. Include dependency-aware pruning and search-space-size warnings.

**Step 2: Implement candidate snapshot**

Represent all tunable dimensions as immutable JSON-serializable config.

**Step 3: Add search strategies**

Implement seeded random search and dependency-aware sampling first. Keep grid only for small spaces where Cartesian product is below `max_candidates`. Defer LLM-guided mutation until baseline optimizer is stable.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/optimizer -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/optimizer/candidate.go internal/optimizer/search_space.go internal/optimizer/search_space_test.go
git commit -m "feat(optimizer): add candidate search space"
```

### Task 9: Add Internal Candidate Runner

**Files:**
- Create: `internal/optimizer/runner.go`
- Create: `internal/optimizer/internal_runner.go`
- Modify: `internal/rag/service.go`
- Modify: `internal/graph/rag_graph.go`
- Test: `internal/optimizer/internal_runner_test.go`

**Step 1: Write failing tests**

Use a fake RAG service and verify candidate config is applied without mutating production service state.

**Step 2: Introduce config snapshot application**

Candidate application must clone or overlay prompt, retrieval, rerank, graph, and chunk/index settings.

**Step 3: Handle chunk/index candidates safely**

For chunking and embedding/index candidates, default to disabled. When explicitly enabled, use isolated temporary collection/index namespace rather than rewriting production indexes, persist namespace artifacts, and attach TTL metadata.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/optimizer ./internal/rag ./internal/graph -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/optimizer/runner.go internal/optimizer/internal_runner.go internal/optimizer/internal_runner_test.go internal/rag/service.go internal/graph/rag_graph.go
git commit -m "feat(optimizer): add internal rag candidate runner"
```

### Task 10: Add External Harness Runner

**Files:**
- Create: `internal/optimizer/harness.go`
- Create: `internal/optimizer/redaction.go`
- Create: `internal/optimizer/harness_test.go`
- Modify: `internal/config/config.go`
- Modify: `.env.example`

**Step 1: Write failing tests**

Test argv validation, executable allowlist enforcement, shell-command rejection, timeout, JSON metrics parsing, stdout/stderr capture, env/argv/stdout/stderr/artifact redaction, and metric whitelist validation.

**Step 2: Implement harness interface**

```go
type HarnessRunner interface {
    Run(ctx context.Context, candidate CandidateConfig, dataset DatasetExport) (HarnessResult, error)
}
```

**Step 3: Implement process runner**

Support `codex-cli` and generic command harness through an explicit executable allowlist. Use argv-array execution only. Do not support shell templates or `${VAR}` interpolation.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/optimizer -run TestHarness -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/optimizer/harness.go internal/optimizer/harness_test.go internal/config/config.go .env.example
git commit -m "feat(optimizer): add external harness runner"
```

### Task 11: Persist Optimizer Runs

**Files:**
- Create: `internal/optimizer/repository.go`
- Modify: `internal/storage/postgres/eval.go` or create `internal/storage/postgres/optimizer.go`
- Test: `internal/storage/postgres/repository_test.go`

**Step 1: Write failing repository tests**

Test create run, append candidates, update statuses, store checkpoints, resume from checkpoints, store artifacts, retrieve best candidate, store holdout scores, record search-space size, record sampled count, record token usage/cost, record temp namespaces, and list runs.

**Step 2: Implement persistence**

Use `optimization_runs`, `optimization_candidates`, and `harness_runs`. Persist cleanup status and expiration for temporary namespaces, status reasons, cancel timestamps, token usage, cost, and checkpoint JSON.

**Step 3: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/optimizer/repository.go internal/storage/postgres/optimizer.go internal/storage/postgres/repository_test.go
git commit -m "feat(optimizer): persist optimization runs"
```

### Task 12: Implement Goal-Driven Optimizer Service

**Files:**
- Create: `internal/optimizer/service.go`
- Create: `internal/optimizer/worker.go`
- Create: `internal/optimizer/checkpoint.go`
- Create: `internal/optimizer/ratelimit.go`
- Create: `internal/optimizer/service_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/http/router.go`

**Step 1: Write failing service tests**

Test:
- objective-driven best selection
- constraint rejection
- runner error handling
- max candidate cap
- deterministic tie breaking
- hybrid judge metrics used by objective
- pairwise win-rate objective over baseline
- paired bootstrap promotion rule
- holdout is never used during candidate selection
- final selected candidate is re-evaluated on holdout
- temporary namespace cleanup is triggered on completion and failure
- submit returns `202` and run ID without blocking for completion
- polling returns progress, best-so-far, token usage, cost, and cleanup status
- cancel records checkpoint and stops scheduling new candidates
- resume skips completed candidates and retries only incomplete candidates
- judge provider rate limits, 429 backoff, timeout, and circuit breaker behavior
- budget hard stops for judge call count, wall time, and cost

**Step 2: Implement service**

Generate candidates, enqueue work, run candidate runner asynchronously, run evaluation/judge, score objective, run pairwise comparisons, calculate confidence intervals, promote only significantly better candidates, persist checkpoints after every state transition, enforce provider rate limits and budgets, run holdout evaluation for the final best candidate, clean temporary namespaces, and expose progress through polling.

**Step 3: Wire into app and router**

Replace current simple `eval.Optimizer` call with new asynchronous optimizer service. Add `POST /v1/optimizations`, `GET /v1/optimizations/{id}`, `POST /v1/optimizations/{id}:cancel`, and `POST /v1/optimizations/{id}:resume`.

**Step 4: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/optimizer ./internal/http ./internal/app -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/optimizer/service.go internal/optimizer/service_test.go internal/app/app.go internal/http/router.go
git commit -m "feat(optimizer): run goal-driven optimization"
```

### Task 13: Update OpenAPI, Docs, And Examples

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `docs/evaluation.md`
- Modify: `docs/api.md`
- Modify: `docs/development.md`
- Modify: `examples/curl/40_eval.sh`
- Create: `examples/curl/46_optimize_goal.sh`
- Test: `tests/contract/openapi_test.go`
- Test: `tests/contract/examples_test.go`

**Step 1: Write failing contract tests**

Add contract assertions for new schemas and example scripts.

**Step 2: Update OpenAPI**

Document judge config, pairwise judge results, absolute evidence-check results, confidence intervals, calibration reports, objective spec, safe expression syntax, search space, runner config, async optimization APIs, checkpoint/resume semantics, external harness argv config, redaction rules, temporary namespace cleanup, and optimization candidate responses.

**Step 3: Update docs**

Document metric semantics, pairwise prompt contracts, absolute evidence-check prompt contracts, judge version hashing, gold set calibration, holdout policy, objective design, optimizer safety, and external harness setup.

**Step 4: Add examples**

Add curl examples for hybrid judge evaluation, pairwise optimization against a baseline, and goal-driven optimization with holdout reporting.

**Step 5: Run tests**

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v
```

Expected: PASS.

**Step 6: Commit**

```bash
git add api/openapi.yaml docs/evaluation.md docs/api.md docs/development.md examples/curl/40_eval.sh examples/curl/46_optimize_goal.sh tests/contract/openapi_test.go tests/contract/examples_test.go
git commit -m "docs: document llm judge and goal optimizer"
```

### Task 14: End-To-End Validation

**Files:**
- Modify: `Makefile`
- Create: `tests/contract/judge_optimizer_test.go`
- Create: `tests/live/judge_optimizer_live_test.go`

**Step 1: Add deterministic contract test**

Use fake judge and memory backend to verify the full API flow without external providers, including pairwise preferences, gold calibration, confidence intervals, and holdout reporting.

**Step 2: Add optional live test**

Gate live Ark judge test behind `LIVE_ARK_TESTS=1`. If live tests are enabled, verify judge model is different from the answer generation model or use an ensemble.

**Step 3: Add make targets**

Add:

```makefile
test-judge:
	CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/eval ./internal/optimizer ./tests/contract -v
```

**Step 4: Run full validation**

```bash
make fmt
make vet
make test
go test ./tests/contract -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add Makefile tests/contract/judge_optimizer_test.go tests/live/judge_optimizer_live_test.go
git commit -m "test: add judge optimizer validation"
```

---

## Rollout Plan

1. Ship small reversible database migrations and type additions with no API behavior change.
2. Ship dataset split fields and keep default split as `eval`.
3. Ship metric registry and score-key whitelist before accepting judge or harness metrics.
4. Ship rule-based judge adapter and preserve current evaluation responses.
5. Add optional LLM judge behind explicit request config, with pairwise comparison as the preferred mode.
6. Add judge config hashing, QAG coverage checks, and gold-set calibration before optimizer relies on judge outputs.
7. Add detail API for item-level judge results and pairwise preferences.
8. Replace simple optimizer with asynchronous objective-driven optimizer while preserving current `profiles/top_ks` request as a compatibility shortcut.
9. Enable external harness only when argv execution, executable allowlist, and stdout/stderr redaction are configured.
10. Add live tests, holdout reports, cleanup jobs, checkpoint/resume jobs, and operational documentation after deterministic paths pass.

---

## Backward Compatibility

- Existing `POST /v1/evaluations` requests without `mode` or `judge` keep current rule-based behavior.
- Existing `POST /v1/optimizations` requests with only `profiles` and `top_ks` are translated into a default objective: maximize `accuracy`.
- New `POST /v1/optimizations` returns `202 Accepted` and run ID. Existing callers that expected a synchronous full result need to poll `GET /v1/optimizations/{id}`.
- Existing `GET /v1/evaluations/{id}` keeps returning run summary unless `include_items` or `include_judge` is requested.
- No production RAG config is mutated by optimization candidates.
- If no dataset split is provided, existing items are treated as `eval` and are not automatically used as holdout.

---

## Risks And Mitigations

- **Absolute-score noise:** Prefer pairwise ranking, QAG evidence checks, and coarse labels. Use repeated absolute scoring plus confidence intervals only when pairwise is not possible.
- **Position bias:** Run pairwise judging with swapped A/B order and aggregate decisions back to canonical candidate IDs.
- **Length/style bias:** Include rubric constraints that penalize verbosity without evidence and require evidence-grounded findings.
- **Judge instability:** Store judge model, prompt version, rubric hash, prompt hash, raw output, and rationale. Use deterministic fake judge for CI.
- **Self-preference bias:** Prefer judge models that differ from the answer generation model. Use 2-3 model ensembles and median/majority aggregation where budget allows.
- **Unknown judge quality:** Maintain a 50-100 item human-labeled gold split and periodically report Spearman correlation and Cohen's kappa against human labels. Use dimension-specific thresholds instead of a global `κ >= 0.8`, and require human-review waiver for below-threshold optimizer use.
- **QAG uncertainty:** QAG depends on LLM-generated questions and context answers. Calibrate support labels and manually inspect key-claim coverage so hallucinations are not missed because no verification question was generated.
- **Poor explainability:** Ask judge to reason against the rubric before final JSON, but expose only structured rationale/findings by default. Use raw CoT only for internal debugging with explicit configuration.
- **Release risk:** Require human spot checks before key releases, especially if optimizer changes prompts, retrieval strategy, or models.
- **Cost growth:** Enforce max candidates, max judge calls, timeouts, provider rate limits, circuit breakers, wall-time budget, token accounting, and cost budget.
- **External command risk:** Require argv-array execution, executable allowlist, timeout, redacted env/argv/stdout/stderr, and isolated working directory.
- **Migration rollback risk:** Every schema migration must have a matching down migration and be split into reversible, behavior-preserving rollout steps.
- **Metric key drift:** Validate all score map keys against a central metric registry before storage or objective evaluation.
- **Expression injection:** Use a restricted expression engine with metric-variable whitelist and no function calls, property access, or interpolation.
- **Async run interruption:** Persist checkpoints after each candidate state transition and support cancel/resume using idempotent candidate IDs.
- **Index mutation risk:** Disable re-chunking, re-embedding, and re-indexing by default. If enabled, use candidate-specific collection/index namespace.
- **Temporary resource leakage:** Register all temporary namespaces with owner, TTL, and cleanup status. Run completion cleanup and periodic GC.
- **Metric gaming:** Support constraints and multi-metric objectives rather than optimizing one quality metric alone.
- **Prompt overfitting:** Split datasets into train/eval/holdout subsets. Select on train/eval only and report holdout score separately.
- **Grid search false confidence:** Warn when Cartesian search space is much larger than candidate budget. Default to seeded random or Bayesian sampling.

---

## Delivered Vertical Slice

The merged implementation delivers the following vertical slice:

1. Add judge types and interface.
2. Implement fake/rule-based judge.
3. Implement pairwise LLM judge with strict JSON parsing.
4. Implement A/B order-swap aggregation for pairwise judging.
5. Implement QAG Score for RAG faithfulness checks.
6. Implement absolute evidence-check judge with coarse labels and confidence intervals.
7. Add metric registry and whitelist validation for all score maps.
8. Persist judge results, pairwise preferences, QAG results, raw text responses, parsed JSON responses, token usage, cost, and judge config hashes.
9. Add dataset split and gold calibration support with dimension-specific agreement thresholds.
10. Extend `POST /v1/evaluations` with `mode=hybrid`.
11. Add `GET /v1/evaluations/{id}?include_items=true&include_judge=true`.
12. Update OpenAPI and docs.

This delivered slice gives users reliable LLM-as-Judge evaluation together with the goal-driven optimizer path, while preserving deterministic metrics as the CI-safe baseline.
