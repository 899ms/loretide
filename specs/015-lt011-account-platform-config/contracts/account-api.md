# Contract: 品牌账号 API 与存储

## 这个 Account 是什么，不是什么

> **是**：品牌下的**内容渠道账号**——在某个平台上发内容的那个号。
> **不是**：登录账号（上游 `user`）。
> **不是**：IM 机器人安装（上游 `channel_installation`，那是挂在 `agent_id` 上的飞书/Slack 应用凭据）。

「Account / ChannelAccount 是同一实体」：**一个实体、一张表**，不做两层。

## 存储

表 `content_account`，列全部 `text` / `jsonb` / `timestamptz`，与 `content_diagnostic_run` 等同层表一致：

| 列 | 类型 | 说明 |
|---|---|---|
| `account_id` | `text` PK | 与诊断三表已有的 `account_id text` 同类型——那些列本就是为这个实体预留的 |
| `workspace_id` | `text` | 所属品牌。**无外键**；隔离靠应用层过滤 + 授权助手 |
| `platform` | `text` | 受控枚举，带 `CHECK` 兜底 |
| `display_name` | `text` | 去空白后非空 |
| `settings` | `jsonb` | 表达/内容形式设置。本卡最小切片；完整字段落地时**不需要加列** |
| `created_at` / `updated_at` | `timestamptz` | |

**无外键、无级联。** 索引 `(workspace_id)` 用独立文件的 `CREATE INDEX CONCURRENTLY`，一文件一语句。

`CHECK` 是列约束，**不是**外键也**不是**级联——不在数据库硬约束的禁止之列。

## 平台枚举（首版 8 个）

`xiaohongshu` `douyin` `wechat_mp` `bilibili` `zhihu` `weibo` `kuaishou` `shipinhao`

**Go 侧枚举为准**（产出 400 诊断错误对象），**库 `CHECK` 兜底**。两处必须一致，有用例逐值比对。

> **新增一个平台需要一次迁移**（改 `CHECK`）。这是纵深防御的代价，写在这里以免加平台时才发现。

## 接口

| 操作 | 方法 | 说明 |
|---|---|---|
| 创建 | POST | 平台 + 显示名 + 可选 settings |
| 读取单个 | GET | 按 id |
| 列表 | GET | 只返回当前空间的，顺序稳定 |
| 更新 | PATCH | 只影响目标账号 |

**本卡不提供删除**——账号停用/删除的语义（历史内容归属）是产品决定，留给后续卡。

### 每个操作都先经授权

调用 `workspacecore.Authorize(...)`；拒绝时用它的**规范映射**：

| 情况 | 响应 |
|---|---|
| 非成员 / 角色不足 / 空间不可用 / 账号不属于本空间 / 账号不存在 | **404** + 诊断错误对象 |
| 未认证 | **401** |
| 平台非法 / 显示名为空 | **400** + 诊断错误对象 |

**404 的五种情况响应必须逐字节相同**——否则拒绝本身就能被用来探测哪些 id 是真的。

拒绝 body 只含 `{error, code, trace_id, component, retryable, next_action}`，**不含任何对象字段**。

## 不变量

| # | 规则 | 断言位置 |
|---|---|---|
| A1 | 同品牌两账号互不影响：改其一，另一个逐字段不变 | `content_account_test.go` |
| A2 | 跨品牌读 → 404，body 无对象字段 | `content_account_test.go` |
| A3 | 跨品牌写 → 404，且目标数据未变 | `content_account_test.go` |
| A4 | 不存在的 id → **与 A2 逐字节相同的响应** | `content_account_test.go` |
| A5 | 非法平台 → 400，未写入 | `account_test.go` + handler |
| A6 | 枚举内每个平台都能创建 | `account_test.go` |
| A7 | Go 枚举与库 `CHECK` 取值**逐值一致** | `account_test.go` |
| A8 | 显示名去空白后为空 → 400 | `account_test.go` |
| A9 | 列表只返回当前空间，顺序稳定 | `content_account_test.go` |
| A10 | 删除工作区 → 账号行清零 | `content_account_test.go` |
| A11 | 新表已登记进删除清单（不在 `unclassified`） | `workspace_delete_manifest_test.go` |

## 变异验证

| # | 改动 | 应变红 |
|---|---|---|
| M1 | 列表不按 `workspace_id` 过滤 | A9、A2 |
| M2 | 更新不校验账号归属 | A3 |
| M3 | 平台校验恒真 | A5 |
| M4 | 跨品牌拒绝改成 403 | A4（与 A2 不再逐字节相同） |
| M5 | 删除工作区不删账号 | A10 |

## 接入合同

`ip-profile` 是第三个落地模块。E1 由 `service.go` import `content/diagnostics` 满足；**E2 由真实接入满足**——账号的创建与更新写审计事件（`Audit`），拒绝写技术事件；E3 由 `account_test.go` 引用 `diagnostics` 满足。

`pnpm check:diagnostics-contract` 应报 **`checked 3 landed modules`**。

> 合同第 5 节：**通过检查 ≠ 接入合格。** 三条只证明痕迹存在；语义由 A1–A11 承担。
