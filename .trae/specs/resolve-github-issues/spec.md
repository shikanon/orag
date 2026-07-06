# GitHub Issues 收口 Spec

## Why
GitHub 仓库存在多组开放 issue，其中既有已在当前代码或历史规格中实现但未关闭的重复问题，也有仍需补齐的正确性与隔离性缺陷。需要先同步远程最新代码，再按根因合并开发、验证并关闭已经完成的 issue，避免重复开发和误导性开放状态。

## What Changes
- 同步 `origin` 最新分支与主干状态，确认当前工作分支不丢失已有实现。
- 审计所有开放 issue，按根因合并为可验证工作项，并记录每个 issue 的关闭依据。
- 实现尚未完成的需求：知识库真实删除、知识库写入错误传播、数据集/评估租户隔离、语义缓存 profile 隔离、入库知识库存在性校验、重新入库旧 chunk 清理、模型 API Key 默认校验。
- 对当前代码已经实现的 trace 类 issue 执行回归验证，并关闭对应 GitHub issue。
- 对重复 issue 在根因修复完成后批量关闭，关闭说明引用验证命令和修复范围。
- 第二轮收口当前新增开放 issue：逐个审计 #115-#130、#142、#146、#147、#155-#157，修复仍未满足的行为，关闭已验证完成项，并通过 PR 合并到远程主干。

## Impact
- Affected specs: GitHub issue 收口、知识库 API、数据集与评估租户隔离、入库一致性、RAG 缓存隔离、配置校验、trace 观测。
- Affected code: `internal/kb`、`internal/storage/postgres`、`internal/storage/qdrant`、`internal/http`、`internal/app`、`internal/dataset`、`internal/eval`、`internal/ingest`、`internal/rag`、`internal/config`、`api/openapi.yaml`、`docs/*`、`tests/*`。

## ADDED Requirements
### Requirement: 远程同步与 Issue 审计
The system SHALL synchronize from `origin`, enumerate all open issues in `https://github.com/shikanon/orag/issues`, and classify each issue as already implemented, duplicate of an implemented fix, or requiring development.

#### Scenario: Issue classification
- **WHEN** the maintainer runs the收口流程
- **THEN** each open issue has a recorded root-cause group, implementation status, verification evidence, and close/no-close decision.

### Requirement: 真实删除知识库
The system SHALL implement `DELETE /v1/knowledge-bases/{id}` as a real tenant-scoped deletion that removes the knowledge base and directly owned indexed content.

#### Scenario: Delete existing knowledge base
- **WHEN** a tenant deletes its own knowledge base
- **THEN** the API returns `204`, subsequent get/list no longer show it, and retrieval no longer returns its chunks.

#### Scenario: Delete unknown or cross-tenant knowledge base
- **WHEN** a tenant deletes a missing or other-tenant knowledge base
- **THEN** the API returns a stable not-found response without deleting other-tenant data.

### Requirement: 知识库持久化错误传播
The system SHALL propagate PostgreSQL knowledge-base write/read errors through repository interfaces to application and HTTP layers.

#### Scenario: PostgreSQL write fails
- **WHEN** `PutKnowledgeBase` fails because the backend returns an error
- **THEN** `POST /v1/knowledge-bases` returns a 5xx error and does not report a false `201 Created`.

### Requirement: 数据集和评估租户隔离
The system SHALL enforce tenant ownership checks for dataset item writes, dataset item reads, evaluation runs, and optimizer paths.

#### Scenario: Cross-tenant dataset access
- **WHEN** tenant A references tenant B's dataset
- **THEN** add-item and evaluation paths reject the request before reading or evaluating tenant B data.

### Requirement: 入库目标校验与原子可见性
The system SHALL validate knowledge-base existence before document ingestion and prevent failed or replacement ingestion jobs from exposing stale or partially committed chunks.

#### Scenario: Ingest into missing knowledge base
- **WHEN** an ingestion request references an unknown knowledge base
- **THEN** the request fails with a stable not-found error and no document/chunk rows are committed.

#### Scenario: Re-ingest same document
- **WHEN** a document is re-ingested into the same knowledge base
- **THEN** old chunks are removed or superseded before new chunks become visible.

### Requirement: Profile-scoped Semantic Cache
The system SHALL scope semantic cache lookups and writes by tenant, profile, and query identity so different RAG profiles cannot reuse incompatible cached answers.

#### Scenario: Same query different profile
- **WHEN** two profiles ask the same query under the same tenant
- **THEN** semantic cache entries do not cross profile boundaries.

### Requirement: 模型 API Key 默认校验
The system SHALL require real model provider API keys by default for non-mock provider modes and document local mock/test exceptions.

#### Scenario: Missing production API key
- **WHEN** the service starts with a real model provider and no required API key
- **THEN** startup or config validation fails with an actionable error.

### Requirement: GitHub Issue 关闭
The system SHALL close all open issues whose requirements are implemented and verified, including duplicate issues covered by the same root-cause fix.

#### Scenario: Close completed issue
- **WHEN** a fix is implemented or confirmed already present and relevant tests pass
- **THEN** the issue is closed with a concise comment containing fix summary, verification command, and commit/branch context.

### Requirement: 第二轮开放 Issue 收口
The system SHALL enumerate the current open issues from `https://github.com/shikanon/orag/issues`, process each issue with an explicit fix-or-verify decision, and leave no completed issue open.

#### Scenario: Process each current issue
- **WHEN** the maintainer starts the second issue-closing loop
- **THEN** every open issue has a recorded status, code or documentation change if needed, focused verification evidence, and a close decision.

### Requirement: Pull Request and Merge
The system SHALL deliver the verified issue fixes through a remote branch, create a pull request against `main`, and merge it after required checks pass.

#### Scenario: Merge verified changes
- **WHEN** all targeted issues are fixed or verified and regression tests pass
- **THEN** a PR is opened with issue references, merged into the remote repository, and local `main` is synchronized with the merge commit.

## MODIFIED Requirements
### Requirement: Existing API Error Semantics
Knowledge-base, dataset, evaluation, ingestion, and delete APIs SHALL return stable error codes for not-found, cross-tenant, validation, and backend failure cases rather than silently returning empty results or success.

### Requirement: Existing Trace Issue Handling
Trace-related issues SHALL be treated as complete only after current code verifies failed query trace persistence and duplicate `trace_id` behavior according to the existing trace observability specification.

## REMOVED Requirements
### Requirement: Keep Completed GitHub Issues Open
**Reason**: Open duplicate/completed issues make project status inaccurate and trigger repeated coding loops.
**Migration**: Close completed issues with evidence comments after verification.
