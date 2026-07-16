# Backup, restore, and disaster recovery

This runbook is the minimum recovery procedure for a production ORAG
deployment. It covers the two sources of truth that must be recovered
together: PostgreSQL metadata and Qdrant vectors. Restoring only one store can
make documents appear active without searchable vectors, or leave vectors that
PostgreSQL correctly rejects.

Keep backups outside the host, encrypt them at rest, restrict access to the
operations group, and record the ORAG version, migration level, collection
names, embedding dimension, and image digests beside every backup. Never put a
database password, API key, JWT secret, or backup credential in Git.

## Recovery objectives and change freeze

Set the deployment's target values explicitly:

| Objective | Meaning | Reference starting point |
| --- | --- | --- |
| RPO | Maximum accepted data loss | 24 hours until scheduled backups are configured |
| RTO | Maximum time to restore service | 2 hours for a single-host restore |

Before a coordinated backup, stop ingestion, deletion, and optimizer writes.
Queries may continue, but a short read-only window gives the PostgreSQL and
Qdrant snapshots a clear consistency boundary:

```bash
docker-compose --env-file .env stop orag-api orag-console
```

Record the exact release and schema state before copying data:

```bash
docker-compose --env-file .env images
docker-compose --env-file .env run --rm --no-deps orag-api oragctl migrate --status
```

`migrate --status` is read-only: it lists every migration bundled with the
running release as `applied` or `pending`, with the applied timestamp. Keep
this output with the backup manifest. A pending migration means the backup and
the intended restore image are not at the same schema level; resolve that
before claiming a restore drill passed.

## PostgreSQL backup

Use the PostgreSQL client that matches or is newer than the server version and
write the dump to encrypted backup storage, not the application repository:

```bash
set -eu
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "/backup/orag/$stamp"
docker-compose --env-file .env exec -T postgres \
  pg_dump -U orag -d orag --format=custom --no-owner --no-acl \
  > "/backup/orag/$stamp/postgres.dump"
sha256sum "/backup/orag/$stamp/postgres.dump" \
  > "/backup/orag/$stamp/SHA256SUMS"
```

Copy the dump and its checksum to an independent encrypted location. A local
file alone is not a backup if the host and its disk are lost together.

After both artifacts and `manifest.json` are present, verify the restore
preconditions before touching a restore target:

```bash
oragctl backup-verify --dir "/backup/orag/$stamp"
```

The manifest uses `orag.backup.v1`, records only release/migration provenance
and artifact names, and must list `postgres.dump` and
`qdrant-snapshots.tgz`. The verifier checks both files against `SHA256SUMS`
and rejects unknown manifest fields or credential-shaped fields. It is
read-only and is not a substitute for the isolated restore drill below.

## Qdrant backup

For a named collection, request a Qdrant snapshot and copy the resulting file
out of the Qdrant volume. Repeat for the primary and semantic-cache
collections:

```bash
for collection in orag_chunks orag_semantic_cache; do
  curl --fail --request POST \
    "http://127.0.0.1:6333/collections/${collection}/snapshots"
done

docker-compose --env-file .env exec -T qdrant sh -c \
  'tar -C /qdrant/storage -czf - snapshots' \
  > "/backup/orag/$stamp/qdrant-snapshots.tgz"
sha256sum "/backup/orag/$stamp/qdrant-snapshots.tgz" \
  >> "/backup/orag/$stamp/SHA256SUMS"
```

If the Qdrant API is protected, pass its API key through the shell environment
or a secret manager; do not replace the placeholder with a committed value.
For large collections, prefer a storage-level snapshot or object-storage
export supported by the Qdrant deployment rather than streaming through the
shell.

## Restore drill in an isolated stack

Run the first restore into a separate project, host, or namespace. Never
overwrite production volumes before the restored data passes the checks below.

1. Verify `SHA256SUMS` before extracting any artifact.
2. Start PostgreSQL and Qdrant with empty restore volumes.
3. Restore PostgreSQL, then run the release's migrations:

   ```bash
   cat postgres.dump | docker-compose exec -T postgres \
     pg_restore -U orag -d orag --no-owner --no-acl --exit-on-error
   docker-compose run --rm --no-deps orag-api oragctl migrate
   ```

4. Restore the Qdrant snapshot through the supported Qdrant snapshot restore
   API or the documented storage restore procedure for the installed version.
   Confirm both collections have the expected vector size and cosine distance.
5. Start API and Console, then verify:

   ```bash
   curl --fail http://127.0.0.1:8080/healthz
   curl --fail http://127.0.0.1:8080/readyz
   curl --fail http://127.0.0.1:8080/metrics | grep -E '^orag_up[ ]+1'
   ```

6. Run a known cited query and fetch its trace. Confirm the answer's citation
   points to a document present in PostgreSQL and that every returned dense
   candidate is authorized by `chunks.searchable`.
7. Exercise a failed re-ingestion and a repeated knowledge-base delete in the
   isolated stack. The former must not expose the failed version; the latter
   must eventually clean external vectors without losing the retry metadata.
8. Record elapsed restore time, migration level, image digests, collection
   configuration, and the verification output as the restore-drill evidence.

## Production cutover and rollback

After the isolated restore passes, schedule a write freeze and stop the
production API. Restore into new volumes, run the same checks, and switch the
reverse proxy only after `/readyz` is healthy. Keep the old volumes read-only
until the application smoke test and a short observation window complete.

If the restore fails, switch the proxy back to the old API and volumes; do not
delete the original data while investigating. If an application release is
the cause, roll back API and Console to the previous immutable image digest,
then rerun `/readyz` and the cited-query smoke test. Never move or overwrite a
published Git tag.

## Drill cadence and evidence

- Take an encrypted PostgreSQL dump and Qdrant snapshot at least daily until a
  lower RPO is required.
- Perform a complete isolated restore at least once per release and quarterly
  thereafter.
- Alert when a scheduled backup is missing, its checksum is invalid, or the
  last restore drill is older than the agreed interval.
- Store the drill report with backup IDs, checksums, release digest, migration
  level, start/end timestamps, and operator approval.

For the server layout and immutable GHCR image deployment, see the
[reference server deployment guide](server-deployment.md). For dependency and
readiness failures, see [troubleshooting](troubleshooting.md).
