# Feature Specification: 账号资料范围偏好（LT-014）

**Feature Branch**: `claude/spec-018-lt014-account-scope-preference`
**Created**: 2026-09-15
**Status**: Draft
**Input**: `tasks/todo.md` LT-014——「定义 local/web/all 范围，账号首次默认 all，保存上次开始界面的选择；先提供可测试设置接口，开始界面在 EP-04 接入」

追踪 R-023 / R-025、开发基线 §5。**CP-05 的最后一块。**

---

## Current State（以代码为准，2026-09-15 于 `app-main` `77fd852` 核实）

| 事实 | 核实方式 | 结论 |
|---|---|---|
| 账号有 `settings` jsonb | 迁移 `477`；`ipprofile.Account.Settings map[string]any` | **偏好的落点**，零迁移 |
| 工作区时区已用同一手法 | `server/internal/handler/workspace.go:212` 的 `workspaceTimezoneKey = "loretide.timezone"`、`validateTimezoneSetting`、`timezoneFilled` | 写时校验、**读时填默认且不回写**——本卡逐条照抄这个形状 |
| 前端镜像 | `packages/core/workspace/timezone.ts` 的 `TIMEZONE_SETTINGS_KEY` / `DEFAULT_TIMEZONE` / `withWorkspaceTimezone` | core 类型照此写 |
| `Patch.Settings` 是**整体替换** | `content_account.go` 的 `UpdateAccount`：`params.Settings = settings` | 只发一个键会**抹掉其余 settings**——这决定了 Q2 |
| 运行快照已有两个字段 | `diagnostics/contract.go:55`：`Scope string \`json:"source_scope"\`` 与 `Preference string \`json:"saved_preference"\`` | **独立字段已经存在**，本卡不造第二套 |
| 「复现不回写偏好」已有断言 | `diagnostics/reproduce_preference_test.go` 的 `TestReproduceDoesNotWriteBackTheSavedPreference`（D13-V10 / spec 008 G5），用 `"strict-sources-only"` 这个夹具永不产生的值，避免「没写」与「写了刚好相同」混为一谈 | 本卡补的是**账号那一侧**：运行与复现不得改动账号存着的偏好 |
| 夹具默认值 | `simulator.go:21` 与 `shapes.go:164` 都是 `Scope:"all", Preference:"all"` | 与本卡的默认 `all` 一致，不需要改夹具 |
| 账号端点已挂载 | `router.go` 的 `/api/content-accounts`（`#79`） | 新增子路由落在同一分组内 |
| `settings` 今天没有任何已定义键 | `grep -rn "loretide\." server/internal/content/ip-profile/` → 无 | `loretide.scope` 是第一个 |

**上面每一行都能用左列的命令在 `77fd852` 上重现。** 没核实的没有写进这张表。

---

## 三个范围是什么意思

| 值 | 含义 |
|---|---|
| `local` | 只用本地已有资料 |
| `web` | 只用联网检索到的资料 |
| `all` | 两者都用 |

**首次默认 `all`**：一个从没选过的账号，读出来就是 `all`，而不是空串或 `null`。空值会把「还没选」这个状态推给每一个调用方各自处理一遍，而开始界面（EP-04）需要的是一个能直接放进单选框的值。

**这里不定义范围怎么被执行。** 本卡交付的是「存住并读回这个选择」，谁去按它取资料是 EP-04 之后的事。

---

## 与诊断快照的关系：对齐，不另起一套

「异常/复现继续使用独立字段、不能回写偏好」这句验收，在代码里已经有一半了：

- `Snapshot.Scope`（`source_scope`）= **这一次运行实际用的范围**，是快照上的独立字段；
- `Snapshot.Preference`（`saved_preference`）= 运行发生时账号存着的偏好，**记录用**；
- `TestReproduceDoesNotWriteBackTheSavedPreference` 已经守住「复现不回写快照上的 `saved_preference`」。

本卡要补的是**另一半**：运行和复现也不得改动**账号 `settings` 里存着的那个值**。两者是两个存储位置，现有断言只覆盖了前者。

**因此本卡不新增任何范围字段、不新增快照字段。** 「运行快照接口可表达固定值」这条验收，说的是 `Snapshot.Scope` 本来就是一个普通字符串字段，可以被钉成一个与账号偏好**不同**的值——本卡加断言证明这一点成立，而不是加一个新接口去实现它。

---

## Clarifications

### Session 2026-09-15

三个问题已按推荐值暂定实施，**不阻塞**，主任务可随时改判（见文末）。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 —— 没选过的账号读出来是 `all`（P1）

