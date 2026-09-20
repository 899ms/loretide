# 026 修订 B：收件箱接入今日工作台，以及导航的去向

**Spec**: [spec.md](./spec.md)（#158，裁决见其「裁决记录」一节） · **Issue**: #188
**Branch**: `claude/spec-026b-today-inbox-nav`，base `app-main` @ `94bd89a`
**只含规格修订。侧栏与默认落地页涉及上游文件，本卡按第 13 步列出最小改动与替代方案，不实施。**

---

## 0. 先报一个缺陷：今日工作台上 12 个跳转全是坏的

**这是 #165 我自己写的代码，已经合进 `app-main`。** 本卡是导航题，查资料时撞上的。

`apps/web/app/[workspaceSlug]/(dashboard)/today/page.tsx` 里 12 处这样写：

```ts
navigation.push(`/${wsId}/topics`)     // 10 处
navigation.push(`/${wsId}/accounts`)   //  2 处
```

而 `wsId` 来自 `useWorkspaceId()`：

```ts
// packages/core/hooks.tsx:13
export function useWorkspaceId(): string {
  const ws = useCurrentWorkspace();
  ...
  return ws.id;          // ← UUID
}
```

路由段是 **`[workspaceSlug]`**，解析只认 slug：

```ts
// packages/core/workspace/queries.ts:34
export function workspaceBySlugOptions(slug: string) {
  return queryOptions({
    ...workspaceListOptions(),
    select: (list) => list.find((w) => w.slug === slug) ?? null,   // ← 只比 slug
  });
}
```

所以点「查看全部」或任何一条的「打开」，去的是 `/<uuid>/topics`。`workspaceBySlugOptions` 找不到工作区 → 落到 `NoAccessPage` 或被弹回。**今日工作台上没有一个跳转是通的。**

为什么测试没拦住：026 按宪法 II 不写 UI 单测，跳转目标全在手验项里（026-U-22「跳转正确」），而手验项**至今一条都没执行过**。这正是宪法 X「打勾不等于验收」说的那件事——我在 #165 的 PR 正文里写了「026-U-22 跳转正确」，它一直是「未执行」。

**正确写法**（仓库自己的规矩，`paths.ts` 开头写着「All navigation in shared packages MUST go through this module — no hardcoded string paths」）：

```ts
const slug = useRequiredWorkspaceSlug();          // @multica/core/paths
paths.workspace(slug).topics()                    // 需要先给 paths.ts 加这些方法（见 §3）
```

→ **FR-B-01**。修它要么等 §3 的 `paths.ts` 方法，要么先用 `useRequiredWorkspaceSlug()` 拼字符串过渡。**实施 PR 必须带上这条，而且它比本卡的新功能更紧急。**

---

## Current State（以代码为准，2026-09-20 / `app-main` @ `94bd89a`）

### 今日工作台现有六个区块

顺序被 spec.md 的 FR-005 钉死（SOP §2 原文五项），FR-005a 把「账号配置缺项」定为**排在五项之后的第六个附加区块**：

1 值得写的选题 → 2 正在推进的作品 → 3 需要处理的审核 → 4 到期交接与待登记 → 5 待补录的反馈 → 6 账号配置缺项（附加）

> **spec.md 的 FR-005b 已经过时**：它说第五项「待补录的反馈」显示「暂不可用（反馈记录接入后）」。#177 / #184 已把 `feedback-learning` 落地（存储、接口、页面都在）。**本修订不改它**——那是 027 的接入卡该做的事，但必须记下来，否则工作台会一直显示一个假的「暂不可用」。→ **FR-B-07**

### source-inbox 已可读（#176 / #182）

- `GET /api/content-sources?status=&tag=`，`status` 受控 `inbox` / `organized` / `archived`。
- core hooks：`useContentSources(wsId, status, tag)`，契约 `packages/core/content/source-inbox/contract.ts`。
- 页面 `/{slug}/sources` 已落地，新建入口在页内「收一条」区块。

