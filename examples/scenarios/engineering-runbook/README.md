# Engineering Runbook Scenario

## Role

Engineering and SRE teams use ORAG to search runbooks, incident notes, architecture docs, and API references during debugging.

## Why Use ORAG

Use ORAG when engineers need a fast answer grounded in internal operational knowledge and enough trace evidence to explain why a diagnostic answer was produced.

## When To Use It

Choose this scenario for incident triage, on-call handoff, architecture lookup, API troubleshooting, and postmortem knowledge reuse.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides runbook and escalation source material for the runnable demo.
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
./examples/scenarios/engineering-runbook/run.sh
```

For read-only operational checks, inspect `examples/skills/self-check-diagnose-ops.md`.

## Demo Implementation

- `run.sh` imports `demo-data.md` through `DOC_CONTENT`.
- The demo creates a knowledge base, imports runbook material, asks a latency-triage question, and looks up the saved trace.
- Override `BASE_URL`, `STATE_DIR`, `QUERY`, or `DOC_CONTENT` to point the demo at a different service or runbook.

## Reused Assets

- `examples/curl/05_health_ready.sh`
- `examples/curl/00_login.sh`
- `examples/curl/10_create_kb.sh`
- `examples/curl/20_upload_doc.sh`
- `examples/curl/30_query.sh`
- `examples/curl/36_trace_lookup.sh`
- `examples/skills/self-check-diagnose-ops.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
