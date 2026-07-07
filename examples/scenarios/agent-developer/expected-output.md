# Expected Output

- MCP discovery returns `initialize` and `tools/list` JSON-RPC responses.
- Ralph Loop and self-check smoke files document `ralph_loop_run` and `orag_check` tool calls.
- `make agent-sync-check` verifies generated MCP and Skill artifacts are synchronized with `api/openapi.yaml`.
- Agent developers can copy client config and Skill prompts into their agent environment without reverse-engineering ORAG internals.
