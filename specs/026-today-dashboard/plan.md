# Implementation Plan: 今日工作台（SOP §2 入口页）

**Spec**: [spec.md](./spec.md)（PR #158，`aa50ce7`）
**Issue**: #159
**Branch**: `claude/impl-026-today-dashboard`，base `app-main`

## 一处必须先解决的约束：派生口径放哪里

Issue #159 写「`packages/core/content/today/`（或按登记表允许的位置）」。**`packages/core/content/today/` 不可行**，而且不是风格问题：

1. `scripts/check-content-boundaries.mjs:131` 对三个 root 下**未登记的模块名**直接报 `unregistered module`。`today` 不在 `scripts/content-boundaries.json` 的 12 个模块里。
2. 退一步，把它塞进某个已登记模块也不行：工作台要同时读 `topic-planning`、`work-editor`、`review-delivery`、`ip-profile` 四家，而**没有任何一个已登记模块同时依赖这四个**——
   - `project-collab` = workspace-core / topic-planning / work-editor / review-delivery / diagnostics（**缺 ip-profile**）
   - `agent-workflow` = workspace-core / ip-profile / knowledge-base / topic-planning / work-editor / diagnostics（**缺 review-delivery**）

   要让任一个成立都得改 `modules` 依赖表，而 FR-021 与派单都禁止。

**采用的落点**：`packages/core/today/`，在 `packages/core/content/` **之外**，且**不 import 任何内容模块**——输入用本地声明的结构化类型。这样它在边界检查里既不是模块（无 owner），也没有跨模块 import，天然合规；同时它真的是纯函数，符合「纯函数」的本义。

类型漂移由**适配器页**兜住：页面把真实的 `TopicCard` / `Artifact` / `ReviewRequest` / `DeliveryTask` / `ExpressionProfile` 传进这些函数，TypeScript 的结构化兼容会在那里报错。**typecheck 就是这层的证明**。

**登记表的改动**：只往 `adapters` 加一条新页面（新建页面无法不加——适配器之外的文件 import 内容模块 index 会被 `check-content-boundaries.mjs:141` 拒绝）。`modules` 依赖表**一个字不动**，与 FR-021 的字面一致（它禁的是「依赖表」）。

## 文件清单

### 新增

| 文件 | 内容 |
|---|---|
| `packages/core/today/types.ts` | 结构化输入类型 + 区块输出类型。零 import。 |
| `packages/core/today/sections.ts` | 六个区块的派生口径纯函数 + 排序 + 上限 10。 |
| `packages/core/today/sections.test.ts` | node 测试（`// @vitest-environment node`）。 |
| `packages/core/today/index.ts` | 重导出。 |
| `apps/web/app/[workspaceSlug]/(dashboard)/today/page.tsx` | 适配器：组合六个区块，用已有 hooks。 |
| `specs/026-today-dashboard/manual-ui-todo.md` | 手验项。 |
| `specs/026-today-dashboard/plan.md` / `tasks.md` | 本文件与任务表。 |

### 修改

| 文件 | 改动 |
|---|---|
| `packages/core/package.json` | `exports` 加 `./today` |
| `scripts/content-boundaries.json` | `adapters` 加新页面一条（**`modules` 不动**） |
| `packages/views/locales/{en,zh-Hans,ja,ko}/common.json` | `contentToday` 文案块 |

**清单外的改动一律进 PR 正文单列。**

## 区块与数据来源

| # | 区块 | hooks | 派生口径（core 纯函数） |
|---|---|---|---|
| 1 | 值得写的选题 | `useContentTopics` + 每个关联账号 `useAccountProfile` | `worthWritingTopics`：仅 `draft`；理由摘要；预计投入取账号 `weekly_hours`，`status !== "confirmed"` → 未确认，无关联账号 → 未关联 |
| 2 | 正在推进的作品 | `useContentWorks` + 每个作品 `useContentArtifacts` | `worksInProgress`：至少一份文档 `draftStatus === "working"` |
| 3 | 需要处理的审核 | `useContentReviews`（不带 status，一次取回） | `reviewsNeedingAttention`：`pending` 与 `changes_requested` |
| 4 | 到期交接与待登记 | `useContentDeliveries`（不带 status） | `deliveriesNeedingAction`：直接用服务端读时派生的 `due` / `pendingRegistration`，**不自行重算** |
| 5 | 待补录的反馈 | 无 | 无。`feedback-learning` 不存在 → 恒定「暂不可用」 |
| 6 | 账号配置缺项 | `useContentAccounts` + 每个账号 `useAccountProfile` | `accountsMissingConfig`：`readiness.can_start === false`，逐项列 `missing[]` |

**区块 1 与区块 6 共用同一批 `useAccountProfile`**（同一 query key，React Query 自动去重），符合 Assumptions 2「不要各取一遍」。

## 关键取舍

1. **审核与交付一次取回、在 core 里筛**，而不是按 status 发多次请求。`useContentReviews` 的 `status` 只接单值，发两次请求会让「待处理」的定义散在调用点上；FR-021a 要求这个判定在 core。
2. **`due` / `pendingRegistration` 直接用服务端的布尔**（FR-007）。core 只做筛选与排序，**不重算到期规则**——重算就是第二份真相。
3. **`weekly_hours` 的 `pending` 必须显示「未确认」**（FR-010b）。把一个待补充的数字当成预计投入，比不显示更糟。
4. **上限 10 由一个函数统一施加**（`capSection`），不是每个区块各写一遍 `.slice(0, 10)`——否则总数会和列表脱钩（FR-021b 要求同源）。
5. **单条失败标注而非消失**（FR-017）：区块 1 / 2 / 6 的 N+1 里，某个 profile / artifacts 请求失败时，该条带失败标记留在列表里。

## 不做的事

- 不新增表、不新增端点、不改 `modules` 依赖表、不设默认落地页、不加侧边栏入口。
- 不写 UI 单测（宪法 II）。不引入实时推送（FR-020）。
- **工作流第 12 步不适用**：本卡没有新增带路径参数的端点（FR-019）。这一条在 PR 正文明确记为「不适用及原因」，不默默跳过。
