# Extension contracts and support levels

`github.com/shikanon/orag/extensions` is the public Beta contract for parser,
chunker, embedder, retriever, reranker, model-provider/generator and storage integrations.
It has no dependency on `internal/*`; extension authors can run the
network-free checks in `github.com/shikanon/orag/extensions/conformance` in
their own module.

## Contract interfaces

| Interface | Responsibility |
| --- | --- |
| `Parser` | Converts a source document into normalized text and metadata. |
| `Chunker` | Splits parsed text into retrieval-ready chunks. |
| `Embedder` | Converts ordered text inputs into equally ordered numeric vectors. |
| `Retriever` | Returns ranked retrieval candidates for a query. |
| `Reranker` | Reorders a supplied candidate set for a query. |
| `ModelProvider` | Identifies a model integration and exposes its `Generator`. |
| `Generator` | Produces a response from a prompt. |
| `Storage` | Reports whether a storage dependency is ready for use. |

```go
if err := conformance.Embedder(context.Background(), myEmbedder); err != nil {
    panic(err)
}
```

The contracts are `v0beta1`, not a v1 ABI promise. ORAG deliberately does not
load arbitrary extensions into a running server yet: before v1, the existing
configuration and provider registry remain the supported runtime boundary.

## Support matrix

| Integration | Level | Evidence |
| --- | --- | --- |
| ORAG-maintained public integration | certified | Must pass the relevant conformance suite and ship with CI evidence. No public adapter is certified yet. |
| Built-in production providers and storage adapters | beta | Covered by the service/integration gates; they are not public plug-ins yet. |
| Third-party integrations | community | Maintainer must publish a passing conformance result and compatibility statement. |
| Any unreviewed adapter | experimental | No compatibility or operational guarantee. |

Only an integration that is maintained by ORAG and passes its relevant public
conformance suite may be labelled `certified`. Passing the suite proves the
minimal interface behavior, not performance, security, provider availability
or production capacity.

## Contract rules

- Implement context cancellation and never retain caller document/query data
  beyond the call without explicit product-level consent.
- Preserve input/output cardinality for embeddings and reranking.
- Return finite scores and vectors; do not leak provider secrets in errors.
- Treat the conformance suite as a compatibility floor and run the project’s
  own correctness, security, and integration tests in addition.
