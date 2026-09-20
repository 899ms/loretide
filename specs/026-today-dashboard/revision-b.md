# 026 修订 B：收件箱接入今日工作台，以及导航的去向

**Spec**: [spec.md](./spec.md)（#158） · **Issue**: #188 · **裁决**: PR #190 评论，已回写
**Branch**: `claude/spec-026b-today-inbox-nav`，base `app-main` @ `94bd89a`
**只含规格修订。上游文件一个未动，全部留给实施 PR。**

> **本文件初版有一处事实错误，已删除。** 初版写「spec.md 的 FR-005b 已经过时，工作台显示的是一句不实的话」，并据此提了 FR-B-07。**这是错的**：#177 已把 FR-005b 改成真实数据源（`GET /api/content-feedback/pending`）并划掉旧的验收场景，#184 也已把页面的 `FeedbackSection` 接到 `usePendingFeedback` + `pendingFeedbackSummary`。规格与页面都是对的，没有任何要补的。
>
> 错因：我凭写 #158 时的记忆引用 FR-005b，没有重读当前文件。**「我写过的条款」和「现在的条款」不是一回事。**

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

→ **FR-B-01**，已立 **Issue #191**，**在本修订的实施之前单独修**（裁决指定的顺序）：经 `paths.ts` 给内容页加方法（`upstream:` **纯新增**单列）、改用 slug、路径构造加 node 测试。

主控独立 grep 过：**只有 `today/page.tsx` 有这个问题**，其余内容页没有。我在 `app-main` @ `94bd89a` 上复核一致——12 处（10 × topics、2 × accounts），全仓其它位置 0 处。

---

## Current State（以代码为准，2026-09-20 / `app-main` @ `94bd89a`）

### 今日工作台现有六个区块

顺序被 spec.md 的 FR-005 钉死（SOP §2 原文五项），FR-005a 把「账号配置缺项」定为**排在五项之后的第六个附加区块**：

1 值得写的选题 → 2 正在推进的作品 → 3 需要处理的审核 → 4 到期交接与待登记 → 5 待补录的反馈 → 6 账号配置缺项（附加）

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

### 区块位置与导航

- **FR-B-08**: 快速采集入口 MUST 在**页头**，待整理 MUST 是**第七个附加区块**，排在「账号配置缺项」之后（裁决 Q1 = A）。`spec.md` 的 FR-005a MUST 同步改为「附加区块两个」——§2 五项的顺序不受影响。
- **FR-B-09**: 侧栏 MUST 按 **S1** 落地：一个「内容」分组、五个入口（`/today` `/sources` `/topics` `/accounts` `/diagnostics`）（裁决 Q2）。MUST 只复用上游侧栏组件，MUST NOT 新增控件或配色。七个上游文件的改动 MUST 单列 `upstream:` 提交且**既有行零删除**。
- **FR-B-09a**: 新图标名 MUST 在实施 PR 里**先给建议再定**（裁决）。至少 `/today` 与 `/sources` 需要新的 `RouteIconName`——`Inbox` 与 `ListTodo` 已分别被 `/inbox` 与 `/issues` 占用，复用会让两个入口长得一样。
- **FR-B-10**: 本卡 MUST NOT 改 `paths.ts` 的 `root()`（裁决 Q3 = A）。`/today` 是否设为默认落地页，等侧栏入口落地且其手验项真的验过之后再议。

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

## 裁决记录（主控，PR #190 评论）

| 问题 | 裁决 | 落到哪里 |
|---|---|---|
| **Q1** 收件箱位置 | **A**：页头「快速采集」入口 + **第七个附加区块**。`spec.md` 的 **FR-005a 改为「附加区块两个」** | FR-B-02 / FR-B-03 / FR-B-08 |
| **Q2** 侧栏 | **S1**：五个入口一个「内容」分组；`upstream:` 单列且**既有行零删除**；**图标名在实施 PR 先给建议再定** | FR-B-09 / §3 |
| **Q3** 默认落地页 | **A**：暂不改 `root()` | FR-B-10 |

**FR-005b 不在本卡范围**——初版对它的判断是错的（见文件开头的更正）。

### 实施顺序（主控指定）

1. 本修订回写后合并；
2. **先修缺陷**：从 `origin/app-main` 起 `claude/fix-026-today-nav-slug` 修 **Issue #191**——经 `paths.ts` 给内容页加方法（`upstream:` **纯新增**单列）、改用 slug、路径构造加 node 测试；
3. 再做 #188 的实施（本修订的 FR-B-02 ~ FR-B-06、FR-B-09）。

## 不做的事

不新增表、不新增端点、不改登记表 `modules`、不写 UI 单测、不碰执行器。本修订**不动任何文件**，上游改动全部留给实施 PR，按 FR-B-09 单列 `upstream:` 且既有行零删除。

`plan.md` / `tasks.md` 随实施 PR 产出。
