# Contract: 选题卡与冻结简报

## 选题卡 `content_topic_card`

| 规则 | 内容 |
|---|---|
| T-1 | 字段覆盖 `docs/01` §5.2 七项：写给谁与解决什么问题与拟表达什么判断 / 为什么适合这个 IP / 为什么是现在 / 与已有作品的关系 / 证据缺口与投入 / 适合哪些渠道 / 推荐动作 |
| T-2 | 每一项**允许如实说明「没有」**。MUST NOT 用必填逼出编造内容——§5.2 明写「没有时效依据就直接说明」 |
| T-3 | 状态取值：`draft` / `started` / `saved` / `deferred` / `dropped`（对应四个动作加初始态）。服务端是权威，前端的枚举分支带 `default` |
| T-4 | `deferred` / `dropped` 带**快捷原因**与**自由备注**两列 |
| T-5 | 属于一个品牌（`workspace_id`）；账号引用**可空** |
| T-6 | 写入这些记录 MUST NOT 触碰账号的人设、范围偏好或任何 IP 配置 |

## 简报版本 `content_brief_revision`

照 `content_account_revision`（迁移 479）的形状。

| 规则 | 内容 |
|---|---|
| B-1 | 字段覆盖 §5.3 十一项：受众 / 核心问题 / 主张与边界 / 渠道 / 形式 / 结构 / 引用要求 / 资料范围 / 交付物 / 时间上限 / 成本上限 |
| B-2 | **渠道可以是多个**（§5.3：「简报可以包含多个渠道」） |
| B-3 | **Append-only**：没有 UPDATE 路径，没有 DELETE 路径。唯一的删除是品牌删除 |
| B-4 | `revision_id` 是被外部引用的稳定键，以单独的 `CREATE UNIQUE INDEX CONCURRENTLY` 保证唯一；`revision` 是每张卡自增的计数器，**只给人读与排序**，MUST NOT 作跨表键 |
| B-5 | `UNIQUE (topic_card_id, revision)`，**单独一个 CONCURRENTLY 迁移文件** |
| B-6 | 同一张卡重复「开始」MUST NOT 产生第二份首版 |

## 存储

| 规则 | 内容 |
|---|---|
| S-1 | **无外键、无 `REFERENCES`、无级联**。关系与清理在应用代码里解 |
| S-2 | 每个索引 `CREATE INDEX CONCURRENTLY`，**单独一个迁移文件、单条语句**；建表迁移里不建索引 |
| S-3 | 两张表登记进工作区删除清单，并在删除事务的同一 CTE 链里各一条 `DELETE` |

## 授权

| 规则 | 内容 |
|---|---|
| A-1 | 每个端点经 `workspace-core` 的 `Authorize`，**不自己判成员关系** |
| A-2 | 越权是 **404**，经 `RefusalStatus` / `RefusalBody` |
| A-3 | 带路径参数的端点有一条**穿过真实中间件、路径参数 ≠ 上下文值**的用例 |

## HTTP 端点（PR 1）

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` / `POST` | `/api/content-topics` | 列表 / 手工新建选题卡；列表可带 `?account_id=<id>` 或 `?account_id=none`（只看未关联）筛选 |
| `GET` | `/api/content-topics/{id}` | 读取一张卡；查询同时带 `workspace_id` |
| `POST` | `/api/content-topics/{id}/account` | 关联 / 改关联 / 解除关联账号（Issue #130）；`account_id` 为 `null` 即解除；跨品牌账号按 404 语义拒绝；写入持删除栅栏并记一条 `link-account` 审计 |
| `POST` | `/api/content-topics/{id}/actions` | `start` / `save` / `defer` / `drop`；`start` 同事务创建且只创建一份首版简报 |
| `GET` / `POST` | `/api/content-topics/{id}/briefs` | 版本列表 / 追加版本 |
| `GET` | `/api/content-topics/{id}/briefs/{revisionId}` | 按稳定 `brief_revision_id` 读取旧版 |

简报端点故意没有 `PATCH` / `PUT` / `DELETE`。`revision` 只排序，路径使用稳定的 `revisionId`。

## 接入合同

新模块目录从创建那一刻起受 `docs/development/diagnostics-onboarding-contract.md` 第 2 节约束，落地模块数由 **3** 变 **4**。至少四面：

- **审计**：改变状态的操作在**同一个事务内**调 `Store.AuditTx`；审计写失败即整体回滚；
- **删除栅栏**：每条写入在自己的事务里**先取** `LockWorkspaceForContentDiagnosticWrite`（FOR KEY SHARE），无行即工作区已删、按 404 语义拒绝。`AuditTx` 也取同一把锁，但那只覆盖有审计的路径，不作数（Issue #104）；
- **技术日志**：失败路径产出 `Event`，填 `Component` / `Severity` / `Action` / `Outcome` / `Code`；写失败只计数不拖垮业务；
- **trace**：跨步骤用 `Child(ctx)`，**不自己造 trace id**；
- **脱敏**：进日志或审计的 `Event` 一律过 `Sanitize`；HTTP 边界只用 `RequestIdentity`。

## 最容易悄悄失败的地方

- **端点没挂路由。** #71 / #73 交付了八个账号端点却都没挂进 `router.go`，处理器测试照样全绿。第一个 PR 必须有一条穿过真实路由的用例。
- **建表迁移里顺手建索引。** PostgreSQL 拒绝在多语句串里建并发索引，而迁移运行器不开事务——错误会在应用迁移时才炸。
- **忘了登记删除清单。** `TestWorkspaceDeletionManifestCoversPublicSchema` 会红，#68 踩过一次。
- **把 §5.3 的「运行继续引用旧版」记成已验收。** 今天没有运行可钉版本，结构正确不等于这条验过。
