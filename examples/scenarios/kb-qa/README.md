# Knowledge-base Q&A Scenario

## Role

Application teams use ORAG to build private knowledge-base assistants over imported documents.

## Why Use ORAG

Use ORAG when a product needs answers grounded in private documents with citations and traceability.

## When To Use It

Start here for support bots, internal policy search, onboarding assistants, or any workflow that imports source material before asking questions.

## Scenario Files

- `sample-input.md` describes the representative user input.
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

Then run or inspect the scenario-specific commands:

```sh
./examples/curl/05_health_ready.sh
./examples/curl/00_login.sh
./examples/curl/10_create_kb.sh
./examples/curl/20_upload_doc.sh
./examples/curl/25_upload_file.sh
./examples/curl/30_query.sh
```

## Reused Assets

- `examples/curl/05_health_ready.sh`
- `examples/curl/00_login.sh`
- `examples/curl/10_create_kb.sh`
- `examples/curl/20_upload_doc.sh`
- `examples/curl/25_upload_file.sh`
- `examples/curl/30_query.sh`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
