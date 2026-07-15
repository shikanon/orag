# Multimodal Assets Scenario

## Role

Multimodal application developers, QA engineers, and platform teams use this scenario to keep a shared remote fixture set for image, audio, video, long-video upload, and script-document paths.

## Why Use ORAG

Use ORAG when multimodal retrieval or upload flows need repeatable test inputs across media types without committing binary fixtures into the repository.

## When To Use It

Choose this scenario when validating upload handlers, document parsing, media metadata extraction, multimodal prompt grounding, or cross-modal retrieval preparation. The long video is large and should only be used for upload-boundary testing, not normal smoke runs.

## Scenario Files

- `sample-input.md` describes the representative multimodal test request.
- `demo-data.md` records the exact remote asset URLs and intended usage.
- `main.go` validates that the manifest uses HTTPS URLs with filenames and prints per-asset metadata.
- `expected-output.md` lists the observable success signals.
- Commands below use a Go manifest demo and do not download the remote assets.

## Run

From the repository root, run the Go scenario demo:

```sh
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/scenarios/multimodal-assets
```

## Demo Implementation

- `main.go` stores the seven approved remote test URLs in code so contract tests can catch accidental drift.
- The demo validates HTTPS scheme and filename presence for every asset.
- The demo prints media kind, filename, extension, URL, upload-test flag, long-file-only flag, and role usage dimensions.
- It intentionally avoids downloading files during normal execution, especially `TestLongVideo.mp4`.

## Usage Dimensions

- User: multimodal upload, parsing, retrieval, and RAG quality validation teams.
- Business problem: cover image, BGM/audio, video, long-video upload, and docx script paths with one shared remote fixture set.
- Input data: 2 images, 1 BGM file, 2 short videos, 1 long video, and 1 docx script.
- ORAG capabilities: remote asset registration, upload flow readiness, document parsing, media metadata, and cross-modal retrieval preparation.
- Success signal: every URL is HTTPS, asset type and usage are explicit, and the long video is marked upload-only.

## Reused Assets

- `examples/scenarios/multimodal-assets/main.go`
- `examples/scenarios/multimodal-assets/demo-data.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