「待整理」= `status = inbox`。工作台要的两样东西都拿得到，**不需要新端点**。

### 五个内容页全部没有侧栏入口

`/today`、`/sources`、`/topics`、`/accounts`、`/diagnostics` 都只能手输 URL。

### 加一个侧栏入口要动哪些文件（第 13 步评估）

侧栏不是「加一行」。`packages/views/layout/app-sidebar.tsx:110` 的注释写明：

> Nav items reference WorkspacePaths method names so they can be resolved against the current workspace slug at render time. **Only parameterless paths are valid nav destinations.**

链路是：`paths.ts` 的方法名 → `NavKey` → `WORKSPACE_PAGES` 注册段与图标 → `layout.nav.*` 文案 → 侧栏分组数组 → 渲染。**每一环少一个都编译不过或图标解析不到。**

| # | 文件 | 归属 | 改动 |
|---|---|---|---|
| 1 | `packages/core/paths/paths.ts` | **上游** | `workspaceScoped()` 加 5 个无参方法 |
| 2 | `packages/core/paths/route-icons.ts` | **上游** | `RouteIconName` 加图标名（若复用现有则不必）、`NavLabelKey` / `WorkspacePageKey` 各加 5 项、`WORKSPACE_PAGES` 加 5 条 |
| 3 | `packages/views/layout/route-icon-components.tsx` | **上游** | 新图标名 → 组件（`Record<RouteIconName, LucideIcon>` 缺一个就是编译错误） |
| 4 | `packages/views/layout/app-sidebar.tsx` | **上游** | `NavKey` / `NavLabelKey` 各加 5 项、新增 `contentNav` 数组、渲染一个分组 |
| 5 | `packages/views/locales/{4}/layout.json` | **上游** | `nav.*` 加 5 条 × 4 语言 |
| 6 | `packages/views/editor/utils/link-handler.ts` | **上游** | `WORKSPACE_ROUTE_SEGMENTS` 加 5 段 |
| 7 | `packages/core/paths/consistency.test.ts` | **上游** | C4 断言的硬编码清单同步（注释写明「改任一边都要改两边」） |

**七个文件全部是上游 Multica 的。** 这是本卡最大的一处成本，也是它必须先出规格再实施的原因。

替代方案见 §3。

### 默认落地页是一行

```ts
// packages/core/paths/paths.ts:28
root: () => `${ws}/issues`,
```

进入工作区落在哪，就这一行决定。改它**影响每一个用户的每一次进入**，不只是内容功能。

---

## Requirements（本修订新增，编号 FR-B-xx）

### 先修坏的

- **FR-B-01**: 今日工作台的全部跳转 MUST 用 **slug** 而不是 workspace id 构造。实施 PR MUST 修掉现有 12 处，并 MUST 有一条不依赖 DOM 的用例钉住「跳转目标里不出现 workspace id」——否则它会以同样的方式回来。

### §11「随时」行的两件事

- **FR-B-02**: 工作台 MUST 提供**快速采集入口**，一次点击到 `/{slug}/sources` 的新建位置。它 MUST NOT 在工作台上就地创建条目——采集要填类型、正文或链接、批注，把这些塞进工作台会长出第二个采集表单，而 028 已经有一个。
- **FR-B-03**: 工作台 MUST 呈现**待整理收件箱**：`status = inbox` 的条目，上限 **10**（与其余区块一致，FR-021b），超出显示还有多少条并可跳转查看全部。
- **FR-B-04**: 该区块的每一条 MUST 能识别到具体条目（标题 / 链接 / 类型），MUST 写明它被列出的理由（待整理），并 MUST 可跳转到 `/{slug}/sources`。
- **FR-B-05**: 该区块 MUST 独立加载、独立失败（FR-015），空时 MUST 显示明确空态（FR-016）。
- **FR-B-06**: 判定与排序 MUST 是 `packages/core/today/` 的纯函数（FR-021a），页面只渲染。本区块 MUST NOT 新增表或端点。

