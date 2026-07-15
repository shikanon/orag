# Finite optimizer expression results plan

**Issue:** [#223](https://github.com/shikanon/orag/issues/223)

**Goal:** Preserve finite objective values and fail closed when expression arithmetic produces NaN or infinity.

## Delivery

- [x] Reproduce the remote fuzz failure with a deterministic large-decimal seed.
- [x] Avoid overflowing the rounding scale for very large finite values.
- [x] Reject non-finite expression results with a validation error.
- [x] Add named regression tests and keep the failure pattern in the fuzz seed corpus.
- [ ] Pass local and protected remote gates, then merge a ready PR that closes #223.

## Verification

- Focused finite and overflow regression tests
- 20-second local `FuzzCompileExpression` run
- `make agent-gate`
- Protected remote checks including `native (expression)`
