# Product Team Scenario

## Role

Product teams use ORAG to validate whether a knowledge assistant answers the right questions with enough quality evidence to support release decisions.

## Why Use ORAG

Use ORAG when product managers need to compare answer quality, inspect citations, collect trace evidence, and tune retrieval settings before shipping an AI feature.

## When To Use It

Choose this scenario for feature discovery, launch readiness reviews, prompt or retrieval experiments, and product-quality regression checks.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides launch-readiness review material for the runnable demo.
- `run.sh` loads `demo-data.md`, sets role-specific query and evaluation variables, and runs the maintained service scripts.
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
./examples/scenarios/product-team/run.sh
```

## Demo Implementation

- `run.sh` imports `demo-data.md` through `DOC_CONTENT`.
- The demo creates a reviewable answer, runs evaluation, and runs optimization for launch-readiness evidence.
- Override `BASE_URL`, `STATE_DIR`, `QUERY`, `EVAL_QUERY`, or `GROUND_TRUTH` to adapt the product review.

## Reused Assets

- `examples/curl/05_health_ready.sh`
- `examples/curl/00_login.sh`
- `examples/curl/10_create_kb.sh`
- `examples/curl/20_upload_doc.sh`
- `examples/curl/30_query.sh`
- `examples/curl/40_eval.sh`
- `examples/curl/45_optimize.sh`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
