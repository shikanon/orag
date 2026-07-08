package offlineknowledge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	evalpkg "github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/kb"
)

var (
	ErrCodexToolScopeViolation = errors.New("codex tool scope violation")
	ErrCodexToolUnavailable    = errors.New("codex tool dependency unavailable")
)

type CodexToolCall struct {
	SessionID string
	TenantID  string
	KBID      string
	Tool      ReadOnlyToolName
	Query     string
	Vector    []float64
	ChunkID   string
	DocID     string
	Entities  []string
	TopK      int
	MaxRows   int
	Timeout   time.Duration
	Cluster   QuestionCluster
}

type CodexToolResult struct {
	Tool          ReadOnlyToolName    `json:"tool"`
	Rows          []CodexToolRow      `json:"rows,omitempty"`
	EvalResults   []CodexEvalResult   `json:"eval_results,omitempty"`
	ExistingItems []OptimizationItem  `json:"existing_items,omitempty"`
	Replay        *RecallReplayResult `json:"replay,omitempty"`
}

type CodexToolRow struct {
	ChunkID          string            `json:"chunk_id"`
	DocID            string            `json:"doc_id"`
	DocVersion       string            `json:"doc_version,omitempty"`
	ChunkContentHash string            `json:"chunk_content_hash,omitempty"`
	Text             string            `json:"text,omitempty"`
	Score            float64           `json:"score,omitempty"`
	Rank             int               `json:"rank,omitempty"`
	From             string            `json:"from,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type CodexEvalResult struct {
	RunID         string             `json:"run_id"`
	DatasetItemID string             `json:"dataset_item_id,omitempty"`
	Answer        string             `json:"answer,omitempty"`
	Metrics       map[string]float64 `json:"metrics,omitempty"`
	CreatedAt     time.Time          `json:"created_at,omitempty"`
}

type CodexEvalResultLookup interface {
	LookupEvalResults(ctx context.Context, tenantID, kbID, query string, limit int) ([]CodexEvalResult, error)
}

type CodexToolAuditEvent struct {
	ID         string           `json:"id"`
	SessionID  string           `json:"session_id,omitempty"`
	TenantID   string           `json:"tenant_id"`
	KBID       string           `json:"kb_id"`
	Tool       ReadOnlyToolName `json:"tool"`
	Rows       int              `json:"rows"`
	Steps      int              `json:"steps"`
	Allowed    bool             `json:"allowed"`
	Error      string           `json:"error,omitempty"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt time.Time        `json:"finished_at"`
}

type CodexToolAuditSink interface {
	RecordCodexToolAudit(ctx context.Context, event CodexToolAuditEvent) error
}

type CodexToolMetrics interface {
	AddOfflineKnowledgeDeepSearchSteps(count int64)
}

type CodexToolRegistryOptions struct {
	Retriever   kb.Retriever
	ChunkSource kb.ChunkSource
	GraphStore  kb.GraphStore
	Repository  Repository
	EvalLookup  CodexEvalResultLookup
	Replayer    RecallReplayer
	Quota       ToolQuota
	MaxSteps    int
	Audit       CodexToolAuditSink
	Metrics     CodexToolMetrics
	Now         func() time.Time
	DefaultRows int
}

type CodexToolRegistry struct {
	retriever   kb.Retriever
	chunks      kb.ChunkSource
	graph       kb.GraphStore
	repository  Repository
	evalLookup  CodexEvalResultLookup
	replayer    RecallReplayer
	quota       ToolQuota
	maxSteps    int
	audit       CodexToolAuditSink
	metrics     CodexToolMetrics
	now         func() time.Time
	defaultRows int

	mu       sync.Mutex
	steps    map[string]int
	qps      map[string]tenantQPSWindow
	auditSeq uint64
}

type tenantQPSWindow struct {
	Second int64
	Count  int
}

