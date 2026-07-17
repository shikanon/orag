# Production Configuration Guard Design

**Status:** Approved for direct implementation under the repository owner's standing review instruction on 2026-07-17

**Roadmap:** Phase 3 — security and tenant boundaries

## Problem

ORAG documents that operators must replace demo credentials and disable
deterministic mock providers, but the configuration loader currently permits
the local defaults in every environment. A typo in a deployment file can
therefore make a public service start with an unsafe secret or mock runtime.

## Decision

Add the explicit deployment intent `ORAG_ENV`, with `development` as the
backward-compatible default and `production` as the strict mode. No URL,
hostname, image tag, or inferred cloud setting implicitly enables the guard.
This makes production intent auditable and keeps the no-key walkthrough and
tests available only when they do not opt into production.

In `ORAG_ENV=production`, startup must reject:

- any `DEBUG=true`, `ALLOW_DETERMINISTIC_MOCK=true`, mock model provider, or
  local object-storage mock upload;
- a JWT secret or API-key pepper shorter than 32 bytes, one of the known demo
  values, or a pepper equal to the JWT secret;
- a bootstrap administrator password shorter than 16 bytes or equal to the
  known `admin` demo password;
- a `PUBLIC_BASE_URL` that is not an absolute HTTPS URL, has user info, or
  names `localhost`, an IP literal, or a `.local` host.

The guard validates only process configuration. It does not attempt to prove
that a secret is random, a provider key works, a DNS record is controlled, or
network traffic is encrypted end-to-end. Those are deployment/operator
responsibilities and remain documented separately.

## Alternatives considered

1. Infer production from `PUBLIC_BASE_URL`. Rejected: local TLS and staging
   URLs would unexpectedly alter startup, while a public HTTP typo could avoid
   validation.
2. Remove every local default. Rejected: it breaks the documented no-key
   walkthrough and tests without providing an explicit deployment contract.
3. Add opt-in `ORAG_ENV=production` with strict validation. Chosen: it is
   explicit, testable, backward compatible, and can be required by deployment
   manifests.

## Components and flow

1. `ServerConfig` gains `Environment`; the loader accepts only `development`
   or `production` and exposes `IsProduction()`.
2. `Config.Validate()` invokes a focused production validator after normal
   provider validation. It returns a stable error naming the failed setting,
   never the secret value.
3. Unit tests table-drive every rejected production condition and one complete
   allowed production configuration. Existing explicit mock development tests
   demonstrate that local behavior remains unchanged.
4. The environment template, reference deployment guide and operations index
   declare `ORAG_ENV=production`; a contract test prevents documentation and
   Compose deployment guidance from drifting.

## Security boundaries

- Error messages name environment variables but never include their values.
- The guard treats loopback/IP and `.local` hosts as local-only. It does not
  inspect DNS resolution, so a public domain must still be protected by the
  operator's TLS and DNS controls.
- No user secret, provider configuration, or object-storage credential is
  written to source, a test fixture, documentation output, or telemetry.
- The new guard is not a claim that the roadmap's production pilot, secret
  rotation procedure, or external threat-model review is complete.

## Acceptance evidence

- Focused config tests prove production rejects each unsafe class and accepts
  a safe full configuration.
- A contract test requires `ORAG_ENV=production` in the sample production
  environment and operations documentation.
- `make agent-gate` passes.
- The reference deployment guide remains clear that production API/Console
  need a dedicated hostname rather than the static documentation path.
