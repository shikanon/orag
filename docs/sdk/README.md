# Public Go SDK (Beta)

ORAG provides an embedded Go SDK at `github.com/shikanon/orag`. It runs the same project, API key, ingestion, RAG, dataset, evaluation, readiness, and trace services used by the HTTP API, but exposes only public DTOs. No `internal/*` package appears in the caller contract.

The SDK is currently **beta**. Its core workflow is tested from a standalone downstream Go module, but pre-1.0 compatibility rules still apply. The HTTP API remains the appropriate boundary when clients are not Go programs or need process isolation and centralized authentication.

## Install

```bash
go get github.com/shikanon/orag@v0.1.0-beta.2
```

The SDK currently follows the repository Go toolchain declared in `go.mod`.

## No-key walkthrough

`MockConfig` explicitly enables deterministic mock models and in-memory storage. It needs no PostgreSQL, Qdrant, network access, or real provider API key:

```go
ctx := context.Background()
client, err := orag.New(ctx, orag.MockConfig())
if err != nil {
    return err
}
defer client.Close()

project, err := client.CreateProject(ctx, orag.CreateProjectRequest{Name: "walkthrough"})
if err != nil {
    return err
}

knowledgeBase, err := client.CreateKnowledgeBase(ctx, orag.CreateKnowledgeBaseRequest{
    ProjectID: project.ID,
    Name: "walkthrough knowledge",
})
if err != nil {
    return err
}

_, err = client.IngestText(ctx, orag.IngestTextRequest{
    KnowledgeBaseID: knowledgeBase.ID,
    Name: "hello.txt",
    Text: "ORAG is a Go-native RAG service.",
})
if err != nil {
    return err
}

response, err := client.Query(ctx, orag.QueryRequest{
    KnowledgeBaseID: knowledgeBase.ID,
    Query: "What is ORAG?",
})
```

Run the complete ingestion, query, trace, dataset, and evaluation example:

```bash
go run ./examples/go/sdk
```

Mock output is deterministic test data. Do not present it as a real model result or enable deterministic mocks in production.

## Configuration

Choose one constructor:

- `New(ctx, config)` accepts an explicit `Config` and does not read ambient environment variables. Start with `DefaultConfig`, then set storage endpoints and provider credentials.
- `NewFromEnv(ctx)` uses the same environment loader as `cmd/orag-api`; it is useful when an embedded process intentionally shares the service deployment convention.
- `New(ctx, MockConfig())` is the dependency-free local/test path.

Example explicit production-oriented configuration:

```go
cfg := orag.DefaultConfig()
cfg.TenantID = "tenant_acme"
cfg.Storage.DatabaseURL = os.Getenv("DATABASE_URL")
cfg.Storage.QdrantHost = os.Getenv("QDRANT_HOST")
cfg.Storage.QdrantAPIKey = os.Getenv("QDRANT_API_KEY")
cfg.Models.APIKeys["volcengine"] = os.Getenv("ARK_API_KEY")

client, err := orag.New(ctx, cfg)
```

Credentials belong in a secret manager or process environment, never in source control. `DefaultConfig` opens PostgreSQL and Qdrant resources and validates selected real model providers; call `Close` during shutdown.

## Core workflow

The beta surface covers:

- projects: `CreateProject`, `ListProjects`, `GetProject`, `UpdateProject`;
- API keys: `CreateAPIKey`, `ListAPIKeys`, `RotateAPIKey`, `RevokeAPIKey`, `AuthenticateAPIKey`;
- knowledge bases: `CreateKnowledgeBase`, `ListKnowledgeBases`, `GetKnowledgeBase`, `DeleteKnowledgeBase`;
- ingestion: `IngestText`, `IngestFile`, `GetIngestionJob`;
- retrieval and generation: `Query`, `StreamQuery`;
- evaluation: `CreateDataset`, `AddDatasetItem`, `RunEvaluation`, `GetEvaluation`;
- release control: `CreatePipelineVersion`, `ListPipelineVersions`, `ValidatePipelineVersion`, `ListEnvironments`, `ListReleases`, `Promote`, `Rollback`;
- operations: `Readiness`, `GetTrace`, `ListTraces`.
- local reproducibility: `RunMockPerformanceBaseline` produces a validated performance-baseline JSON value from a fixed no-key workload.

Requests use the client tenant unless `TenantID` is explicitly set on the request. Treat a client as tenant-scoped and avoid sharing it across unrelated trust boundaries.

`CreateAPIKey` and `RotateAPIKey` return the complete secret exactly once. Rotation atomically creates the successor and immediately revokes the source, so distribute the replacement before changing downstream callers. Store secrets in a secret manager immediately; do not log them. `ListAPIKeys` returns metadata only and its public type has no secret or hash field. Project-scoped keys use `RoleProjectEditor` or `RoleProjectViewer`; tenant-wide administration uses `RoleTenantAdmin`. Embedded applications can verify a presented secret with `AuthenticateAPIKey`; service deployments normally let the HTTP authentication middleware do that work.

## Typed stream semantics

`StreamQuery` returns `<-chan QueryEvent` and emits:

1. `QueryEventResponse` with one complete `QueryResponse`;
2. `QueryEventDone` and channel closure; or
3. one terminal `QueryEventError` and channel closure.

The beta SDK computes a full answer before emitting the response. It does **not** claim token-level generation streaming. Cancel the context to stop the operation.

## Errors and concurrency

Operations return `*orag.Error` with a stable `Code`, operation/resource metadata, retry guidance, and the preserved cause. Use `errors.Is` for categories and `errors.As` for details:

```go
if errors.Is(err, orag.ErrNotFound) {
    // Handle a missing resource.
}
var sdkErr *orag.Error
if errors.As(err, &sdkErr) && sdkErr.Retryable {
    // Apply a bounded retry policy.
}
```

`Client` methods are safe for concurrent use. `Close` is nil-safe and idempotent; callers must not begin new operations after shutdown.

## Compatibility and checks

The SDK is beta in `v0.1.0-beta.2`. Breaking pre-1.0 changes are recorded in `CHANGELOG.md` with migration guidance when practical. Verify an upgrade with:

```bash
make sdk-check
```

That gate compiles external-package tests, scans exported documentation for `internal/*` type leaks, and runs tests plus `go vet` in a standalone consumer module.

For a local performance regression baseline, use `make benchmark-report-run BENCHMARK_REPORT=.tmp/performance-baseline.json` followed by `make benchmark-report-verify BENCHMARK_REPORT=.tmp/performance-baseline.json`. This is deterministic mock evidence for a disclosed local environment, not a production or cross-hardware performance claim.

The standalone consumer resolves the published module directly at
`github.com/shikanon/orag v0.1.0-beta.2`; it intentionally has no `replace`
directive pointing back to this repository. This keeps the release check honest:
it proves that a downstream module can download and use the tagged SDK without
access to the source checkout.

## Known beta limitations

- Model-judge, QAG, pairwise judge, holdout-gate, optimizer, and advanced ingestion controls remain HTTP/control-plane-first APIs.
- `StreamQuery` is event streaming, not token streaming.
- Embedded mode owns database/vector clients in the current process; the application must call `Close` and manage process-level resource limits.
- In-memory mock storage is ephemeral and not a production persistence option.
