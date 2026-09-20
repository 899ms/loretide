# Contract: 运营规则（029）

**Spec**: [../spec.md](../spec.md) ｜ **Status**: 裁决后定稿（主控 2026-09-20，PR #192 评论）

本文件写死**形状**：键名、取值、默认值、三态、端点、拒绝体。规格说「要什么」，这里说「长什么样」。

---

## 1. 存储：工作区 `settings` 的两个键 + 账号 `settings` 的一个键

裁决 Q1=A：**没有新表，没有迁移。** 沿用 `loretide.timezone` / `loretide.auto_precheck` / `loretide.scope` 已经用了三次的那套形状。

### 1.1 工作区键 `loretide.operating_rules`

一个键，不是四个。四项设置是同一次编辑里填的同一张表，拆成四个键只会让「整体替换」这件事要防四遍。

```json
{
  "loretide.operating_rules": {
    "cadence":     { "xiaohongshu": 3, "wechat_mp": 0 },
    "templates":   { "xiaohongshu": { "note": "标题 20 字内，首图 3:4" } },
    "review_rule": "self",
    "observation": { "default": 14, "by_channel": { "douyin": 7 } }
  }
}
```

| 字段 | 类型 | 缺省 | 说明 |
|---|---|---|---|
| `cadence` | `{platform: int}` | `{}` | 每周目标条数。**键缺席 = 未设；`0` = 这周不发。** 两者不同 |
| `templates` | `{platform: {note: string}}` | `{}` | 每渠道一段字段约束说明。自由文本 |
| `review_rule` | `"self"` | `"self"` | 受控集**恰好一项**（FR-017） |
| `observation.default` | `int` | 缺席 | 全局「发布后 N 天」 |
| `observation.by_channel` | `{platform: int}` | `{}` | 按渠道覆盖全局 |

**`platform` 的取值是 `ip-profile` 的八个**（裁决 Q2=A）：`xiaohongshu` `douyin` `wechat_mp` `bilibili` `zhihu` `weibo` `kuaishou` `shipinhao`。其中只有 `xiaohongshu` / `wechat_mp` / `douyin` / `shipinhao` 今天能交付——**界面要说这件事**（FR-012a）。

### 1.2 账号键 `loretide.homepage`

```json
{ "loretide.homepage": "https://www.xiaohongshu.com/user/profile/xxx" }
```

字符串，缺省 `""`。**只存不访问**（FR-014）。只放行 `http` / `https`。

与 `loretide.scope` 同一层、同一套读写形状。**不改 `content_account` 表。**

---

## 2. 「未设」与零值：三个函数，不是一个

这是本卡最容易悄悄出错的地方，019 已经在布尔上踩过一次。**每个可缺省的数值字段都要有这三个**：

```
ReadCadence(rules, platform)     -> (value int, stored bool)
ReadObservation(rules, platform) -> (days int, source "channel"|"global"|"none")
```

- **`stored == false` 时 `value` 无意义**，调用方不得使用。返回两个值而不是一个 `*int`，是为了让「忘了看第二个返回值」在 Go 里至少是显眼的。
- `ReadObservation` 的 `source` 是三态而不是布尔：**「这个渠道自己设了 7 天」和「这个渠道没设，用全局的 14 天」是两件不同的事**，界面要能说出是哪一种。

---

## 3. 观察时点派生：恰好三态

```
ObservationDue(publishedAt *time.Time, days int, hasDays bool, now time.Time) Due
```

```
Due = "passed" | "not_yet" | "unknown"
```

| 情况 | 答案 |
|---|---|
| `hasDays == false`（渠道与全局都没设） | **`unknown`** |
| `publishedAt == nil`（025 允许为空） | **`unknown`** |
| `publishedAt + days <= now` | `passed` |
| 其余 | `not_yet` |

**`unknown` 不是失败，也不是 `not_yet` 的别名。** 没有起算点就没有答案；往任何一边猜都是在造事实。这一条与 025 的 `version_match` 三态、027 的「空 ≠ 0」是同一条底线的第三次出现。

**没有默认天数。** 模块里不得出现任何写死的天数常量——一条检索用例钉住（SC-009）。

---

## 4. 端点：自己的合并端点，不是 PATCH 上的字段

