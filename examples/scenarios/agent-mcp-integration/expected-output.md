# Expected Output

- The discovery smoke returns `initialize` and `tools/list` JSON-RPC responses.
- Tool discovery includes `ralph_loop_run` when generated tools are available.
- `make agent-sync-check` validates generated MCP/Skill artifacts against `api/openapi.yaml`.
- Skill guides show client-specific prompts and evidence expectations without exposing bearer tokens.
