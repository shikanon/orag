<div align="center">

# ORAG

**A Go-native RAG service framework for ingestion, hybrid retrieval, generation, evaluation, and optimization.**

<p>
  <a href="./README.zh-CN.md"><img alt="README in Simplified Chinese" src="https://img.shields.io/badge/简体中文-F5F5F5"></a>
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DBEDFA"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/shikanon/orag?color=4e6b99"></a>
  <a href="https://github.com/shikanon/orag/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/shikanon/orag/ci.yml?branch=main&label=CI"></a>
  <a href="https://github.com/shikanon/orag/actions/workflows/security.yml"><img alt="Security" src="https://img.shields.io/github/actions/workflow/status/shikanon/orag/security.yml?branch=main&label=Security"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/shikanon/orag"><img alt="OpenSSF Scorecard" src="https://api.scorecard.dev/projects/github.com/shikanon/orag/badge"></a>
  <a href="https://www.tensorbytes.com/orag/"><img alt="Documentation" src="https://img.shields.io/badge/docs-tensorbytes.com-0B51E5"></a>
  <a href="./go.mod"><img alt="Go Version" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white"></a>
  <a href="./api/openapi.yaml"><img alt="OpenAPI" src="https://img.shields.io/badge/OpenAPI-3.x-6BA539?logo=openapiinitiative&logoColor=white"></a>
</p>

<p>
  <a href="#quick-start">Quick Start</a> ·
  <a href="#core-features">Core Features</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="https://www.tensorbytes.com/orag/">Hosted Docs</a> ·
  <a href="./docs/README.md">Docs</a> ·
  <a href="./ROADMAP_EN.md">Roadmap</a> ·
  <a href="./api/openapi.yaml">OpenAPI</a>
</p>

</div>

ORAG is a Go-native RAG service framework for building an end-to-end workflow across knowledge-base ingestion, hybrid retrieval, answer generation, evaluation, and optimization. It exposes HTTP APIs with Hertz, orchestrates the RAG pipeline with Eino Graph, and uses PostgreSQL + Qdrant as the default production-like runtime dependencies.

> ORAG requires real model provider API keys by default. Ark / Doubao is the recommended default provider. Configure `ARK_API_KEY` or `VOLCENGINE_API_KEY` before starting the service. Deterministic mock providers are only available in explicit test mode to avoid accidental mock usage in real deployments.

## Highlights

- **Go-native service stack**: Built with Go, Hertz, Eino Graph, PostgreSQL, and Qdrant, making it easy to integrate with existing backend systems.
- **Production-like defaults**: The default `qdrant_postgres` backend uses Qdrant for dense retrieval and semantic cache, and PostgreSQL for metadata, FTS sparse retrieval, traces, and evaluation results.
- **Clear API contract**: OpenAPI, curl examples, Go HTTP client examples, contract tests, and the built-in `/docs` endpoint stay aligned.
- **Evaluation-first workflow**: Datasets, evaluation runs, and profile/top-k optimization reuse the same online RAG query path to reduce offline/online drift.
- **Explicit provider boundary**: The provider registry selects vendors by capability, including chat, embedding, rerank, and multimodal parsing. Real providers require API key validation by default.

## Capability Maturity

ORAG labels each public capability as `experimental`, `beta`, or `stable` to describe its compatibility commitment. Every HTTP operation carries its own OpenAPI `x-orag-maturity` label; the `v0.1.0-beta.3` distribution being Beta does not imply that every included capability has reached `beta`. Experimental capabilities require independent validation and an explicit fallback before real-world use.

`v0.1.0-beta.1` was ORAG's first public Beta release; `v0.1.0-beta.3` is the current recommended distribution.

See the [compatibility and capability maturity policy](./docs/compatibility.md) for the full definitions, deprecation rules, and migration expectations. HTTP operations expose the same maturity through OpenAPI `x-orag-maturity`.

## Table Of Contents

