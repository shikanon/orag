# Public GHCR Release Verification Design

**Roadmap:** Phase 1/3 — public multi-architecture images and deployable Compose baseline

## Problem

Building and pushing a container image proves registry write access, but does
not prove that an unauthenticated operator can pull the release image.  A
prerelease must not be created until its API and Console tags resolve publicly
to the multi-architecture digest recorded in the release notes.

## Decision

Add a release-workflow job after the two image builds and before GitHub Release
creation.  The job downloads the image digest artifacts and, for each image,
uses only anonymous GHCR bearer-token and manifest requests.  It requires:

- HTTP 200 from the immutable release tag;
- an OCI/Docker manifest-list content type;
- a `Docker-Content-Digest` equal to the Buildx-produced digest; and
- `linux/amd64` and `linux/arm64` entries in the returned manifest index.

The GitHub prerelease depends on this job as well as image builds.  A private
package, mutable/mispointed tag, or single-architecture publication therefore
fails before release notes claim a deployable public image.

## Scope and Error Handling

This does not change package visibility through GitHub APIs; that is an
organization/repository setting.  It verifies the customer-facing invariant
after publication.  HTTP, digest, media-type, architecture, and missing-artifact
failures terminate the release job with a diagnostic that names the image and
tag but never prints credentials.

## Verification

Contract tests pin the release-job dependency and anonymous-manifest checks.
The release workflow itself is the integration gate.  Existing public
`v0.1.0-beta.2` API and Console tags have been independently checked through
anonymous GHCR requests before this change.
