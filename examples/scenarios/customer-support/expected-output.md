# Expected Output

- `30_query.sh` returns an answer grounded in imported support material.
- The response includes citations that a support agent can attach to the reply.
- The response includes a `trace_id` that can be inspected with `36_trace_lookup.sh` during escalation.
- Missing service, token, knowledge base, or document state fails fast with actionable messages from the shared curl helper.
