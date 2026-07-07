# Customer Support Scenario

## Role

Customer support and pre-sales teams use ORAG to answer product, policy, and troubleshooting questions from maintained knowledge sources.

## Why Use ORAG

Use ORAG when support agents need grounded answers with citations, trace IDs, and repeatable evidence instead of manually searching product docs during every conversation.

## When To Use It

Choose this scenario for support consoles, internal service desks, pre-sales assistants, and FAQ bots that must import product material before answering customer questions.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides support-policy and escalation source material for the runnable demo.
- `run.sh` loads `demo-data.md`, sets role-specific ORAG example variables, and runs the maintained service scripts.
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
./examples/scenarios/customer-support/run.sh
```

## Demo Implementation

- `run.sh` imports `demo-data.md` through `DOC_CONTENT` and uploads the same file through `UPLOAD_PATH`.
- The demo creates a knowledge base, imports support material, asks a customer-escalation question, and looks up the saved trace.
- Override `BASE_URL`, `STATE_DIR`, `QUERY`, or `DOC_CONTENT` to point the demo at a different service or support article.

## Reused Assets

- `examples/curl/05_health_ready.sh`
- `examples/curl/00_login.sh`
- `examples/curl/10_create_kb.sh`
- `examples/curl/20_upload_doc.sh`
- `examples/curl/25_upload_file.sh`
- `examples/curl/30_query.sh`
- `examples/curl/36_trace_lookup.sh`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
