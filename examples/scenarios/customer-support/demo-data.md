# Customer Support Demo Data

ORAG support policy:

- Support answers must be grounded in imported product documentation.
- Every customer-facing answer should include citations when the response uses private knowledge.
- Escalations should include the `trace_id` from the query response so engineering can inspect retrieval and generation behavior.

Customer issue:

The customer uploaded a troubleshooting guide and asks why the assistant answer references citations instead of returning a generic response.

Recommended support outcome:

Explain that citations prove which source material was used, confirm the answer was generated from the current knowledge base, and attach the trace ID if the customer reports a wrong or incomplete answer.
