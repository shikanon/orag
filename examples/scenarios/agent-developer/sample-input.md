# Sample Input

Agent developer request: Add ORAG as a tool provider for my coding agent so it can run bounded Ralph Loop verification and read-only self-checks with trace evidence.

Inputs:

- MCP stdio client configuration.
- JSON-RPC smoke lines for tool discovery.
- Skill prompts for Codex, Claude Code, Trae, and operational self-check workflows.

Agent goal: discover ORAG tools, call a bounded verification tool, and return structured evidence to the user.
