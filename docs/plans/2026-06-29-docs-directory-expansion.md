# Docs Directory Expansion Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 ORAG 的 `docs` 从扁平文档集合升级为分层文档中心，补充面向上手、API、架构、评估和运维的更详细专题页。

**Architecture:** 保留现有顶层文档作为兼容和长文入口，新增 `getting-started/`、[`api/`](../../api)、`architecture/`、`evaluation/`、`operations/` 子目录承载更细粒度文档。[`docs/README.md`](../README.md) 作为总目录，仓库根 [`README.md`](../../README.md) 只保留高层导航。

**Tech Stack:** Markdown、Go 1.26、Hertz、Eino Graph、Qdrant、PostgreSQL、Ark/豆包、Docker Compose、OpenAPI。

---

### Task 1: 新增分层文档结构

**Files:**
- Create: [`docs/getting-started/README.md`](../getting-started/README.md)
- Create: [`docs/getting-started/api-smoke.md`](../getting-started/api-smoke.md)
- Create: [`docs/api/README.md`](../api/README.md)
- Create: [`docs/api/auth-and-errors.md`](../api/auth-and-errors.md)
- Create: [`docs/api/ingestion-and-query.md`](../api/ingestion-and-query.md)
- Create: [`docs/architecture/README.md`](../architecture/README.md)
- Create: [`docs/architecture/rag-pipeline.md`](../architecture/rag-pipeline.md)
- Create: [`docs/evaluation/README.md`](../evaluation/README.md)
- Create: [`docs/operations/README.md`](../operations/README.md)
- Create: [`docs/operations/troubleshooting.md`](../operations/troubleshooting.md)

**Steps:**
1. 为每个子目录增加 README 入口，说明适用读者和阅读顺序。
2. 把上手、API、架构、评估和运维细节拆到独立页面。
3. 所有新文档保持与当前实现一致，不夸大尚未实现的能力。

### Task 2: 更新导航入口

**Files:**
- Modify: [`docs/README.md`](../README.md)
- Modify: [`README.md`](../../README.md)

**Steps:**
1. 在 [`docs/README.md`](../README.md) 中新增分层目录地图。
2. 保留顶层旧文档入口，标记为“兼容长文”。
3. 在根 README 的文档中心表格中指向新版子目录。

### Task 3: 验证

**Files:**
- Check: `docs/**/*.md`
- Check: [`README.md`](../../README.md)

**Steps:**
1. 检查所有新增相对链接目标存在。
2. 运行 `PATH="/usr/local/go/bin:/opt/homebrew/bin:$PATH" make openapi-validate`。
3. 查看 `git status --short`，确认只包含预期文档变更。
