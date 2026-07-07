# Evaluation and Optimization Scenario

## Role

Product, platform, and ML quality teams use ORAG evaluation results to compare retrieval behavior before changing production settings.

## Why Use ORAG

Use ORAG evaluation when retrieval quality needs repeatable metrics instead of one-off manual inspection.

## When To Use It

Choose this before changing profiles, top-k, ranking weights, or dataset content in production.

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
./examples/curl/40_eval.sh
./examples/curl/45_optimize.sh
```

## Reused Assets

- `examples/curl/40_eval.sh`
- `examples/curl/45_optimize.sh`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