### 顺带记下的过期条款

- **FR-B-07**: spec.md 的 **FR-005b 已过时**：`feedback-learning` 已于 #177 / #184 落地，第五区块不应再恒显「暂不可用」。**本修订不改它**，由 027 的工作台接入卡处理。在那之前，工作台对「待补录的反馈」的呈现是**已知不实**，MUST 记入 manual-ui-todo 的已知边界。

### 区块位置与导航

- **FR-B-08**: 收件箱区块的位置见 [NEEDS CLARIFICATION: Q1]。
- **FR-B-09**: 侧栏入口方案见 [NEEDS CLARIFICATION: Q2]。无论裁决如何，MUST 只复用上游侧栏组件与既有图标，MUST NOT 新增控件或配色。
- **FR-B-10**: 默认落地页见 [NEEDS CLARIFICATION: Q3]。

---

## Success Criteria（新增）

- **SC-B-01**: 今日工作台上**每一个**跳转都落到正确的工作区页面（12 处逐一点过，0 处进 `NoAccessPage`）。
- **SC-B-02**: 快速采集入口一次点击到达新建位置；工作台上 **0 个**采集表单。
- **SC-B-03**: 待整理区块只列 `status = inbox`；`organized` 与 `archived` 各一条负例不出现。
- **SC-B-04**: 该区块上限 10，超出显示「还有 N 条（共 M 条）」，总数是**截断前**的。
- **SC-B-05**: 收件箱端点 503 时**只有该区块**显示失败，其余区块照常。

---

## §3 侧栏与默认落地页：最小改动与替代方案（第 13 步）

### 侧栏

| 方案 | 做法 | 上游文件数 | 代价 |
|---|---|---|---|
| **S1** | 按上表改七个文件，在侧栏加一个「内容」分组，5 个入口 | **7** | 最完整、和既有导航一致（高亮、桌面标签页、编辑器内链接全都自动对）；但七个文件全是上游，且 `consistency.test.ts` 的硬编码清单要同步 |
| **S2** | 只加 `/today` 一个入口 | **7**（同样七处，只是每处 1 条而非 5 条） | 成本几乎一样——链路是固定的，**加一个和加五个动的是同一批文件**。所以 S2 省不了多少 |
| **S3** | 不动侧栏；在今日工作台页头放一行指向其余四页的链接，`/today` 本身仍靠 URL 直达 | **0** | 零上游改动；但「怎么第一次找到 `/today`」没有答案——等于把问题推给默认落地页（Q3） |
| **S4** | 不动侧栏；等一张「内容侧栏分组」的专卡，连同桌面端标签页一起做 | **0**（本卡） | 把上游改动集中到一张卡里评审；代价是内容功能在那之前始终只能 URL 直达 |

**我的推荐：S1**，理由是 S2 并不更省（链路固定），而 S3 不解决「第一次怎么找到」。**但 S1 必须作为独立的 `upstream:` 提交**，PR 正文单列「上游改动」一节，且既有行零删除。

**若选 S1，图标建议复用已有名**：`Inbox` 已被 `/inbox` 占用，`/sources` 可用 `FileText`；`/today` 用 `ListTodo`？——已被 `/issues` 占用。**图标复用会让两个入口长得一样**，这是 S1 真正的麻烦处：五个新入口里至少 `/today` 与 `/sources` 需要新图标名，也就是 `RouteIconName` 要扩，第 3 个文件必须动。这一点我不替裁决，见 Q2。

### 默认落地页

| 方案 | 做法 | 影响面 |
|---|---|---|
| **D1** | 改 `paths.ts:28` 的 `root()` 为 `${ws}/today` | **每个用户每次进入工作区**。`root()` 还被「切换工作区」「删除工作区后跳转」等处使用——改它不是只改落地页 |
| **D2**（推荐） | **不改 `root()`**。等侧栏有了入口（S1）之后，由主任务观察真实使用再决定 | 零影响。026 的 Q3 = C 本来就是「先做 `/today`，默认落地页另议」，本卡不替它做决定 |
| **D3** | 按用户偏好决定落地页 | 要新增偏好存储与设置项，远超本卡 |

