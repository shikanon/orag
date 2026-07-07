# Platform Team Scenario

## Role

Platform teams use ORAG as a shared RAG capability layer for multiple applications, tenants, and agent clients.

## Why Use ORAG

Use ORAG when a platform needs one maintained service for authentication, knowledge-base lifecycle, ingestion, query, trace, evaluation, optimization, and generated agent-facing assets.

## When To Use It

Choose this scenario when building an internal AI platform, offering RAG as a service to business teams, or validating that API, MCP, and Skill assets stay aligned.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides shared-service readiness material for the runnable demo.
- `run.sh` loads `demo-data.md`, sets role-specific query and evaluation variables, and runs the maintained service plus agent-sync checks.
- `expected-output.md` lists the observable success signals.
- Commands below reference maintained examples instead of duplicating raw API calls.

## Run

From the repository root, start the API first when the scenario uses service-mode curl scripts:

```sh
./scripts/dev-up.sh
make migrate
make run
./scripts/wait-ready.sh
```

Then run the scenario demo:

```sh
./examples/scenarios/platform-team/run.sh
```

## Demo Implementation

- `run.sh` imports `demo-data.md` through `DOC_CONTENT`.
- The demo validates health, auth, ingestion, query, trace lookup, evaluation, optimization, and generated agent asset sync.
- Override `BASE_URL`, `STATE_DIR`, `QUERY`, `EVAL_QUERY`, or `GROUND_TRUTH` to adapt the platform readiness smoke.

## Reused Assets

- `examples/curl/05_health_ready.sh`
- `examples/curl/00_login.sh`
- `examples/curl/10_create_kb.sh`
- `examples/curl/20_upload_doc.sh`
- `examples/curl/30_query.sh`
- `examples/curl/36_trace_lookup.sh`
- `examples/curl/40_eval.sh`
- `examples/curl/45_optimize.sh`
- `examples/mcp/README.md`
- `examples/skills/README.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
