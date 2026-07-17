# Credential rotation runbook

本 runbook 区分机器 API Key 的即时轮换与服务端 JWT/pepper 的维护窗口
切换。任何步骤都不应把 secret、token、完整 API Key、`.env` 或请求 body
写入工单、Git、聊天记录或公开 issue。

## 0. 演练本地产品路径

在有 Docker、Compose、curl、jq 和 Go 的工作站运行：

```bash
make credential-rotation-drill
```

该命令不会读取部署 `.env`，只操作临时本地 volume。它证明 API Key
`POST /v1/api-keys/{api_key_id}/rotate` 是 immediate cutover：source 变为
`401`，replacement 可以继续访问。它不是生产轮换记录。

## 1. 单个机器 API Key

1. 识别 tenant、项目、调用方和当前 Key 的公开 ID；确认调用方能安全接收
   一次性 replacement secret。
2. 使用 tenant admin token 调用 `POST /v1/api-keys/{api_key_id}/rotate`。
3. 立即将返回的 replacement secret 写入调用方的 secret manager；响应关闭
   后不可再次读取。
4. 用 replacement 执行受限健康业务操作；验证 source 返回 `401`。
5. 记录时间、公开 source/replacement ID、调用方、验证结果和 trace ID，
   不记录 secret。若 replacement 未保存，使用 user 登录重新创建 Key；
   不要恢复已撤销 source。

此操作无 grace period。需要无停机迁移的集成应预先在应用层设计多个独立
credential slot；ORAG 不在同一 API Key 上提供双 secret 重叠期。

## 2. 服务端 JWT_SECRET 轮换

JWT 轮换会使全部已签发 user token 无效。安排维护窗口并执行：

1. 备份当前部署配置的**非 secret**清单、镜像 digest 和 migration 状态；
   不导出 secret 值。
2. 更新 server-side secret manager 中的 `JWT_SECRET`，保持
   `API_KEY_PEPPER` 独立且不变；以相同 release 重建 API 实例。
3. 确认 `/readyz`，再确认旧 user token 获得 `401`。
4. 使用 bootstrap/admin 重新登录，执行受限查询和 trace lookup。
5. 将维护窗口、release digest、readiness、旧 token 拒绝、新登录成功和
   回滚决定记录为受限运维证据。

回滚前评估泄露范围：恢复旧 JWT 仅会重新接受尚未过期的旧 token，不能作为
已泄露 secret 的常规恢复方式。

## 3. 服务端 API_KEY_PEPPER 轮换

pepper 是存储 API Key HMAC 的输入。更改它会让**所有**既有 API Key 无法
验证；因为 replacement 在旧 pepper 下创建，也不能在 cutover 前创建可用的
新 Key。安排会影响所有机器调用方的维护窗口：

1. 通知机器调用方并停止或 drain 其写入；保留 user-admin 登录路径。
2. 记录非 secret 变更清单、镜像 digest、当前 API Key 的公开 ID 与 owner。
3. 更新 server-side `API_KEY_PEPPER` 并重启 API；确认 `/readyz`。
4. 验证旧 API Key 获得 `401`，使用新的 user login 创建新的最小权限 API
   Key 并立即分发到各调用方的 secret manager。
5. 对每个调用方验证 replacement 业务操作和 project scope，再撤销不再需要
   的 metadata。记录公开 ID、owner、时间、状态码和 trace ID。

不要把 pepper 轮换称为 API Key endpoint rotation，也不要试图通过数据库
批量重写 hash 来保留旧 Key；这会扩大 secret exposure 和不可审计的变更面。

## 4. Provider、数据库与对象存储凭据

这些凭据由各 provider/secret manager 的流程轮换。先创建新凭据并验证
最小权限连接，再更新 server-side injection、滚动重启并验证 readiness 和
真实业务路径；仅在确认新凭据可用后撤销旧凭据。记录 credential reference
或版本号而不是值。
