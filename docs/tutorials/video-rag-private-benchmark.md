# Video RAG private benchmark protocol

`video-rag/1.0.0` publishes an immutable evaluation **Protocol**, not a data
pack. Video-MME's official terms prohibit redistributing or publishing its
videos and benchmark material. ORAG therefore never downloads, mirrors,
proxies, caches, or serves Video-MME video, subtitle, annotation, question, or
answer data.

## What is public

Quick and Benchmark Protocol files declare only the benchmark identity, the
official source URL, fixed server sampling bounds, and the `temporal_page` P0
runtime profile. They do not contain a media URL, source object key, signed
URL, checksum, subtitle text, question, answer, frame, or credential.

## What stays private

An owner who has separately obtained authorization may import media into their
own project-private store and must affirm that authorization. ORAG verifies a
source alias, SHA-256, size, MIME type, duration, and ordered timed subtitles
before it creates fixed-duration temporal segments.

For a source alias `clip-a`, 0–10 seconds is always represented as
`clip-a@0-10000`. The corresponding internal segment identity is a SHA-256 of
the verified media digest, timestamps, and pinned extractor version. Clients
cannot choose source paths, time ranges, provider settings, or extractor
versions.

## Current boundary

The Protocol parser, private-source validator, and deterministic temporal
segment builder are available. A public Replay remains intentionally
unavailable: it requires a separately authorized, aggregate-only controlled
run. No text RAG runtime is permitted as a fallback for `temporal_page`.

## Verify or publish the Protocol

The release root contains only two JSON declarations and their checksum list.
The guarded publish command uses credentials supplied by the release
environment; it never reads credentials from the Protocol.

```sh
make video-protocol-publish ORAG_PACK_PUBLISH=1 \
  VIDEO_PROTOCOL_ROOT=tutorial-protocols/video-rag/1.0.0
make video-protocol-verify \
  VIDEO_PROTOCOL_ROOT=tutorial-protocols/video-rag/1.0.0
```

After publishing, verify through anonymous HTTPS:

```sh
base=https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs/video-rag/1.0.0
curl -fsSLO "$base/SHA256SUMS"
shasum -a 256 -c SHA256SUMS
```
