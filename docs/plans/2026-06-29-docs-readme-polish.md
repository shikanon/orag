# Docs Readme Polish Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 优化 ORAG 的 README 与文档入口，使其更接近 RAGFlow 风格的项目主页表达，同时保持当前实现边界准确。

**Architecture:** README 负责产品化总览、能力矩阵、快速开始和文档导航；`docs/README.md` 作为文档中心，承接 API、开发、评估和运维深水区。现有专题文档保持细节，不重复堆叠到首页。

**Tech Stack:** Markdown、GitHub Shields、Go 1.26、Hertz、Eino Graph、Qdrant、PostgreSQL、Ark/豆包。

---

### Task 1: README 产品化改版

**Files:**
- Modify: `README.md`

**Steps:**
1. 增加居中标题、项目标签和状态徽章。
2. 调整首屏文案，突出 ORAG 是 Go RAG service framework。
3. 增加能力矩阵、架构概览、快速开始、API smoke、验证和文档导航。
4. 保留 mock Ark、真实依赖、Mac Go flags 等关键约束。

### Task 2: 文档中心入口

**Files:**
- Create: `docs/README.md`

**Steps:**
1. 增加文档地图，按“快速上手、接口集成、质量评估、部署运维、架构方案”分组。
2. 增加常用路径和维护规则，减少入口分散。

### Task 3: 验证

**Files:**
- Check: `README.md`
- Check: `docs/README.md`

**Steps:**
1. 检查 Markdown 链接路径是否指向现有文件。
2. 运行 `make openapi-validate`，确认文档改动未影响 API 契约测试。
