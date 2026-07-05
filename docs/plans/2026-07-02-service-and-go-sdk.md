# Service and Go SDK Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** ORAG 同时支持以 Docker 服务方式对外提供 HTTP API，也支持作为 Go SDK 被其他 Go 模块直接导入使用。

**Architecture:** 保留现有 [`cmd/orag-api`](../../cmd/orag-api)、[`internal/http`](../../internal/http) 和 [`deployments/`](../../deployments) 作为服务化入口；新增模块根包 `github.com/shikanon/orag` 作为公共 SDK facade，封装 [`internal/app`](../../internal/app)、RAG 查询、文档入库、trace 查询和关闭资源。HTTP 服务后续只负责协议适配，核心初始化和业务能力由 SDK 复用，避免服务模式与 SDK 模式产生两套逻辑。

**Tech Stack:** Go 1.26, CloudWeGo Hertz, PostgreSQL/pgx, Qdrant, Docker Compose, existing `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson` test flags.

---

### Task 1: 定义公共 SDK API 边界

**Files:**
- Create: `sdk_test.go`
- Create: `orag.go`
- Create: `sdk_types.go`
- Modify: [`README.md`](../../README.md)
- Modify: [`docs/development.md`](../development.md)

**Step 1: Write the failing external-package SDK test**

Create `sdk_test.go` with `package orag_test` so the test behaves like a real downstream Go module and cannot import `internal/*`.

```go
package orag_test

import (
	"context"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestSDKCanCreateMemoryClientAndQuery(t *testing.T) {
	t.Parallel()

	client, err := orag.NewClient(context.Background(), orag.Config{
		StorageBackend: "memory",
		ChatModel: orag.StaticChatModel{
			Answer: "基于上下文的回答 [chunk:chk_test]",
			Embedding: []float64{0.1, 0.2, 0.3},
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	resp, err := client.Query(context.Background(), orag.QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "hello",
		TraceID:         "trace_sdk_query",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if resp.TraceID != "trace_sdk_query" {
		t.Fatalf("TraceID = %q, want trace_sdk_query", resp.TraceID)
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . -run TestSDKCanCreateMemoryClientAndQuery -v
```

Expected: FAIL because root package `orag`, `Config`, `NewClient`, `QueryRequest`, and `StaticChatModel` do not exist.

**Step 3: Add the minimal public SDK facade**

Create `orag.go` with package `orag`.

```go
package orag

import (
	"context"
	"log/slog"

	"github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/platform/logger"
	"github.com/shikanon/orag/internal/rag"
)

type Client struct {
	app *app.App
}

func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	internalCfg := cfg.toInternal()
	logg := cfg.Logger
	if logg == nil {
		logg = logger.New(internalCfg.Server.Debug)
	}
	core, err := app.New(ctx, internalCfg, logg)
	if err != nil {
		return nil, err
	}
	if cfg.ChatModel != nil {
		core.RAG.Model = cfg.ChatModel.internalArkClient()
	}
	return &Client{app: core}, nil
}

func NewClientFromEnv(ctx context.Context) (*Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	core, err := app.New(ctx, cfg, logger.New(cfg.Server.Debug))
	if err != nil {
		return nil, err
	}
	return &Client{app: core}, nil
}

func (c *Client) Close() error {
	if c == nil || c.app == nil {
		return nil
	}
	return c.app.Close()
}

func (c *Client) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	resp, err := c.app.RAG.Query(ctx, rag.QueryRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		TopK:            req.TopK,
		Profile:         rag.Profile(req.Profile),
		TraceID:         req.TraceID,
	})
	if err != nil {
		return QueryResponse{}, err
	}
	return fromRAGResponse(resp), nil
}
```

Create `sdk_types.go` with public DTOs and config conversion. Keep public types in package `orag`; do not expose `internal/*` types in method signatures.

