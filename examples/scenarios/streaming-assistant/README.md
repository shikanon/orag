# Streaming Assistant Scenario

## Role

Frontend and assistant teams use ORAG streaming to power low-latency chat experiences.

## Why Use ORAG

Use ORAG streaming when an assistant UI should display partial answer chunks while retrieval and generation are still running.

## When To Use It

Choose this path for chat surfaces, support consoles, or IDE assistants that need low perceived latency.

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
./examples/curl/35_query_stream.sh
```

## Reused Assets

- `examples/curl/35_query_stream.sh`
- `examples/curl/20_upload_doc.sh`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