新建一个账号，谁也没碰过它的范围偏好。读它，得到 `all`。

**为什么是 P1**：EP-04 的开始界面要拿这个值渲染单选框。没有默认值，那个界面第一次打开就是三个都没选中的状态。

**验收**
1. 新建账号立即读 → `all`；
2. **读不写库**：读完之后 `settings` 里仍然**没有** `loretide.scope` 这个键（与工作区时区同样的规则——响应是完整的，存储是原样的）；
3. `#71` 之前建的账号（`settings` 为 `{}` 或 `null`）读出来同样是 `all`。

### User Story 2 —— 选了就存住，下次还是它（P1）

把账号 A 的范围设成 `local`。再读，是 `local`。重启服务、换一台客户端，仍然是 `local`。

**验收**
1. 设置后读到的是设置的值；
2. 值在服务端，不在任何本地存储；
3. 账号原有的其他 `settings` 键**没有被抹掉**——这一条单独有用例，因为 `Patch.Settings` 是整体替换（见 Q2）。

### User Story 3 —— 账号之间互不影响（P1）

改 A 的范围三次，B 的范围**纹丝不动**（B 没选过就仍是 `all`）。

### User Story 4 —— 非法值被拒，且拒得清楚（P1）

把范围设成 `everything`、空串、数字、或大小写不符的 `Local`。

**验收**
1. 一律 **400 + 诊断错误对象**（不是数据库约束错误漏出来）；
2. 拒绝之后**存储没有变化**——一个写了半截的 `settings` 比一次被拒的编辑糟糕得多；
3. 拒绝的判断**不只在专用端点上**：从 `PATCH /{id}` 的 `settings` 里塞一个非法 `loretide.scope` 同样被拒，否则专用端点的校验形同虚设。

### User Story 5 —— 跑一次诊断运行，账号的偏好不动（P1）

账号 A 的偏好是 `local`。跑一次模拟运行，再跑一次复现。

**验收**
1. 运行结束后 A 的 `settings` 里还是 `local`；
2. 运行自己的快照带的是它**实际用的**范围（`source_scope`），与账号偏好可以不同；
3. 这条用一个**夹具永不产生的偏好值**来断言，和 `reproduce_preference_test.go` 的手法一致——否则「没写」和「写了刚好相同」分不开。

### User Story 6 —— 越权读写与不存在一模一样（P2）

另一个品牌的成员拿着账号 id 来设范围：404，且与「这个账号不存在」**逐字节相同**。

### Edge Cases

- `settings` 是 `null` / 不是对象 / 是数组 → 读作 `all`，不崩；
- `loretide.scope` 存着一个非法值（被别的写入路径塞进去的）→ **读作 `all`**，不把坏值传出去，也不顺手改库；
- 同一账号并发两次设置 → 后写的赢；本卡不引入版本号，范围偏好不是需要历史的东西（与人设提示词不同，LT-012 那个才需要）；
- 设成与当前相同的值 → 正常成功，仍记一条审计事件（「谁在什么时候确认过这个选择」本身是信息）。

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**：范围偏好 MUST 取值于受控集合 `local` / `web` / `all`；其余一律拒绝。
- **FR-002**：偏好 MUST 存在 `content_account.settings` 的 `loretide.scope` 键下；本卡 MUST NOT 新增迁移。
- **FR-003**：没有存过值的账号读 MUST 得到 `all`。
- **FR-004**：读 MUST NOT 写库——补默认只发生在响应上。
- **FR-005**：写入 MUST 保留账号 `settings` 里的其他键。
- **FR-006**：非法值 MUST 回 400 与诊断错误对象，且 MUST NOT 产生任何存储变化。
- **FR-007**：非法值的拒绝 MUST 同样覆盖 `PATCH /{id}` 携带的 `settings`。
- **FR-008**：一个账号的偏好变化 MUST NOT 影响另一个账号。
- **FR-009**：越权 MUST 经 `workspacecore` 回 404，与不存在无法区分。
- **FR-010**：每次写入 MUST 经诊断记一条审计事件。
- **FR-011**：诊断运行与复现 MUST NOT 改动账号存储的偏好；本卡 MUST 为此加断言。
- **FR-012**：本卡 MUST NOT 新增快照字段——`Snapshot.Scope` 与 `Snapshot.Preference` 已存在，且 `Scope` 可被钉成与账号偏好不同的固定值。
- **FR-013**：范围的受控集合 MUST 在 Go 与 core 之间同源，并有测试比对（沿用 `#79` 的平台常量手法）。
- **FR-014**：新增路由 MUST 有存在性用例（工作流第 12 步）。
- **FR-015**：本卡 MUST NOT 新增界面；开始界面在 EP-04 接入。
- **FR-016**：本卡 MUST NOT 写 UI 单测（constitution 原则 II）。

