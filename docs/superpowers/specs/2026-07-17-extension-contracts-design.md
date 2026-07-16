# Public Extension Contracts Design

**Status:** Implemented and verified on 2026-07-17

**Roadmap:** Stage 5 — stable extension points and ecosystem readiness

## Decision

ORAG exposes a dependency-free public package at
`github.com/shikanon/orag/extensions`. It defines narrow contracts for parser,
chunker, embedder, retriever, reranker, generator, and storage health. A
separate public `extensions/conformance` package gives extension authors an
executable baseline suite.

The contracts are intentionally not a runtime plugin loader. The HTTP service
and public SDK retain their existing configuration and provider registry while
the project is pre-1.0. This avoids making an unverified dependency injection
surface stable merely because an interface exists. Runtime registration can be
introduced later behind these already-tested contracts.

## Support matrix

Every documented extension must carry one of three levels:

- `certified`: maintained by ORAG and passes the corresponding conformance
  suite in repository CI;
- `community`: maintained outside ORAG and publishes its own conformance
  result; and
- `experimental`: no compatibility guarantee.

No non-conforming provider may be listed as certified. The contract itself is
`beta` until ORAG v1.0, matching the repository compatibility policy.

## Conformance invariants

- parser output has non-empty text and independent metadata;
- chunker output is ordered and every chunk has text;
- embedding preserves input cardinality and returns non-empty finite vectors;
- retrieval results are uniquely identified and have finite scores;
- reranking returns exactly the submitted candidate IDs once each with finite
  scores;
- generation returns non-empty output; and
- storage health is cancellation-aware and returns a descriptive status.

The suite uses deterministic, content-free fixtures and never calls a network
or real model provider.

## Non-goals

- Loading arbitrary plugins into a running server.
- Classifying existing vendor metadata as certified without an adapter-level
  conformance result.
- Promising a v1 stable ABI before the project reaches v1.0.
