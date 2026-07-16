# Video RAG Private Benchmark Implementation Plan

> **Execution note:** This plan is executed directly on the dedicated feature branch. Progress is verified through the listed focused and regression checks.

**Goal:** Provide a legally safe, reproducible Video RAG P0 tutorial using project-private, owner-authorized media and deterministic temporal evidence.

**Architecture:** Add a strict Video Benchmark Protocol beside Pack and Recipe manifests; it declares only fixed benchmark identity and server-owned sampling limits. Private video imports produce immutable source records and deterministic temporal segment records; the isolated `temporal_page` runtime indexes only server-generated segment descriptions and evaluates time evidence without distributing Video-MME data.

**Tech Stack:** Go, PostgreSQL, project-private object storage, existing tutorial Clone/Run services, SHA-256, Hertz API, React Console.

## Global Constraints

- Never download, mirror, proxy-cache, publish, copy, or distribute Video-MME data, annotations, subtitles, frames, or media.
- Public protocol has no media URL, object key, signed URL, question, answer, subtitle text, source hash, or credentials.
- Source media is owner-provided, project-private, SHA-addressed and accepted only after licence confirmation.
- Temporal evidence IDs use fixed source alias/start/end/extractor-version inputs; browser requests cannot select timestamps, extraction settings, provider, or paths.
- `temporal_page` must not use a text tutorial runtime definition or fallback.
- Public Replay remains unavailable until a legally authorized, aggregate-only controlled run exists.

---

### Task 1: Validate the immutable Video Benchmark Protocol

**Files:**
- Create: `internal/tutorial/video_protocol.go`
- Create: `internal/tutorial/video_protocol_test.go`
- Create: `tutorial-protocols/video-rag/1.0.0/{quick,benchmark}/protocol.json`
- Create: `tutorial-protocols/video-rag/1.0.0/SHA256SUMS`

**Interfaces:**
- Produces `ParseVideoProtocol(raw []byte, template Template, pack PackRef) (VideoBenchmarkProtocol, error)`.
- Produces `VideoBenchmarkProtocol{TemplateID, Version, Tier, Benchmark, Sampling, Runtime}`.

- [ ] **Step 1: Write failing protocol tests**

