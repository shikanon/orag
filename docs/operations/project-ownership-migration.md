# Project 资源归属迁移

迁移 `000018_project_owned_roots.sql` 为知识库和数据集增加 `project_id`；后续迁移 `000019_project_owned_evaluation_optimization.sql` 将相同归属传播到评测和优化运行根资源。两次迁移共同让项目级 API Key 可以在数据、评测和优化链路上保持同一授权边界。

## 升级行为

- 每个已有 tenant 会获得一个确定性的兼容 Project：`prj_default_<tenant_id>`。
- 兼容 Project 会获得 `development`、`staging`、`production` 三套环境。
- 已有知识库和数据集会回填到该兼容 Project。
- 已有评测和优化运行会从关联数据集继承 Project；无法匹配历史数据集的孤立记录会回填到 tenant 的兼容 Project。
- 新建评测或优化运行要求数据集与知识库属于同一 Project，读取、取消和恢复均按运行记录自身的 `project_id` 授权。
- 新建知识库和数据集可以显式传入同 tenant 下的 `project_id`；跨 tenant 或不存在的 Project 会返回 `404 project_not_found`。
- beta 兼容期内仍允许旧客户端省略 `project_id`。这只是临时兼容路径，后续阶段会在所有调用方升级后收紧为必填和数据库非空约束。

迁移使用 `(tenant_id, project_id)` 复合外键，防止资源关联到其他 tenant 的 Project。启动引导也会幂等创建默认 Project 与三套环境，因此全新数据库和已有数据库得到相同结果。

## 发布前检查

先备份 PostgreSQL，然后执行迁移并确认没有未归属资源：

```sql
SELECT count(*) FROM knowledge_bases WHERE project_id IS NULL;
SELECT count(*) FROM datasets WHERE project_id IS NULL;
SELECT count(*) FROM evaluation_runs WHERE project_id IS NULL;
SELECT count(*) FROM optimization_runs WHERE project_id IS NULL;
SELECT tenant_id, project_id, count(*)
FROM knowledge_bases
GROUP BY tenant_id, project_id;
```

对于升级前已经存在的记录，预期四个空值计数均为 `0`。beta 兼容期内，未显式传入 `project_id` 的新客户端仍可能创建空归属记录；发布检查应区分迁移遗留数据与兼容期新增数据。同时抽样确认 `projects` 和 `project_environments` 中存在对应默认记录，并确认评测、优化记录的 Project 与关联数据集一致。

## 回滚边界

两次 down migration 会分别移除对应资源表上的索引、复合外键和 `project_id` 列，但不会删除自动创建的兼容 Project 或环境。这些记录可能在升级后已被引用或修改，自动删除会带来数据损失风险；如确需清理，应在确认没有依赖后由运维人员单独执行。
