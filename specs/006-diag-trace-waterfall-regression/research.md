# Research: 006 诊断 trace 瀑布视图与回归关联

Phase 0。Technical Context 无 NEEDS CLARIFICATION——五个原本的未决项已在 spec 的 Clarifications（2026-09-14）由主任务定案。以下是把那些定案落到实现时仍需选定的技术决策。

## D1. 层级构造算法与终止性

- **Decision**：两趟算法。第一趟把 `events` 按 `spanId` 建索引（`Map<string, Event>`）；第二趟对每个 span 沿 `parentSpanId` 向上走，边走边把访问过的 `spanId` 记入一个 `Set`，命中已访问者即判定为环。深度由自顶向下的一次遍历赋予，不在渲染时递归。
- **Rationale**：`Map` 建索引使「父是否存在」为 O(1)，整体 O(n)，满足 Technical Context 的性能约束（禁止 O(n²) 的重复父链遍历）。环检测用访问集合而非深度上限，因为深度上限会把**合法的深层嵌套**误判为环；访问集合只在真的成环时触发。
- **终止性论证**：每次向上走都把一个新 `spanId` 放入 `Set`，`Set` 的容量上界是 span 总数，故最多走 n 步必然终止——要么到达无父节点，要么命中已访问者。FR-003 要求的「保持终止性」由此成立，可对成环输入直接断言函数返回而非超时。
- **孤儿处理**：`parentSpanId` 非空但不在索引中 → 顶层 + `orphan` 标注（FR-001）。`parentSpanId` 为空串 → 正常顶层，不标注。两者必须区分：前者是「父不在本次运行内」，后者是「本来就是根」。
- **Alternatives considered**：递归构造 + 深度上限——会把合法深层嵌套误伤，且爆栈风险仍在，否决。

## D2. 时间轴基准与时钟偏差

- **Decision**：基准点取本次运行全部 span 中最小的 `occurredAt`。每个 span 的 `startOffsetMs = occurredAt - 基准`，宽度取 `durationMs`。当某 span 的 `occurredAt` 早于其父的 `occurredAt` 时，该 span 标注 `clockSkew`，**位置照实**不做夹取。
- **Rationale**：Clarification Q3 定为 C。夹取会让图「看起来正常」而掩盖真实的时钟问题，那正是 `clock_skew` 故障场景要暴露的东西；改按 `sequence` 等距排列则彻底失去「等了多久」这一时间轴的唯一价值。
- **实现细节**：`occurredAt` 在 `eventSchema` 中是 `z.string()`（ISO 文本），需 `Date.parse` 转毫秒。解析失败（非法时间串）时该 span 标注 `invalidTime` 并按基准点定位，不得产生 `NaN` 偏移——`NaN` 会让整张图的宽度计算全部塌陷。
- **Alternatives considered**：以 `receivedAt` 为准（选项 D）——`receivedAt` 是落库时刻，不反映执行时序，会把批量写入显示成同时发生，否决。

## D3. 折叠选择策略（200 上限）

- **Decision**：当 span 数超过上限时，先标记「必保留集」= 所有 `errorCode` 非空的 span ∪ 它们各自的完整祖先链；再按 `startOffsetMs` 升序从剩余 span 中补足到上限；未入选者按其最近的**已入选祖先**聚合为一个 `collapsed` 占位行，携带被折叠条数 N。
- **Rationale**：Clarification Q2 定为 C。按时间顺序直接截断的失效方式很具体——失败往往发生在运行末尾，正好被切掉，而看瀑布的首要目的就是找失败步骤。先保失败节点及其父链，保证「展开前就能看见失败在哪一层」。
- **不变量**：折叠后任一可见行的父节点要么可见、要么是顶层——不得出现悬空片段（FR-004）。这条必须有独立用例，因为它是「折叠不破坏层级」的全部含义。
- **Alternatives considered**：按深度截断（选项 D）——深度浅但数量多的运行（例如 200 个平行 span）完全不受控，否决。

## D4. 四态判定的取值来源

- **Decision**：判定只读 `Run` 的 `regression` / `expectedCode` / `actualCode` / `status` 四个已有字段，映射为 `passed` / `failed` / `not_run` / `undecidable`。映射规则：`regression === ""` → `not_run`；`regression === "passed"` 且 `status` 不指示未完成 → `passed`；`regression === "failed"` → `failed`；其余一切（未知枚举值、`passed` 与 `status` 冲突）→ `undecidable`，并原样带出原值。
- **Rationale**：Clarification Q1 定为 A，后端不动。把「未知取值」和「状态冲突」都归入 `undecidable` 而非各自成态，是因为二者对使用者的含义相同（「这个结论不能信，去看原值」），分开只会让界面多一个没人能区分的标签。
- **安全方向**：任何不确定都必须倒向「不是通过」。这是 D13-V09 末句的字面要求，也是本功能唯一可能造成错误验收结论的地方，因此写成不变量：`verdict === "passed"` 当且仅当上述精确条件成立。
- **Alternatives considered**：把 `status` 冲突单列为第五态——增加界面复杂度而不增加可操作性，否决。

## D5. 折叠状态的归属

- **Decision**：折叠 / 展开是组件本地 `useState`，不进 Zustand、不持久化。
- **Rationale**：`CLAUDE.md` 的 State Rules 允许持久化「durable preferences / drafts / layout」，但折叠状态是**单次查看某一次运行时的瞬时视图状态**，切到别的运行即失效，属于「ephemeral UI state」，明确不该持久化。同时 constitution 原则 IV 要求 Zustand 只装跨组件的客户端状态——这里没有第二个消费者。
- **Alternatives considered**：进 Zustand 以便切换标签页后保持——会把瞬时状态写进共享 store，违反 State Rules 的「Do not persist ephemeral UI state」，且需要按 runId 维护清理逻辑，收益不抵成本，否决。

## D6. 上限常量的复用方式

- **Decision**：从 `packages/core/content/diagnostics/contract.ts` 导出的 `STREAM_EVENT_CAP` 取值，瀑布直接引用同一常量，不声明第二个字面量 200。
- **Rationale**：Clarification Q2 明确「同一常量或同值」。引用同一常量而非复制数值，可避免将来调整实时流上限时两处失步——这类重复常量的漂移是典型的静默不一致。
- **核实**：`packages/core/content/diagnostics/contract.ts` 第 23 行 `export const STREAM_EVENT_CAP=200`，已被 `mergeEvents`（第 24 行）与面板（`index.tsx` 第 619–620 行「仅显示最近 200 条」）使用。已导出，本功能只增加一处引用，`contract.ts` 不需要改动。
- **Alternatives considered**：为瀑布单独定义 `WATERFALL_SPAN_CAP = 200`——两个常量语义不同但取值相同，将来必然有人只改一个，否决。

## D7. i18n 文案

- **Decision**：新增文案键沿用现有 `diagnostics.textNNN` 的编号续接方式，四语言（`en` / `zh-Hans` / `ja` / `ko`）同步新增，由 `packages/views/locales/parity.test.ts` 保证不漏。
- **Rationale**：该 parity 测试已在 CI 中运行（`loretide-content.yml`），漏译会直接失败，无需额外机制。中文文案须对照 `conventions.zh.mdx` 的术语表。
- **注意**：四态标签的中文措辞需要避免「通过」单独出现造成误读——沿用面板既有 `text086` 的口径（「通过表示模拟结果符合该故障预期」），在标签旁保留该限定语。