### Key Entities

- **Account.Settings**（已有）：新增一个约定键 `loretide.scope`，值为受控字符串。**不加列、不加表。**
- **Snapshot.Scope / Snapshot.Preference**（已有）：运行侧的独立字段，本卡只加断言，不改形状。

---

## Success Criteria *(mandatory)*

- **SC-001**：新建账号读到 `all`，且读后库里仍无该键。
- **SC-002**：设成 `local` 后读到 `local`，其他 `settings` 键完好。
- **SC-003**：改 A 三次，B 不变。
- **SC-004**：四种非法值全部 400 且存储未变；经 `PATCH settings` 的同样被拒。
- **SC-005**：跑运行 + 复现后账号偏好不变（用夹具不产生的值断言）。
- **SC-006**：越权 404 且与不存在逐字节相同。
- **SC-007**：Go 用例三计数、core vitest 逐文件、`typecheck --force`、三项 check 全过。
- **SC-008**：变异三处各自使对应用例变红。

---

## UI Impact

**无。** 本卡只交付接口与 core 类型；开始界面在 EP-04。LT-013 的账号页**不加**这个字段（Q3）。

---

## Assumptions

1. **不记历史。** 范围偏好是「上次选的是什么」，不是需要回溯的配置版本。人设提示词需要版本是因为运行要钉住快照（LT-012）；范围偏好的历史在运行快照里已经逐次记着了（`saved_preference`）。
2. **不做并发保护。** 后写的赢。这与 LT-012 的 409 不同，是因为那里丢一次写意味着丢一次创作确认，这里只是丢一次单选框的选择。
3. **`local` / `web` / `all` 的执行语义不在本卡。** 本卡只保证存得住、读得回、不被误改。
4. **新增平台式的扩展成本**：将来若要加第四个范围值，Go 常量加一个、core 常量加一个，比对测试会同时红——没有迁移要改，因为这里没有 CHECK 约束（`settings` 是 jsonb，约束在应用层）。

---

## 待澄清问题（已按推荐值暂定，不阻塞）

### Q1 偏好存在哪？

- **A（推荐，已暂定）**：`content_account.settings` 的 `loretide.scope` 键。**零迁移**，与 `#63` 的工作区时区**同一手法**（写时校验 / 读时填默认 / 读不回写），前端也已有 `timezone.ts` 这个可照抄的镜像。
- **B**：`content_account` 加一列。需要一次迁移；换来的是可以按范围建索引——而今天没有任何查询按范围过滤。
- **C**：新表。范围偏好是账号的一个字段，不是一个实体；建表等于给一个字符串配一张表和一套生命周期。

### Q2 写入走什么接口？

`Patch.Settings` 是**整体替换**（`content_account.go` 的 `UpdateAccount` 直接 `params.Settings = settings`），所以只发 `{"loretide.scope":"local"}` 会把账号其余 `settings` 全部抹掉。这不是理论风险，是当前代码的行为。

- **A（推荐，已暂定）**：加一个专用端点 `PUT /api/content-accounts/{id}/scope`，**在服务端读-改-写地合并**。理由：任务卡要的就是「先提供可测试设置接口」；合并发生在服务端，客户端不可能忘；非法值 400、审计事件、越权 404 都有一个明确的归属点。代价是新增一条路由（按第 12 步补存在性用例）。
- **B**：不加路由，复用 `PATCH /{id}`，在 core 加一个 `withAccountScope(settings, scope)` 让**客户端**合并。零新增路由，但把「别忘了合并」这件事交给每一个调用方；而且两个客户端同时改不同的 settings 键时，后到的那个会用自己读到的旧 blob 覆盖对方——服务端合并没有这个问题。
- **无论选哪个**，`PATCH` 路径上的 `loretide.scope` 都要校验（FR-007），否则专用端点的校验可以被绕过。

### Q3 LT-013 的账号页要不要顺手露出这个字段？

- **A（推荐，已暂定）：不加。** 任务卡写明「开始界面在 EP-04 接入」，账号页多一个控件既没有验收也没有手动清单条目，而且它属于「开始一次运行」的语境，不属于「账号资料」的语境。
- **B**：在账号页加一个单选。好处是本卡立刻可见；代价是 EP-04 到来时很可能要把它挪走，届时两处都得改。