- [Core Features](#core-features)
- [Capability Maturity](#capability-maturity)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Examples](#examples)
- [Configuration](#configuration)
- [Testing](#testing)
- [Documentation](#documentation)
- [Roadmap](./ROADMAP_EN.md)
- [Project Scope](#project-scope)
- [License](#license)

## Core Features

| Feature | What it does | API / Entry |
| --- | --- | --- |
| Authentication | Exchanges admin credentials for a Bearer token. Business requests derive the default tenant from the token. | `POST /v1/auth/login` |
| Knowledge bases | Creates, lists, reads, and deletes knowledge bases. Delete also cleans related documents, chunks, vector indexes, and semantic cache entries. | `/v1/knowledge-bases` |
| Document ingestion | Supports JSON text import and multipart file upload with ingestion job persistence. Parsers include basic, MinerU, and Docling. | `/documents:import`, `/documents`, `/ingestion-jobs/{id}` |
| Hybrid retrieval | Combines Qdrant dense retrieval, PostgreSQL FTS sparse retrieval, RRF fusion, and rerank. | `internal/kb`, `internal/rag` |
| RAG query | Provides JSON and SSE query APIs with answer, citations, trace, cache status, and warnings. | `POST /v1/query`, `POST /v1/query:stream` |
| Evaluation | Manages datasets, evaluation runs, rule-based metrics, and persisted evaluation results. | `/v1/datasets`, `/v1/evaluations` |
| Optimization | Runs profile/top-k grid search and tracks optimization state. | `/v1/optimizations` |
| Observability | Exposes liveness, readiness, Prometheus text metrics, structured logs, and traces. | `/healthz`, `/readyz`, `/metrics` |

## Architecture

```text
Client / curl / Go examples / SDK
        |
        v
Hertz HTTP API  ---->  Auth / Tenant / Error Model
        |
        v
Eino RAG Graph
        |
        +--> Parser / Chunker / Loader
        +--> Qdrant dense retrieval + semantic cache
        +--> PostgreSQL metadata + FTS sparse retrieval
        +--> RRF fusion + rerank
        +--> Ark / Doubao chat, embedding, multimodal adapters
        |
        v
Answer + Citations + Trace + Metrics
```

| Layer | Default implementation | Notes |
| --- | --- | --- |
| HTTP API | Hertz | The API service entry point is `cmd/orag-api`; the contract source is `api/openapi.yaml`. |
| RAG orchestration | Eino Graph | Orchestrates retrieval, rerank, generation, citations, and cache flow. |
| Vector search | Qdrant | Default collection: `orag_chunks`. |
| Semantic cache | Qdrant | Default collection: `orag_semantic_cache`, isolated by tenant, profile, query identity, and related dimensions. |
| Metadata & sparse retrieval | PostgreSQL | Stores knowledge bases, documents, chunk metadata, FTS, datasets, evaluation results, and traces. |
| Model providers | Ark / Doubao by default | The provider registry connects chat, embedding, rerank, and multimodal parsing capabilities. |
| Local runtime | Docker Compose | `make demo` starts migration, API, Console, and the no-key walkthrough; `make dev-up` starts only PostgreSQL and Qdrant. |

The default `STORAGE_BACKEND=qdrant_postgres` requires PostgreSQL and Qdrant. `STORAGE_BACKEND=memory` is only intended for local debugging, unit tests, or HTTP-layer troubleshooting, not production usage.

## Quick Start

### Prerequisites

- Docker Desktop
- `docker compose`

### Five-minute no-key walkthrough

```bash
make demo
```

This explicitly enables deterministic mocks, builds and starts PostgreSQL, Qdrant, migration, API, and Console, then completes ingestion, a cited query, trace lookup, and evaluation. The machine-readable result is written to `.orag-demo/walkthrough.json`. Open the Console at `http://localhost:3000` and the interactive API docs at `http://localhost:8080/docs`. Without a local service, browse the [hosted documentation](https://www.tensorbytes.com/orag/) and [hosted API reference](https://www.tensorbytes.com/orag/api.html); [GitHub Pages](https://shikanon.github.io/orag/) remains a mirror.

This path is for local exploration and regression checks, not a production credential template. Stop it with:

```bash
make demo-down
```

### Run locally with a real provider

This path requires Go `1.26+` and a real model provider API key, recommended: `ARK_API_KEY` or `VOLCENGINE_API_KEY`.

```bash
cp .env.example .env
make dev-up
make migrate
make run
```

`make run` starts the API service in the foreground. The default address is `http://localhost:8080`. Check the service from another terminal:

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
```

### Run only the in-memory mock API

For local HTTP checks or unit-test-style runs, explicitly enable deterministic mock providers:

```bash
ALLOW_DETERMINISTIC_MOCK=true \
LLM_CHAT_PROVIDER=mock \
LLM_EMBEDDING_PROVIDER=mock \
LLM_RERANK_PROVIDER=mock \
LLM_MULTIMODAL_PROVIDER=mock \
STORAGE_BACKEND=memory \
make run
```

### Stop services

```bash
make dev-down
```

## Examples

### Curl smoke flow

After the service starts, run the scripts in order:

```bash
examples/curl/00_login.sh
examples/curl/10_create_kb.sh
examples/curl/20_upload_doc.sh
examples/curl/25_upload_file.sh
examples/curl/30_query.sh
examples/curl/35_query_stream.sh
examples/curl/36_trace_lookup.sh
examples/curl/40_eval.sh
examples/curl/45_optimize.sh
examples/curl/50_optimize.sh
```

The scripts use `BASE_URL=http://localhost:8080` by default. Override it with `BASE_URL` if needed. Runtime state is stored in `.orag-demo/`, including the token, knowledge base ID, document ID, ingestion job ID, trace ID, dataset ID, evaluation ID, and optimization ID. Do not commit that directory.

### Public Go SDK (no key required)

Go services can import `github.com/shikanon/orag` and use the same RAG and evaluation core as the HTTP service. This example explicitly uses in-memory storage and deterministic mocks, so it needs no real key or external dependency:

```bash
go run ./examples/go/sdk
```

See the [public Go SDK guide](./docs/sdk/README.md) for configuration, errors, concurrency, and stream semantics. Keep using the HTTP/OpenAPI path demonstrated by `examples/go/basic` when you need process isolation or a non-Go client. The OpenAPI source is `api/openapi.yaml`, and the service exposes documentation at `GET /docs`.

## Configuration

`.env.example` is the local configuration template. Common variables include:

| Category | Variables |
| --- | --- |
| Server | `HOST`, `PORT`, `PUBLIC_BASE_URL` |
| Storage | `STORAGE_BACKEND`, `DATABASE_URL` |
| Qdrant | `QDRANT_HOST`, `QDRANT_GRPC_PORT`, `QDRANT_COLLECTION`, `QDRANT_SEMANTIC_CACHE_COLLECTION`, `QDRANT_AUTO_CREATE_COLLECTIONS` |
| Auth | `JWT_SECRET`, `API_KEY_PEPPER`, `ADMIN_DEFAULT_USERNAME`, `ADMIN_DEFAULT_PASSWORD`, `AUTH_TOKEN_TTL` |
| Model providers | `LLM_CHAT_PROVIDER`, `LLM_EMBEDDING_PROVIDER`, `LLM_RERANK_PROVIDER`, `LLM_MULTIMODAL_PROVIDER`, `ALLOW_DETERMINISTIC_MOCK` |
| Ark / Doubao | `ARK_API_KEY`, `VOLCENGINE_API_KEY`, `ARK_BASE_URL`, `ARK_CHAT_MODEL`, `ARK_EMBEDDING_MODEL`, `ARK_MULTIMODAL_MODEL` |
| Rerank | `LLM_RERANK_PROVIDER`, `RERANK_PROVIDER`, `ARK_RERANK_BASE_URL`, `ARK_RERANK_MODEL`, `ALIYUN_RERANK_API_KEY`, `ALIYUN_RERANK_BASE_URL`, `ALIYUN_RERANK_MODEL` |
| Other provider keys | `OPENAI_API_KEY`, `AZURE_OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `COHERE_API_KEY`, `JINA_API_KEY`, `VOYAGE_API_KEY`, and more |
| Provider base URLs | `AZURE_OPENAI_BASE_URL`, `GOOGLE_CLOUD_BASE_URL`, plus optional `<PROVIDER>_BASE_URL` overrides |
| Ingestion parser | `INGEST_PARSER_METHOD`, `MINERU_APISERVER`, `MINERU_SERVER_URL`, `MINERU_BACKEND`, `MINERU_PARSE_METHOD`, `MINERU_LANG`, `MINERU_FORMULA_ENABLE`, `MINERU_TABLE_ENABLE`, `DOCLING_SERVER_URL`, `DOCLING_TIMEOUT` |

Although `.env.example` sets `REQUIRE_EXTERNAL_PROVIDERS=false`, startup still validates API keys for selected real providers unless deterministic mock providers are explicitly enabled. `/readyz` reports `model_provider=configured` or `model_provider=mock` in explicit test mode; it does not actively call external model services.

The provider registry includes OpenAI, Azure OpenAI, Anthropic, Gemini, Google Cloud, xAI, Mistral, Cohere, DeepSeek, Moonshot, MiniMax, BaiChuan, ZHIPU-AI, Tongyi-Qianwen, VolcEngine, Tencent Hunyuan, XunFei Spark, BaiduYiyan, Xiaomi, Perplexity, Voyage AI, and Jina.

`INGEST_PARSER_METHOD=basic` is the default parser. It extracts text from plain text, HTML, and Office ZIP documents in-process. PDF, images, and embedded DOCX images can be converted into Markdown descriptions through `ARK_MULTIMODAL_MODEL`. `INGEST_PARSER_METHOD=mineru` calls a MinerU-compatible `/file_parse` service. `INGEST_PARSER_METHOD=docling` calls Docling Serve through `/v1/convert/source` or `/v1alpha/convert/source`.

## Testing

Common local verification commands:

```bash
make fmt
make vet
make test
make openapi-validate
```

The `Makefile` injects `CGO_ENABLED=0` and `GOFLAGS=-tags=stdjson,gjson` into Go commands by default. This avoids local cgo linkage issues with Hertz/Sonic native artifacts on Mac amd64 + Go 1.26. When running raw Go commands, use:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go test ./...
```

PostgreSQL + Qdrant integration tests are skipped by default. Run them explicitly with:

```bash
make test-integration-up
make test-integration
make test-integration-down
```

Real Ark smoke tests are skipped by default and only run when explicitly enabled:

```bash
LIVE_ARK_TESTS=1 ARK_API_KEY="$ARK_API_KEY" CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go test ./tests/live -v
```

## Documentation

| Document | Audience | Content |
| --- | --- | --- |
| `docs/README.md` | New contributors | Documentation map, recommended reading paths, and maintenance rules. |
| `docs/getting-started/` | New developers and API smoke users | Local startup, dependency notes, API smoke flow, and state directory usage. |
| `docs/api/` | API consumers, SDK developers, frontend developers | Authentication, error model, knowledge bases, ingestion, query, and SSE. |
| `docs/architecture/` | Backend developers and architecture reviewers | Module map, runtime dependencies, and RAG pipeline. |
| `docs/evaluation/` | Evaluation, algorithm, and quality owners | Dataset structure, rule-based metrics, LLM-as-Judge/QAG, and the goal-driven optimizer. |
| `docs/operations/` | Operators, SREs, deployment owners | Runtime dependencies, health checks, metrics, configuration security, and troubleshooting. |

## Project Scope

- ES/Neo4j are not started by default. The current real backend is PostgreSQL + Qdrant.
- Evaluation keeps deterministic rule-based metrics as the default baseline. Requests that provide `judge` or `qag` configuration enable LLM-as-Judge, QAG details, and calibration-related metrics.
- `/readyz` does not actively call external model services. `model_provider=configured` only means required keys for the selected providers are present.
- `STORAGE_BACKEND=memory` is for local debugging and tests only, not production.
- MinerU and Docling are integrated as remote parsing services. ORAG does not start their Python runtimes inside the API process.

## License

This project is licensed under the terms of the [LICENSE](./LICENSE) file.