`PATCH /api/workspaces/{id}` 与 `PATCH /api/content-accounts/{id}` 的 `settings` 都是**整体替换**（`content_account.go:290` 的 `params.Settings = settings`）。LT-009 与 019 是在**前端**合并的（`withAutoPrecheck`）；LT-014 换了做法，给 `scope` 开了自己的端点并在**服务端**合并，router.go 里那行注释写着原因：

> Its own endpoint rather than a field on PATCH: PATCH replaces the settings blob wholesale, so this merges server-side instead (LT-014).

**本卡照 LT-014，不照 LT-009。** 前端合并意味着每个调用方都要先读一遍完整 settings 再写回去——中间隔着一次网络往返，两个人同时改就会互相抹掉。

| 方法 | 路径 | 做什么 |
|---|---|---|
| `GET` | `/api/operating-rules` | 读本品牌的四项设置（读时填默认，不回写） |
| `PUT` | `/api/operating-rules` | **服务端合并**写回 `loretide.operating_rules`，其余 settings 键逐字节不动 |
| `PUT` | `/api/content-accounts/{id}/homepage` | 服务端合并写 `loretide.homepage` |

**路径参数**：`/api/operating-rules` 两条**没有**路径参数（品牌由 `X-Workspace-ID` 决定），工作流第 12 步对它们没有对象——**这件事要在 PR 正文写明，不是默默跳过**。`/homepage` **有**路径参数，必须有一条穿过真实 router 与中间件、**参数值 ≠ 上下文值**的用例（LT-011/012/013 的教训）。

---

## 5. 拒绝体

沿用 `workspace-core` 的 `RefusalStatus` / `RefusalBody`：**越权与不存在同为 404，同类拒绝体逐字节相同**（`trace_id` 除外）。

400 是诊断错误对象且**点名字段**：

| 情况 | 点名 |
|---|---|
| `cadence` 值为负 | `cadence.<platform>` |
| `platform` 不在八个里 | `cadence` / `templates` / `observation.by_channel` 里出问题的那个键 |
| `review_rule` 不是 `self` | `review_rule` |
| `observation` 天数为负 | `observation.default` / `observation.by_channel.<platform>` |
| 主页链接不是 `http` / `https` | `homepage` |

---

## 6. 模型上下文只收渠道说明

§3.2 原文：「模型上下文仅接收必要的渠道说明」。

```
TemplateNoteFor(rules, platform) string
```

**只有 `templates[platform].note`。** 不含账号名、不含主页链接、不含目标条数、不含观察时点、不含审核规则。一条用例断言组装出的上下文里不出现其余任何字段（SC-006）。

---

## 7. 不做什么（都有守卫用例）

| 不做 | 守卫 |
|---|---|
| 不存平台凭据 | 扫源码与接口定义，无 password / token / cookie / secret 类字段；**且断言模块确实有写入路径**（否则空模块也绿） |
| 不访问主页链接 | 模块与页面都无 HTTP 客户端调用；链接不渲染成会自动取回的元素 |
| 不调度、不提醒 | 无定时器、cron、通知发送路径 |
| 不发明默认天数 | 模块里无写死的天数常量 |
| 不提供成员选择 | 无任何成员选择控件，**包括禁用的** |
| 不调模型、不起执行器 | 照既有三条 |

---

## 8. PR 3：接进 027

PR 3 改 `feedback-learning`。它的依赖表里**有 `workspace-core`**，所以它可以直接 import 本卡的派生函数——不需要读源文件对表，也不需要改登记表。

```
NeedsRegistration(publicationStatus string, metricCount int, due Due) bool
```

| `due` | 行为 |
|---|---|
| `passed` | 按原有两条件判断 |
| `unknown` | **按原有两条件判断**——判断不出到期，不等于这条不用补录 |
| `not_yet` | 不在待补录里 |

**`unknown` 走 `passed` 的分支，不是 `not_yet` 的。** 这是整个 PR 3 最容易做反的一处：把「不知道到没到期」当成「还没到」，会让每一条没有发布时间的记录从工作台上静悄悄消失，而它们恰恰是最需要有人去看一眼的。

守卫 `TestThePendingDerivationHasNoTimeLogic` 改成：

- 模块里的时间比较**允许存在**；
- 但比较用的天数**必须来自参数或设置**，**不得是字面量**；
- 一次可编译变异（把天数写成 `14`）必须让它变红（SC-014）。
