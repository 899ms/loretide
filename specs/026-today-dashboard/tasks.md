# Tasks: 今日工作台（SOP §2 入口页）

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md) · **Issue**: #159

## Phase 1: 落点裁定

- [x] T001 确认 `packages/core/content/today/` 不可行并记录理由（未登记模块；无任何已登记模块同时依赖 topic-planning / work-editor / review-delivery / ip-profile），落点改为 `packages/core/today/`，**零内容模块 import**
- [x] T002 确认只需往 `adapters` 加新页面一条，`modules` 依赖表不动

## Phase 2: core 派生口径（先写测试）

- [x] T003 `packages/core/today/types.ts`：结构化输入类型，**不 import 任何内容模块**；注意 `ip-profile` 是 snake_case（`can_start` / `weekly_hours`）而其余三家是 camelCase
- [x] T004 [P] `sections.test.ts` 先写：区块 1 只收 `draft`，`started` / `saved` / `deferred` / `dropped` 各一条负例（**`deferred` 必须有独立负例**，它是裁决里唯一被点名排除的）
- [x] T005 [P] `sections.test.ts` 先写：预计投入三分——`confirmed` 给数值、`pending` 给「未确认」、无关联账号给「未关联」
- [x] T006 [P] `sections.test.ts` 先写：区块 2 只收「至少一份文档 `working`」的作品；全 `saved` 的作品不收；**无文档的作品不收**
- [x] T007 [P] `sections.test.ts` 先写：区块 3 收 `pending` 与 `changes_requested`，`approved` / `rejected` / `cancelled` 各一条负例
- [x] T008 [P] `sections.test.ts` 先写：区块 4 直接用 `due` / `pendingRegistration`，**给一条 `scheduled` 且 `scheduledAt` 已过但服务端 `due=false` 的用例，断言 core 不自行重算**（这条是 FR-007 的守卫）
- [x] T009 [P] `sections.test.ts` 先写：区块 6 收 `can_start === false` 并逐项保留 `missing[]` 原字段名
- [x] T010 [P] `sections.test.ts` 先写：`capSection` 上限 10 且 `total` 是**截断前**的总数；11 条时 `shown=10, total=11, hidden=1`
- [x] T011 `sections.ts` 实现上述，令 T004–T010 转绿
- [x] T012 每个区块的排序在 core 里定义并有用例（不靠后端返回顺序）
- [x] T013 `index.ts` 重导出；`packages/core/package.json` 的 `exports` 加 `./today`

## Phase 3: 页面（适配器）

- [x] T014 `apps/web/app/[workspaceSlug]/(dashboard)/today/page.tsx`：组合六个区块，**只挂既有组件**，不新增控件、不改样式
- [x] T015 每区块独立加载、独立失败；任一区块失败不影响其余（FR-015）
- [x] T016 单条失败**标注而不静默消失**（FR-017）：区块 1 / 2 / 6 的 N+1 里某条读取失败时带失败标记留在列表
- [x] T017 区块 5「待补录的反馈」：**存在、暂不可用、写明原因**（`feedback-learning` 未落地）；不隐藏、不加载态、不伪造条目（FR-005b）
- [x] T018 区块 6 标题按本义写「账号配置缺项」，排在 §2 五项之后，**不冒充第五项**（FR-005a）
- [x] T019 区块 3 / 4 条目**不可点并写明原因**（`review-delivery` 页面未落地，FR-022a）；核对 `packages/views/content/review-delivery` 是否已存在，存在则直接接上
- [x] T020 选题区块的「候选自动生成暂不可用（EP-04c 未落地）」恒定可见，0 条伪造推荐（FR-011）
- [x] T021 上限 10 的「还有 N 条」与跳转入口（FR-021b）
- [x] T022 `scripts/content-boundaries.json` 的 `adapters` 加这一条；**`modules` 不动**
- [x] T023 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`

## Phase 4: 收尾

- [x] T024 `manual-ui-todo.md`：界面项全部进去；核对 `packages/views` 与 `apps/web` 下**没有本卡新增的 `.test.tsx`**（宪法 II）
- [x] T025 `pnpm typecheck --force` + 三项 check（`check:content-boundaries`、`check:diagnostics-contract`、`generate:reserved-slugs` 无 diff）+ core 与 views vitest
- [x] T026 变异验证**三处**，每处确认对应用例变红、**改完即还原**，变异必须**可编译**：
      (M1) 让区块 1 也收 `deferred` → T004 变红；
      (M2) 让区块 4 自行按 `scheduledAt < now` 重算到期 → T008 变红；
      (M3) 让 `capSection` 的 `total` 变成截断后的条数 → T010 变红
- [x] T027 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T028 PR 正文：落点裁定（为什么不是 `core/content/today/`）、登记表只动 `adapters`、**第 12 步不适用及原因**、UI 影响（复用了哪些既有组件）、手验 Todo、与 #154 的顺序依赖

## Dependencies

```text
T001,T002 落点
  └─ core (T003 → T004..T010 先写 [P] → T011 → T012 → T013)
       └─ 页面 (T014 → T015..T021 → T022 → T023)
            └─ 收尾 (T024 → T025 → T026 → T027 → T028)
```

**T004–T010 必须先写并确认失败。**

## Implementation Strategy

1. **T008 是本卡最容易被"简化"掉的一条**。区块 4 看起来「自己判断一下 `scheduledAt` 有没有过」更直接，但那会造出第二份真相，而服务端的 `due` 才是按 SOP 9.1 定义的那一份。用例必须构造「服务端说 `due=false` 但时间已过」，否则重算与不重算的代码都是绿的。
2. **`capSection` 的 `total` 要在截断前取**（T010）。写成截断后，页面上会出现「共 10 条」而实际有 37 条——比不显示总数更糟，因为它看起来是对的。
3. **单条失败与空态是两件事**（T016）。N+1 里某条 profile 读失败时，如果那条就从列表消失，「今天没有事」和「读不到」在界面上完全一样。
4. **区块 5 不是占位符是交付物**（T017）。照 024 三个 AI 入口的口径：存在、不可用、写明原因。