func NewCodexToolRegistry(opts CodexToolRegistryOptions) *CodexToolRegistry {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = opts.Quota.MaxDeepSearchSteps
	}
	defaultRows := opts.DefaultRows
	if defaultRows <= 0 {
		defaultRows = 10
	}
	return &CodexToolRegistry{
		retriever:   opts.Retriever,
		chunks:      opts.ChunkSource,
		graph:       opts.GraphStore,
		repository:  opts.Repository,
		evalLookup:  opts.EvalLookup,
		replayer:    opts.Replayer,
		quota:       opts.Quota,
		maxSteps:    maxSteps,
		audit:       opts.Audit,
		metrics:     opts.Metrics,
		now:         now,
		defaultRows: defaultRows,
		steps:       map[string]int{},
		qps:         map[string]tenantQPSWindow{},
	}
}

func (r *CodexToolRegistry) Execute(ctx context.Context, call CodexToolCall) (CodexToolResult, error) {
	if r == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	started := r.now()
	if !isReadOnlyTool(call.Tool) {
		err := fmt.Errorf("%w: %q", ErrCodexUnknownTool, call.Tool)
		r.auditCall(ctx, call, started, 0, 0, false, err)
		return CodexToolResult{}, err
	}
	if err := r.reserveTenantQPS(call, started); err != nil {
		r.auditCall(ctx, call, started, 0, 0, false, err)
		return CodexToolResult{}, err
	}
	steps, err := r.reserveStep(call)
	if err != nil {
		r.auditCall(ctx, call, started, 0, steps, false, err)
		return CodexToolResult{}, err
	}
	if r.metrics != nil {
		r.metrics.AddOfflineKnowledgeDeepSearchSteps(1)
	}
	rowsLimit, err := r.rowsLimit(call)
	if err != nil {
		r.auditCall(ctx, call, started, 0, steps, false, err)
		return CodexToolResult{}, err
	}
	ctx, cancel, err := r.withCallTimeout(ctx, call)
	if err != nil {
		r.auditCall(ctx, call, started, 0, steps, false, err)
		return CodexToolResult{}, err
	}
	defer cancel()

	result, err := r.execute(ctx, call, rowsLimit)
	if err != nil {
		r.auditCall(ctx, call, started, 0, steps, false, err)
		return CodexToolResult{}, err
	}
	rowCount := resultRowCount(result)
	r.auditCall(ctx, call, started, rowCount, steps, true, nil)
	return result, nil
}

func (r *CodexToolRegistry) execute(ctx context.Context, call CodexToolCall, rowsLimit int) (CodexToolResult, error) {
	if call.TenantID == "" || call.KBID == "" {
		return CodexToolResult{}, fmt.Errorf("%w: tenant_id and kb_id are required", ErrCodexToolScopeViolation)
	}
	if !isReadOnlyTool(call.Tool) {
		return CodexToolResult{}, fmt.Errorf("%w: %q", ErrCodexUnknownTool, call.Tool)
	}
	switch call.Tool {
	case ReadOnlyToolSearchChunksByText:
		return r.search(ctx, call, rowsLimit, nil)
	case ReadOnlyToolSearchChunksVector:
		return r.search(ctx, call, rowsLimit, call.Vector)
	case ReadOnlyToolGetChunkNeighbors:
		return r.chunkNeighbors(call, rowsLimit)
	case ReadOnlyToolGetDocumentChunks:
		return r.documentChunks(call, rowsLimit)
	case ReadOnlyToolGetGraphChunks:
		return r.graphChunks(ctx, call, rowsLimit)
	case ReadOnlyToolLookupEvalResults:
		return r.evalResults(ctx, call, rowsLimit)
	case ReadOnlyToolLookupExistingItem:
		return r.existingItems(ctx, call, rowsLimit)
	case ReadOnlyToolReplayRecall:
		return r.replayRecall(ctx, call)
	default:
		return CodexToolResult{}, fmt.Errorf("%w: %q", ErrCodexUnknownTool, call.Tool)
	}
}