```go
func TestParseVideoProtocolRejectsMediaURL(t *testing.T) {
    _, err := ParseVideoProtocol([]byte(`{"template_id":"video-rag","version":"1.0.0","tier":"quick","media_url":"https://example.com/a.mp4"}`), videoTemplate(), videoPack())
    if !errors.Is(err, ErrVideoProtocolInvalid) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run the focused test**

Run: `go test ./internal/tutorial -run TestParseVideoProtocolRejectsMediaURL -count=1`

Expected: FAIL because `ParseVideoProtocol` is undefined.

- [ ] **Step 3: Implement strict decoding**

```go
type VideoSampling struct { SegmentMilliseconds int64 `json:"segment_milliseconds"`; MaxSegments int `json:"max_segments"` }
type VideoBenchmarkProtocol struct { TemplateID, Version, Tier string; Benchmark VideoBenchmark; Sampling VideoSampling; Runtime TemporalRuntimeManifest }
func ParseVideoProtocol(raw []byte, template Template, pack PackRef) (VideoBenchmarkProtocol, error) { /* DisallowUnknownFields; fixed Video-MME identity; no source fields; bounded sampling; temporal_page P0 */ }
```

- [ ] **Step 4: Add versioned protocol JSON and checksum contract**

Use `shasum -a 256 -c SHA256SUMS` from `tutorial-protocols/video-rag/1.0.0`.

- [ ] **Step 5: Run protocol tests and commit**

Run: `go test ./internal/tutorial -run 'Test(ParseVideoProtocol|PublishedVideoProtocols)' -count=1`

Commit: `feat: validate private video benchmark protocol`

### Task 2: Persist owner-authorized private video sources

**Files:**
- Create: `internal/tutorial/video_source.go`
- Create: `internal/tutorial/video_source_test.go`
- Modify: `internal/tutorial/private_store.go`

**Interfaces:**
- Produces `VideoSource{Alias, SHA256, Bytes, ContentType, Subtitle}`.
- Produces `ValidateVideoSource(VideoSource) error` and `ValidateTimedSubtitles([]TimedSubtitle) error`.

- [ ] **Step 1: Write failing validation tests**

```go
func TestValidateTimedSubtitlesRejectsNonMonotonicIntervals(t *testing.T) {
    err := ValidateTimedSubtitles([]TimedSubtitle{{StartMS: 10, EndMS: 20}, {StartMS: 15, EndMS: 30}})
    if !errors.Is(err, ErrVideoSourceInvalid) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Implement source and subtitle validation**

```go
type TimedSubtitle struct { StartMS, EndMS int64; Text string }
func ValidateVideoSource(source VideoSource) error { /* alias, video MIME, positive bounded bytes, digest */ }
func ValidateTimedSubtitles(items []TimedSubtitle) error { /* ordered non-overlap, bounded text/count */ }
```

- [ ] **Step 3: Run validation tests and commit**

Run: `go test ./internal/tutorial -run 'TestValidate(VideoSource|TimedSubtitles)' -count=1`

Commit: `feat: validate private video tutorial sources`

### Task 3: Create deterministic temporal evidence segments

**Files:**
- Create: `internal/tutorial/video_segments.go`
- Create: `internal/tutorial/video_segments_test.go`

**Interfaces:**
- Produces `BuildTemporalSegments(source VideoSource, protocol VideoBenchmarkProtocol) ([]TemporalSegment, error)`.
- Produces `TemporalSegment{ID, EvidenceID, StartMS, EndMS, SubtitleText}`.

- [ ] **Step 1: Write failing determinism and alignment tests**

```go
func TestBuildTemporalSegmentsUsesStableEvidenceIDs(t *testing.T) {
    segments, err := BuildTemporalSegments(source, protocol)
    if err != nil || segments[0].EvidenceID != "clip-a@0-10000" { t.Fatalf("%#v %v", segments, err) }
}
```

- [ ] **Step 2: Implement bounded segment construction**

```go
func BuildTemporalSegments(source VideoSource, protocol VideoBenchmarkProtocol) ([]TemporalSegment, error) { /* fixed cadence; cap; overlap subtitles by interval; SHA identity */ }
```

- [ ] **Step 3: Run segment tests and commit**

Run: `go test ./internal/tutorial -run TestBuildTemporalSegments -count=1`

Commit: `feat: derive deterministic video evidence segments`

### Task 4: Add isolated temporal P0 runtime contract

**Files:**
- Modify: `internal/tutorial/manifest.go`
- Modify: `internal/tutorial/clone.go`
- Modify: `internal/tutorial/run_definition.go`
- Modify: `internal/tutorial/run.go`
- Modify: `internal/tutorial/*_test.go`

**Interfaces:**
- Produces `supportsVideoRuntime(templateID, tier string) bool`.
- Persists `TemporalAssets []PackObject` inside the private experiment Manifest snapshot.

- [ ] **Step 1: Write a failing no-text-fallback test**

```go
func TestVideoRuntimeDefinitionRequiresTemporalAssets(t *testing.T) {
    _, err := service.runtimeDefinition(videoExperimentWithoutAssets, "baseline")
    if !errors.Is(err, ErrRuntimeUnavailable) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Implement temporal profile and assets**

```go
func supportsVideoRuntime(templateID, tier string) bool { return templateID == "video-rag" && (tier == "quick" || tier == "benchmark") }
```

- [ ] **Step 3: Run tutorial regression tests and commit**

Run: `go test ./internal/tutorial -count=1`

Commit: `feat: add isolated temporal tutorial runtime`

### Task 5: Expose owner import, redacted status, and documentation

**Files:**
- Modify: `internal/http/tutorial_clones.go`
- Modify: `api/openapi.yaml`
- Modify: `console/src/features/tutorials/*`
- Create: `docs/tutorials/video-rag-private-benchmark.md`
- Create: `docs-site/tutorials/video-rag-private-benchmark.html`

**Interfaces:**
- Consumes an authorized private upload reference and source alias.
- Produces source-alias-only status and temporal P0 readiness.

- [ ] **Step 1: Write HTTP tests for redaction and tenant isolation**

```go
if strings.Contains(response.Body, "object_key") || strings.Contains(response.Body, "subtitle_text") { t.Fatal("private video detail leaked") }
```

- [ ] **Step 2: Implement server-owned import route and Console state**

The request permits only source alias and an existing private upload token; server derives all other coordinates.

- [ ] **Step 3: Build OpenAPI, Console and hosted docs**

Run: `make openapi-validate && npm --prefix console run build && make docs-build`

- [ ] **Step 4: Commit**

Commit: `feat: expose private video benchmark import`

### Task 6: Verify, publish protocol only, and document Replay boundary

**Files:**
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `docs/api/README.md`
- Test: `internal/tutorial/video_protocol_test.go`

**Interfaces:**
- Produces guarded `video-protocol-publish` and anonymous `video-protocol-verify` targets.

- [ ] **Step 1: Add guarded release targets and tests**

```make
video-protocol-publish:
	@test "$(ORAG_PACK_PUBLISH)" = "1"
	go run ./cmd/orag-pack-release -publish "$(VIDEO_PROTOCOL_ROOT)"
```

- [ ] **Step 2: Verify no Video-MME media appears in release root**

Run: `find tutorial-protocols/video-rag -type f | grep -E '\.(mp4|webm|srt|vtt)$' && exit 1 || true`

- [ ] **Step 3: Run complete checks and commit**

Run: `go test ./... && go vet ./... && make openapi-validate && make docs-build`

Commit: `docs: publish private video benchmark boundary`
