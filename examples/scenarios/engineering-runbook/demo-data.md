# Engineering Runbook Demo Data

Query latency runbook:

- Check `/readyz` first to confirm PostgreSQL, Qdrant, and model-provider dependencies are healthy.
- Inspect the query response `trace_id` before changing retrieval settings.
- Compare retrieval timing, citation count, and answer metadata in the trace detail.
- If latency increased after a deploy, attach the trace ID, service version, query text, and knowledge-base ID to the incident ticket.

Escalation rule:

Do not tune top-k or profile settings during an incident without trace evidence. Capture the current trace first, then compare it with an evaluation or optimization run after the incident is stable.
