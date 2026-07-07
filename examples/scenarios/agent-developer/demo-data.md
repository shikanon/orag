# Agent Developer Demo Data

Agent integration target:

- The agent must discover ORAG tools through MCP stdio.
- The agent must be able to call bounded verification tools such as `ralph_loop_run`.
- The agent must be able to run read-only checks such as `orag_check` without mutating user systems.
- Skill prompts for Codex, Claude Code, Trae, and operational workflows should describe required environment variables, evidence, and safety boundaries.

Expected agent behavior:

An agent should report a verdict, artifacts, and trace evidence. It should not invent API contracts, print secrets, or run unbounded repair loops.