func (r *CodexToolRegistry) search(ctx context.Context, call CodexToolCall, rowsLimit int, vector []float64) (CodexToolResult, error) {
	if r.retriever == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	results, err := r.retriever.Retrieve(ctx, kb.SearchRequest{
		TenantID:        call.TenantID,
		KnowledgeBaseID: call.KBID,
		Query:           call.Query,
		Vector:          vector,
		TopK:            firstPositive(call.TopK, rowsLimit),
	})
	if err != nil {
		return CodexToolResult{}, err
	}
	return CodexToolResult{Tool: call.Tool, Rows: r.rowsFromSearchResults(ctx, call.TenantID, call.KBID, results, rowsLimit)}, nil
}

func (r *CodexToolRegistry) chunkNeighbors(call CodexToolCall, rowsLimit int) (CodexToolResult, error) {
	if r.chunks == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	chunks := sortedChunks(r.chunks.Chunks(call.TenantID, call.KBID))
	index := -1
	for i, chunk := range chunks {
		if chunk.ID == call.ChunkID {
			index = i
			break
		}
	}
	if index < 0 {
		return CodexToolResult{Tool: call.Tool}, nil
	}
	start := index - rowsLimit/2
	if start < 0 {
		start = 0
	}
	end := start + rowsLimit
	if end > len(chunks) {
		end = len(chunks)
	}
	rows := make([]CodexToolRow, 0, end-start)
	for _, chunk := range chunks[start:end] {
		if chunk.ID == call.ChunkID {
			continue
		}
		rows = append(rows, rowFromChunk(chunk, 0, 0, "neighbor"))
	}
	return CodexToolResult{Tool: call.Tool, Rows: rows}, nil
}

func (r *CodexToolRegistry) documentChunks(call CodexToolCall, rowsLimit int) (CodexToolResult, error) {
	if r.chunks == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	rows := make([]CodexToolRow, 0, rowsLimit)
	for _, chunk := range sortedChunks(r.chunks.Chunks(call.TenantID, call.KBID)) {
		if chunk.DocumentID != call.DocID {
			continue
		}
		rows = append(rows, rowFromChunk(chunk, 0, len(rows)+1, "document"))
		if len(rows) >= rowsLimit {
			break
		}
	}
	return CodexToolResult{Tool: call.Tool, Rows: rows}, nil
}

func (r *CodexToolRegistry) graphChunks(ctx context.Context, call CodexToolCall, rowsLimit int) (CodexToolResult, error) {
	if r.graph == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	entities := call.Entities
	if len(entities) == 0 {
		entities = kb.ExtractGraphEntities(call.Query, rowsLimit)
	}
	results, err := r.graph.ExpandGraph(ctx, kb.GraphExpansionRequest{
		TenantID:        call.TenantID,
		KnowledgeBaseID: call.KBID,
		Entities:        entities,
		Limit:           rowsLimit,
	})
	if err != nil {
		return CodexToolResult{}, err
	}
	return CodexToolResult{Tool: call.Tool, Rows: r.rowsFromSearchResults(ctx, call.TenantID, call.KBID, results, rowsLimit)}, nil
}

func (r *CodexToolRegistry) evalResults(ctx context.Context, call CodexToolCall, rowsLimit int) (CodexToolResult, error) {
	if r.evalLookup == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	results, err := r.evalLookup.LookupEvalResults(ctx, call.TenantID, call.KBID, call.Query, rowsLimit)
	if err != nil {
		return CodexToolResult{}, err
	}
	if len(results) > rowsLimit {
		results = results[:rowsLimit]
	}
	return CodexToolResult{Tool: call.Tool, EvalResults: results}, nil
}

func (r *CodexToolRegistry) existingItems(ctx context.Context, call CodexToolCall, rowsLimit int) (CodexToolResult, error) {
	if r.repository == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	items, err := r.repository.ListOptimizationItems(ctx, OptimizationItemFilter{
		TenantID: call.TenantID,
		KBID:     call.KBID,
		Limit:    rowsLimit,
	})
	if err != nil {
		return CodexToolResult{}, err
	}
	if query := strings.TrimSpace(call.Query); query != "" {
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.CanonicalQuestion), strings.ToLower(query)) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return CodexToolResult{Tool: call.Tool, ExistingItems: items}, nil
}

