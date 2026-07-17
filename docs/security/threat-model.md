# 威胁模型与凭据边界

本文记录 ORAG 当前 Beta 的安全边界和可验证控制，不替代部署方的
风险评估、合规审查或事件响应程序。

## 资产与边界

| 资产 | 信任边界 | 主要保护措施 | 检测与恢复 |
| --- | --- | --- | --- |
| 用户登录 token | 浏览器、反向代理、API | JWT 签名、短 TTL、HTTPS、生产配置门禁 | `401 invalid_bearer_token`；轮换 JWT 后重新登录。 |
| 机器 API Key | 自动化客户端、API、数据库 | 32 字节随机 secret、HMAC pepper、常量时间验证、项目 RBAC、一次性返回 | API Key rotation drill；撤销或立即轮换后旧 Key 被拒绝。 |
| `JWT_SECRET` / `API_KEY_PEPPER` | 仅部署方 secret manager 与 API 进程 | 不进入 Git、日志和公共诊断；生产环境强制强度和独立性 | 维护窗口重启、readiness、旧凭据拒绝与新凭据验证。 |
| provider、数据库和对象存储凭据 | 服务端进程与受限网络 | server-only 注入、redacted diagnostics、CI secret scanning | provider/存储方的轮换和审计流程。 |
| 租户和项目数据 | HTTP/SDK 调用者、PostgreSQL、Qdrant | tenant/project 谓词、最小角色、跨域 `404` | 授权回归、trace 与审计日志。 |
| prompt、文档和日志 | 入库/查询流程与运维系统 | 不把 prompt、文档、token 或原始凭据写入 metrics；trace ID 关联 | 通过 trace ID 排查并按数据保留策略清理。 |

## 可信攻击路径

1. **泄露的机器凭据继续访问。** API Key 是一次性显示，存储的是
   HMAC；管理员可撤销或立即轮换。轮换成功后旧 Key 不能认证，见
   [credential rotation runbook](../operations/credential-rotation.md)。
2. **数据库泄露后离线猜测 Key。** 数据库只保存 peppered HMAC；pepper
   独立保存在部署方 secret manager。高熵 Key 和限权角色降低影响，但
   不能替代数据库、主机和 secret manager 的访问控制。
3. **错误的生产配置启用 mock 或弱 secret。** `ORAG_ENV=production`
   拒绝 demo/弱/相同的 JWT 与 pepper、mock provider/mock storage、debug
   和非 HTTPS 本机公共 URL。
4. **跨租户或跨项目越权。** 仓储和 HTTP 授权要求 tenant/project 归属；
   不可访问资源以 `404` 表示，避免泄露存在性。
5. **日志或诊断泄露敏感数据。** 生产配置错误只指明配置项而不回显值；
   metrics 使用受控低基数 label。问题报告不得含真实凭据或客户数据。

## 可验证演练

执行以下命令会启动完全隔离的 PostgreSQL、Qdrant 和 explicit mock API，
创建并轮换 tenant-admin API Key，然后验证 source `401`、replacement
`200`：

```bash
make credential-rotation-drill
```

它只在所有断言通过后写入
`.tmp/credential-rotation-drill/run.*/drill-evidence.json`。证据仅包含
构建 revision、公开 Key ID、HTTP 状态和 cutover 标记；不包含 token、完整
Key、pepper、环境变量、请求或响应 body。

## 残余风险与非声明范围

- 此本地演练不证明生产零停机、RTO/RPO、provider credential 吊销、主机
  入侵防护或 secret-manager 权限设置。
- JWT 或 server pepper 轮换不是 HTTP API 操作；它们会导致既有 token 或
  所有 API Key 失效，必须使用维护窗口和
  [服务端轮换 runbook](../operations/credential-rotation.md)。
- 独立部署的恢复、轮换和威胁模型审阅证据仍是生产试点退出门槛。
