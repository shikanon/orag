# Platform Team Demo Data

Shared RAG service readiness checklist:

- The service must expose health and readiness checks before business teams start onboarding.
- Authentication must be verified before knowledge-base creation.
- A platform smoke must create a knowledge base, ingest representative content, query it, and store a trace ID.
- Evaluation and optimization runs provide repeatable quality evidence for retrieval-profile changes.
- MCP and Skill artifacts must stay synchronized with `api/openapi.yaml` so agent clients receive current tool contracts.

Platform onboarding outcome:

Application teams receive one service endpoint, one documented example path, trace evidence for debugging, and quality metrics for release gates.
