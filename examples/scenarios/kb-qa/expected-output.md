# Expected Output

- `05_health_ready.sh` prints healthy `/healthz` and ready `/readyz` JSON.
- `00_login.sh` prints an `access_token` and stores it under `.orag-demo/token`.
- `10_create_kb.sh` stores a knowledge base ID under `.orag-demo/kb_id`.
- `20_upload_doc.sh` and `25_upload_file.sh` return document, chunk, and job metadata.
- `30_query.sh` returns an answer, citations, and a `trace_id` for follow-up diagnostics.