func (r *CodexToolRegistry) replayRecall(ctx context.Context, call CodexToolCall) (CodexToolResult, error) {
	if r.replayer == nil {
		return CodexToolResult{}, ErrCodexToolUnavailable
	}
	cluster := call.Cluster
	if cluster.TenantID == "" {
		cluster.TenantID = call.TenantID
	}
	if cluster.KBID == "" {
		cluster.KBID = call.KBID
	}
	if cluster.CanonicalQuestion == "" {
		cluster.CanonicalQuestion = call.Query
	}
	if cluster.TenantID != call.TenantID || cluster.KBID != call.KBID {
		return CodexToolResult{}, ErrCodexToolScopeViolation
	}
	replay, err := r.replayer.ReplayRecall(ctx, cluster)
	if err != nil {
		return CodexToolResult{}, err
	}
	return CodexToolResult{Tool: call.Tool, Replay: &replay}, nil
}

func (r *CodexToolRegistry) rowsFromSearchResults(ctx context.Context, tenantID, kbID string, results []kb.SearchResult, rowsLimit int) []CodexToolRow {
	rows := make([]CodexToolRow, 0, rowsLimit)
	for _, result := range results {
		chunk := result.Chunk
		if chunk.TenantID != tenantID || chunk.KnowledgeBaseID != kbID {
			continue
		}
		row := rowFromChunk(chunk, result.Score, result.Rank, result.From)
		if r.chunks != nil && row.ChunkContentHash == "" {
			if source, found := findSourceChunk(ctx, r.chunks, tenantID, kbID, chunk.ID); found {
				row.DocVersion = source.DocVersion
				row.ChunkContentHash = source.ChunkContentHash
			}
		}
		rows = append(rows, row)
		if len(rows) >= rowsLimit {
			break
		}
	}
	return rows
}

func (r *CodexToolRegistry) reserveStep(call CodexToolCall) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := call.SessionID
	if key == "" {
		key = call.TenantID + "\x00" + call.KBID
	}
	next := r.steps[key] + 1
	if r.maxSteps > 0 && next > r.maxSteps {
		return r.steps[key], fmt.Errorf("%w: got %d, max %d", ErrCodexStepBudgetExceeded, next, r.maxSteps)
	}
	r.steps[key] = next
	return next, nil
}

func (r *CodexToolRegistry) reserveTenantQPS(call CodexToolCall, now time.Time) error {
	if r.quota.MaxQPSPerTenant <= 0 || call.TenantID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	second := now.UTC().Unix()
	window := r.qps[call.TenantID]
	if window.Second != second {
		window = tenantQPSWindow{Second: second}
	}
	if window.Count >= r.quota.MaxQPSPerTenant {
		r.qps[call.TenantID] = window
		return fmt.Errorf("%w: tenant %q qps %d, max %d", ErrCodexQuotaExceeded, call.TenantID, window.Count+1, r.quota.MaxQPSPerTenant)
	}
	window.Count++
	r.qps[call.TenantID] = window
	return nil
}

func (r *CodexToolRegistry) rowsLimit(call CodexToolCall) (int, error) {
	limit := firstPositive(call.MaxRows, call.TopK, r.defaultRows)
	if r.quota.MaxRowsPerCall > 0 && limit > r.quota.MaxRowsPerCall {
		return 0, fmt.Errorf("%w: rows %d, max %d", ErrCodexQuotaExceeded, limit, r.quota.MaxRowsPerCall)
	}
	return limit, nil
}

func (r *CodexToolRegistry) withCallTimeout(ctx context.Context, call CodexToolCall) (context.Context, context.CancelFunc, error) {
	timeout := call.Timeout
	if timeout <= 0 {
		timeout = r.quota.MaxTimeout
	}
	if r.quota.MaxTimeout > 0 && timeout > r.quota.MaxTimeout {
		return ctx, func() {}, fmt.Errorf("%w: timeout %s, max %s", ErrCodexQuotaExceeded, timeout, r.quota.MaxTimeout)
	}
	if timeout <= 0 {
		return ctx, func() {}, nil
	}
	child, cancel := context.WithTimeout(ctx, timeout)
	return child, cancel, nil
}