```go
package orag

import (
	"log/slog"
	"time"

	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/rag"
)

type Config struct {
	StorageBackend string
	DatabaseURL    string
	QdrantHost     string
	QdrantGRPCPort int
	ArkAPIKey      string
	ChatModelName  string
	EmbeddingModel string
	Logger         *slog.Logger
	ChatModel      Model
}

type Model interface {
	internalArkClient() *ark.Client
}

type QueryRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Query           string
	TopK            int
	Profile         string
	TraceID         string
}

type QueryResponse struct {
	Answer      string
	TraceID     string
	CacheStatus string
	Profile     string
	Warnings    []string
	LatencyMS   int64
	CreatedAt   time.Time
	Citations   []Citation
}

type Citation struct {
	ChunkID   string
	DocumentID string
	SourceURI string
	Section   string
}

func (c Config) toInternal() config.Config {
	cfg, _ := config.Load()
	if c.StorageBackend != "" {
		cfg.Storage.Backend = c.StorageBackend
	}
	if c.DatabaseURL != "" {
		cfg.Database.URL = c.DatabaseURL
	}
	if c.QdrantHost != "" {
		cfg.Qdrant.Host = c.QdrantHost
	}
	if c.QdrantGRPCPort != 0 {
		cfg.Qdrant.GRPCPort = c.QdrantGRPCPort
	}
	if c.ArkAPIKey != "" {
		cfg.Ark.APIKey = c.ArkAPIKey
	}
	if c.ChatModelName != "" {
		cfg.Ark.ChatModel = c.ChatModelName
	}
	if c.EmbeddingModel != "" {
		cfg.Ark.EmbeddingModel = c.EmbeddingModel
	}
	return cfg
}

func fromRAGResponse(resp rag.QueryResponse) QueryResponse {
	citations := make([]Citation, len(resp.Citations))
	for i := range resp.Citations {
		citations[i] = Citation{
			ChunkID:    resp.Citations[i].ChunkID,
			DocumentID: resp.Citations[i].DocumentID,
			SourceURI:  resp.Citations[i].SourceURI,
			Section:    resp.Citations[i].Section,
		}
	}
	return QueryResponse{
		Answer:      resp.Answer,
		TraceID:     resp.TraceID,
		CacheStatus: resp.CacheStatus,
		Profile:     string(resp.Profile),
		Warnings:    append([]string(nil), resp.Warnings...),
		LatencyMS:   resp.LatencyMS,
		CreatedAt:   resp.CreatedAt,
		Citations:   citations,
	}
}
```

**Step 4: Run test to verify the next missing dependency**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . -run TestSDKCanCreateMemoryClientAndQuery -v
```

Expected: FAIL because model injection needs a public test model and [`internal/app`](../../internal/app) currently constructs a concrete Ark client too early.

**Step 5: Commit**

```bash
git add sdk_test.go orag.go sdk_types.go
git commit -m "feat: define public go sdk facade"
```

---

### Task 2: Make app construction SDK-friendly

**Files:**
- Modify: [`internal/app/app.go`](../../internal/app/app.go)
- Create: `sdk_model_test.go`
- Modify: `orag.go`
- Modify: `sdk_types.go`

**Step 1: Write the failing model injection test**

Create `sdk_model_test.go`.

```go
package orag_test

