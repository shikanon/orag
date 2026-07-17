# API Key Rotation Design

**Status:** Approved for direct implementation under the repository owner's standing review instruction on 2026-07-17

**Roadmap:** Phase 3 — secret injection, rotation, redaction and tenant boundaries

## Decision

ORAG will add `POST /v1/api-keys/{api_key_id}/rotate`. A tenant administrator
can rotate a key belonging to the current tenant. The service generates new
32-byte material, preserves the source key's tenant, project scope, role,
name, and future expiry, records the source key ID in the replacement metadata,
and revokes the source key in the same repository transaction. The replacement
secret is returned exactly once in a `201` response; list responses contain
only metadata and never a hash or secret.

This is an **immediate cutover**, not a grace-period or zero-downtime rotation.
Once the operation succeeds, the source secret cannot authenticate. Clients
must persist the replacement secret before a caller discards the response, and
high-availability automation should use separately managed keys when it needs
an overlap window.

## Alternatives considered

1. Create a second key and ask clients to delete the first. Existing behavior;
   it is not a single auditable rotation and can leave stale credentials active.
2. Create a replacement, leave both active for a fixed grace period. Requires
   delayed revocation, scheduler reliability, time-window policy and recovery
   semantics. Deferred until operators have a concrete overlap requirement.
3. Transactionally create a replacement and revoke the source. Chosen: it has
   one clear postcondition, works in the current repository abstractions, and
   can be tested against memory and PostgreSQL stores.

## Data and authorization flow

1. HTTP middleware authenticates a user or tenant-wide admin API key; existing
   `api_key.manage` authorization remains the only authorization gate.
2. The service validates an active, unexpired tenant source, generates the
   replacement secret, and delegates one repository rotation operation.
3. The repository inserts the replacement with `rotated_from_key_id=source`,
   then marks the source revoked at the same UTC timestamp. Any error rolls
   back both writes.
4. The response returns replacement metadata and the one-time secret. Later
   list calls expose `rotated_from_key_id` but no sensitive material.

## Storage and concurrency

Migration `000039_api_key_rotation.sql` adds a nullable self-referencing
`rotated_from_key_id`, a lookup index, and a partial unique index so one source
has at most one recorded replacement. PostgreSQL locks the source with `FOR
UPDATE`; concurrent rotations produce one replacement and one typed conflict.
The in-memory repository performs equivalent active-source checks under mutex.

## Security boundaries

- Secrets never enter logs, list responses, error messages, OpenAPI examples,
  Console state after dialog close, or migration records.
- Cross-tenant, revoked, and expired source IDs have the same not-found surface.
- Random-source and persistence failures leave the source key active.
- This does not rotate server-level JWT, provider, database, or object-storage
  secrets; their rollout procedure remains an operator responsibility.

## Acceptance evidence

- Tests prove attribute preservation, source rejection, replacement
  authentication, cross-tenant concealment, and failure atomicity.
- Migration/repository tests prove lineage and one-success concurrent rotation.
- HTTP/OpenAPI/Console tests prove one-time rendering and no secret in lists.
- `make agent-gate` passes.