func (r *CodexToolRegistry) auditCall(ctx context.Context, call CodexToolCall, started time.Time, rows, steps int, allowed bool, err error) {
	if r.audit == nil {
		return
	}
	event := CodexToolAuditEvent{
		ID:         stableID("codex_tool_audit", call.SessionID, call.TenantID, call.KBID, string(call.Tool), started.Format(time.RFC3339Nano), strconv.FormatUint(r.nextAuditSequence(), 10)),
		SessionID:  call.SessionID,
		TenantID:   call.TenantID,
		KBID:       call.KBID,
		Tool:       call.Tool,
		Rows:       rows,
		Steps:      steps,
		Allowed:    allowed,
		StartedAt:  started,
		FinishedAt: r.now(),
	}
	if err != nil {
		event.Error = err.Error()
	}
	_ = r.audit.RecordCodexToolAudit(ctx, event)
}

func (r *CodexToolRegistry) nextAuditSequence() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditSeq++
	return r.auditSeq
}

type EvalRepositoryToolLookup struct {
	Repository evalpkg.Repository
}

func (l EvalRepositoryToolLookup) LookupEvalResults(ctx context.Context, tenantID, _ string, query string, limit int) ([]CodexEvalResult, error) {
	if l.Repository == nil {
		return nil, ErrCodexToolUnavailable
	}
	detail, found, err := l.Repository.GetEvaluationDetail(ctx, tenantID, query, evalpkg.EvaluationDetailOptions{IncludeItems: true})
	if err != nil || !found {
		return nil, err
	}
	out := make([]CodexEvalResult, 0, len(detail.Items)+1)
	out = append(out, CodexEvalResult{
		RunID:     detail.Run.ID,
		Metrics:   detail.Run.Metrics,
		CreatedAt: detail.Run.CreatedAt,
	})
	for _, item := range detail.Items {
		out = append(out, CodexEvalResult{
			RunID:         item.RunID,
			DatasetItemID: item.DatasetItemID,
			Answer:        item.Answer,
			Metrics:       item.Metrics,
			CreatedAt:     detail.Run.CreatedAt,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func findSourceChunk(ctx context.Context, source kb.ChunkSource, tenantID, kbID, chunkID string) (SourceChunk, bool) {
	for _, chunk := range source.Chunks(tenantID, kbID) {
		if chunk.ID != chunkID {
			continue
		}
		got, found, err := NewChunkSourceMetadataReader().ReadSourceMetadata(ctx, tenantID, kbID, chunk)
		if err != nil {
			return SourceChunk{}, false
		}
		return got, found
	}
	return SourceChunk{}, false
}

func rowFromChunk(chunk kb.Chunk, score float64, rank int, from string) CodexToolRow {
	return CodexToolRow{
		ChunkID:          chunk.ID,
		DocID:            chunk.DocumentID,
		DocVersion:       sourceDocVersion(chunk),
		ChunkContentHash: sourceChunkContentHash(chunk),
		Text:             chunk.Content,
		Score:            score,
		Rank:             rank,
		From:             from,
		Metadata:         copyStringMap(chunk.Metadata),
	}
}

func sortedChunks(chunks []kb.Chunk) []kb.Chunk {
	out := append([]kb.Chunk(nil), chunks...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].DocumentID != out[j].DocumentID {
			return out[i].DocumentID < out[j].DocumentID
		}
		if out[i].Offset != out[j].Offset {
			return out[i].Offset < out[j].Offset
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func resultRowCount(result CodexToolResult) int {
	switch {
	case result.Replay != nil:
		return len(result.Replay.BaselineRecallResults)
	case len(result.EvalResults) > 0:
		return len(result.EvalResults)
	case len(result.ExistingItems) > 0:
		return len(result.ExistingItems)
	default:
		return len(result.Rows)
	}
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