import (
	"context"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestSDKUsesInjectedModelWithoutExternalArk(t *testing.T) {
	t.Parallel()

	model := &orag.StaticModel{
		Answer:    "sdk answer [chunk:chk_1]",
		Embedding: []float64{0.1, 0.2, 0.3},
	}
	client, err := orag.NewClient(context.Background(), orag.Config{
		StorageBackend: "memory",
		Model:          model,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	_, err = client.Query(context.Background(), orag.QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "who are you",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if model.ChatCalls == 0 || model.EmbedCalls == 0 {
		t.Fatalf("model was not used, chat=%d embed=%d", model.ChatCalls, model.EmbedCalls)
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . -run TestSDKUsesInjectedModelWithoutExternalArk -v
```

Expected: FAIL because SDK model injection is not supported yet.

**Step 3: Introduce app options**

Modify [`internal/app/app.go`](../../internal/app/app.go) so app initialization accepts optional dependencies without exposing them publicly.

```go
type Option func(*options)

type options struct {
	model rag.Model
}

func WithModel(model rag.Model) Option {
	return func(o *options) {
		o.model = model
	}
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger, opts ...Option) (*App, error) {
	var options options
	for _, opt := range opts {
		opt(&options)
	}
	model := options.model
	if model == nil {
		model = ark.NewClient(...)
	}
	...
}
```

Also change `internal/rag.Service.Model` from `*ark.Client` to a small interface if it is not already an interface:

```go
type Model interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
	Chat(ctx context.Context, messages []ark.ChatMessage) (string, error)
}
```

**Step 4: Add public `StaticModel` for SDK examples and tests**

In `sdk_types.go`:

```go
type StaticModel struct {
	Answer     string
	Embedding  []float64
	ChatCalls  int
	EmbedCalls int
}

func (m *StaticModel) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	m.EmbedCalls++
	out := make([][]float64, len(texts))
	for i := range out {
		out[i] = append([]float64(nil), m.Embedding...)
	}
	return out, nil
}

func (m *StaticModel) Chat(ctx context.Context, messages []ark.ChatMessage) (string, error) {
	m.ChatCalls++
	return m.Answer, nil
}
```

**Step 5: Run tests**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . ./internal/app ./internal/rag -v
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/app/app.go internal/rag/service.go orag.go sdk_types.go sdk_model_test.go
git commit -m "feat: support sdk dependency injection"
```

---

### Task 3: Add SDK ingestion and trace APIs

**Files:**
- Modify: `orag.go`
- Modify: `sdk_types.go`
- Create: `sdk_ingest_trace_test.go`
- Modify: [`docs/api.md`](../api.md)

**Step 1: Write the failing SDK ingestion and trace test**

Create `sdk_ingest_trace_test.go`.

```go
package orag_test

import (
	"context"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestSDKIngestQueryAndTraceLookup(t *testing.T) {
	t.Parallel()

	client, err := orag.NewClient(context.Background(), orag.Config{
		StorageBackend: "memory",
		Model: &orag.StaticModel{
			Answer:    "ORAG supports SDK usage [chunk:chk_1]",
			Embedding: []float64{0.1, 0.2, 0.3},
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	ingested, err := client.IngestText(context.Background(), orag.IngestTextRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Name:            "sdk.md",
		Text:            "ORAG can be embedded as a Go SDK.",
	})
	if err != nil {
		t.Fatalf("IngestText() error = %v", err)
	}
	if ingested.JobID == "" || ingested.ChunkCount == 0 {
		t.Fatalf("unexpected ingest result: %#v", ingested)
	}

	resp, err := client.Query(context.Background(), orag.QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "SDK?",
		TraceID:         "trace_sdk_ingest_query",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if resp.TraceID != "trace_sdk_ingest_query" {
		t.Fatalf("trace id = %q", resp.TraceID)
	}

	trace, found, err := client.GetTrace(context.Background(), "trace_sdk_ingest_query")
	if err != nil {
		t.Fatalf("GetTrace() error = %v", err)
	}
	if !found || trace.TraceID != "trace_sdk_ingest_query" {
		t.Fatalf("trace found=%v trace=%#v", found, trace)
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . -run TestSDKIngestQueryAndTraceLookup -v
```

Expected: FAIL because SDK ingestion and trace methods do not exist.

**Step 3: Add SDK ingestion and trace methods**

In `orag.go`:

```go
func (c *Client) IngestText(ctx context.Context, req IngestTextRequest) (IngestResult, error) {
	result, err := c.app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		SourceURI:       req.SourceURI,
		Name:            req.Name,
		Content:         []byte(req.Text),
	})
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{
		DocumentID: result.Document.ID,
		JobID:      result.Job.ID,
		ChunkCount: result.Job.ChunkCount,
		Status:     string(result.Job.Status),
	}, nil
}

func (c *Client) GetTrace(ctx context.Context, traceID string) (TraceRecord, bool, error) {
	trace, found, err := c.app.Traces.GetTrace(ctx, traceID)
	if err != nil || !found {
		return TraceRecord{}, found, err
	}
	return fromTraceRecord(trace), true, nil
}

func (c *Client) ListTraces(ctx context.Context, filter TraceListFilter) ([]TraceRecord, error) {
	traces, err := c.app.Traces.ListTraces(ctx, toInternalTraceFilter(filter))
	if err != nil {
		return nil, err
	}
	out := make([]TraceRecord, len(traces))
	for i := range traces {
		out[i] = fromTraceRecord(traces[i])
	}
	return out, nil
}
```

In `sdk_types.go`, add public request/response structs: `IngestTextRequest`, `IngestResult`, `TraceRecord`, `TraceNodeSpan`, `TraceListFilter`, `TraceNodeStat`.

**Step 4: Run tests**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . ./internal/app ./internal/graph -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add orag.go sdk_types.go sdk_ingest_trace_test.go docs/api.md
git commit -m "feat: expose ingest and trace through go sdk"
```

---

### Task 4: Keep Docker service mode as first-class supported mode

**Files:**
- Modify: [`cmd/orag-api/main.go`](../../cmd/orag-api/main.go)
- Modify: [`deployments/Dockerfile`](../../deployments/Dockerfile)
- Modify: [`deployments/docker-compose.yml`](../../deployments/docker-compose.yml)
- Modify: [`README.md`](../../README.md)
- Create: `tests/contract/service_mode_test.go`

**Step 1: Write a service-mode contract test**

Create `tests/contract/service_mode_test.go`.

```go
package contract

import (
	"os"
	"strings"
	"testing"
)

func TestServiceModeArtifactsExist(t *testing.T) {
	dockerfile, err := os.ReadFile("../../deployments/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "cmd/orag-api") {
		t.Fatalf("Dockerfile must build service binary from cmd/orag-api")
	}

	compose, err := os.ReadFile("../../deployments/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"orag-api", "postgres", "qdrant", "8080"} {
		if !strings.Contains(string(compose), want) {
			t.Fatalf("docker-compose.yml missing %q", want)
		}
	}
}
```

**Step 2: Run test**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestServiceModeArtifactsExist -v
```

Expected: PASS if current Docker service artifacts remain valid, otherwise FAIL with missing artifact details.

**Step 3: Make [`cmd/orag-api`](../../cmd/orag-api) use the SDK env constructor if practical**

Modify [`cmd/orag-api/main.go`](../../cmd/orag-api/main.go) only if it reduces duplication. Keep the HTTP server using `internal/http.NewServer` because protocol wiring remains internal.

```go
sdkClient, err := orag.NewClientFromEnv(context.Background())
...
httpserver.NewServer(sdkClient.InternalApp()).Hertz().Spin()
```

If exposing `InternalApp()` would leak internals, keep [`cmd/orag-api`](../../cmd/orag-api) on `internal/app.New` and document that service and SDK share the same app constructor beneath the package boundary.

**Step 4: Verify Docker build path**

Run:

```bash
docker build -f deployments/Dockerfile -t orag-api:local .
```

Expected: PASS when Docker is available. If Docker/network is unavailable, record the environment failure and still run Go contract test from Step 2.

**Step 5: Commit**

```bash
git add cmd/orag-api/main.go deployments/Dockerfile deployments/docker-compose.yml README.md tests/contract/service_mode_test.go
git commit -m "test: lock service mode docker artifacts"
```

---

### Task 5: Document dual usage with runnable examples

**Files:**
- Create: `examples/sdk/basic/main.go`
- Modify: [`README.md`](../../README.md)
- Modify: [`docs/getting-started/README.md`](../getting-started/README.md)
- Modify: [`docs/development.md`](../development.md)
- Modify: [`docs/architecture/README.md`](../architecture/README.md)

**Step 1: Add SDK example**

Create `examples/sdk/basic/main.go`.

```go
package main

import (
	"context"
	"fmt"
	"log"

	orag "github.com/shikanon/orag"
)

func main() {
	ctx := context.Background()
	client, err := orag.NewClient(ctx, orag.Config{StorageBackend: "memory"})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	_, err = client.IngestText(ctx, orag.IngestTextRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Name:            "hello.md",
		Text:            "ORAG can run as a Docker service or be embedded as a Go SDK.",
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Query(ctx, orag.QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "How can ORAG be used?",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Answer)
}
```

**Step 2: Add docs section for two supported modes**

Update README and getting-started docs with:

```markdown
## 使用方式

- 服务模式：使用 Docker Compose 启动 `orag-api`，通过 HTTP/OpenAPI、curl 或业务服务调用。
- SDK 模式：在 Go 项目中 `import "github.com/shikanon/orag"`，直接创建 `orag.Client` 执行入库、查询和 trace 读取。
```

Add commands:

```bash
docker compose -f deployments/docker-compose.yml up --build
go run ./examples/sdk/basic
```

**Step 3: Run example compilation**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./examples/sdk/basic
```

Expected: PASS.

**Step 4: Commit**

```bash
git add examples/sdk/basic/main.go README.md docs/getting-started/README.md docs/development.md docs/architecture/README.md
git commit -m "docs: document docker service and go sdk usage"
```

---

### Task 6: Final verification

**Files:**
- Modify: `.trae/specs/<change-id>/tasks.md` if this work is tracked in a spec
- Modify: `.trae/specs/<change-id>/checklist.md` if this work is tracked in a spec
- Modify: `.trae/specs/<change-id>/progress.md` if this work is tracked in a spec

**Step 1: Run formatting**

Run:

```bash
make fmt
```

Expected: PASS and only Go formatting changes.

**Step 2: Run full Go tests**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./...
```

Expected: PASS.

**Step 3: Run vet**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go vet ./...
```

Expected: PASS.

**Step 4: Run contract tests**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -v
```

Expected: PASS.

**Step 5: Run Docker build when available**

Run:

```bash
docker build -f deployments/Dockerfile -t orag-api:local .
```

Expected: PASS. If Docker daemon or network is unavailable in the environment, record the exact failure and keep Step 2-4 as mandatory blockers.

**Step 6: Commit**

```bash
git add .
git commit -m "feat: support docker service and go sdk modes"
```