**我的推荐：D2**。把一行改掉很容易，难的是它改的是所有人的第一屏，而 `/today` 的手验项**一条都还没执行过**——在验收之前把它设成所有人的入口，正是宪法 X 要防的。

---

## 待裁决（clarify）

### Q1：收件箱在工作台上是第七区块，还是页头快速入口？

**Context**: FR-B-08。spec.md 的 FR-005 把 §2 五项的顺序钉死，FR-005a 把「账号配置缺项」定为第六个附加区块。§11「随时」行要的是**两样东西**：快速入口 + 待整理收件箱。

| 选项 | 做法 | 代价 |
|---|---|---|
| **A（推荐）** | **两样分开**：页头一个「快速采集」按钮（FR-B-02），待整理作为**第七个附加区块**排在第六之后 | 各归各位：入口是动作，收件箱是清单，§11 那一行本来就是两件事。FR-005 / FR-005a 的顺序不受影响，只是附加区块从一个变两个 |
| B | 合成第七区块，采集按钮放在区块标题旁 | 少一处页头改动；但「随时想收一条」要先滚到页面第七块，快速入口就不快了 |
| C | 只做页头入口，不做待整理区块 | 最小；但 §11 明写「快速入口**与**待整理收件箱」，只做一半 |

**推荐 A。** 附加区块从「一个」变「两个」要同步改 spec.md 的 FR-005a 措辞（它现在写的是「第六个附加区块」）。

### Q2：侧栏加不加，加几个，图标怎么办？

**Context**: FR-B-09、§3 的 S1–S4。关键事实：**加一个入口和加五个入口动的是同一批七个上游文件**，所以「先加一个试试」并不更省。

| 选项 | 做法 |
|---|---|
| **A（推荐）** | **S1，五个全加**，一个「内容」分组；`RouteIconName` 扩两到三个新图标名（至少 `/today`、`/sources` 需要），单列 `upstream:` 提交 |
| B | S1 但只加 `/today`，其余仍 URL 直达 |
| C | S4：本卡不动侧栏，另立一张「内容侧栏分组」的卡，连桌面端标签页一起做 |
| D | S3：不动侧栏，工作台页头放一行指向其余四页的链接 |

**推荐 A**，但**这是本卡上游成本最高的一项，裁决前我不动任何一个文件**。若倾向把上游改动集中评审，选 C 也合理——代价是内容功能在那之前只能 URL 直达。

**附带一问**：新图标名具体用哪几个（lucide 里 `CalendarCheck` / `Inbox` 系列 / `FileStack` 之类），要不要我在实施卡里给一版建议再定？

### Q3：`/today` 设不设为默认落地页？

**Context**: FR-B-10、026 的 Q3 = C 留下的决定。`paths.ts:28` 的 `root: () => `${ws}/issues`` 一行。

| 选项 | 做法 |
|---|---|
| **A（推荐）** | **D2：暂不改**。等侧栏入口落地、`/today` 的手验项真的验过之后再议 |
| B | D1：本卡一并改 `root()` |
| C | D3：做成用户偏好 |

**推荐 A。** `root()` 不只是落地页——切换工作区、删除工作区后的跳转都用它。而 `/today` 的 27 条手验项**至今一条未执行**；把未验收的页面设成所有人的第一屏，正是宪法 X 要防的那类。

---

## 不做的事

不新增表、不新增端点、不改登记表 `modules`、不写 UI 单测、不碰执行器。侧栏与默认落地页**裁决前不动任何上游文件**。FR-005b 的过期问题**不在本卡修**（属 027 的接入卡）。

`plan.md` / `tasks.md` 随实施 PR 产出。
