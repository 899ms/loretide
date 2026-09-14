# Quickstart: 验证 008 的六项缺口是否真的闭合

本文件是**验证指南**，不含实现代码。每项都给出「怎么跑」和「凭什么算通过」。

## 前置

```bash
pnpm install                       # 全量，不要用 --filter，否则 apps/docs 缺 node_modules
```

后端集成用例需要一个本地 PostgreSQL（**不要用开发库**）：

```bash
export LORETIDE_DIAG_TEST_DATABASE_URL="postgres://postgres@127.0.0.1:15433/loretide_diag_ci?sslmode=disable"
export MULTICA_TEST_DATABASE_URL="postgres://postgres@127.0.0.1:15433/multica_handler_test?sslmode=disable"
```

未设这两个变量时相关用例会跳过或失败，**不要**把跳过当通过。

## G1 / G2 — 跳转推导（纯函数）

```bash
pnpm --filter @multica/core exec vitest run content/diagnostics/linkage.test.ts
```

**逐个文件指定，不要用 `content/diagnostics/` 目录通配**——通配会把原则 II 排除的 `queries.test.tsx` 一并跑掉。

通过标准：
- `describeTraceJump` 的 6 行边界表（contracts/trace-jump.md）全部有对应用例且通过。
- 关键一条：`traceId` 为空 → `unavailable`，**且**断言它不返回 filter。只断言 `reason` 不够。
- `describeObjectVersions` 的 6 行边界表全部通过，含「同 id 不同 type 不合并」。

## G3 — 下一动作穷尽性

```bash
cd server && go test ./internal/content/diagnostics -run 'TestNextAction' -count=1 -v
```

通过标准：A1 ～ A5 五条断言（contracts/next-action.md）用例名逐条可见且 PASS。

**变异验证必做**：按 contracts/next-action.md 的四处改动逐一验证会变红，改完即还原。没做变异验证的，不要在对照表里写「已闭合」。

## G4 — 快照无媒体载荷

```bash
cd server && go test ./internal/content/diagnostics -run 'TestSnapshotHasNoMedia' -count=1 -v
```

通过标准：字段清单断言覆盖 16 个字段名与类型。

**变异验证**：给 `Snapshot` 临时加一个 `Blob []byte` 字段 → 必须变红；再加一个合法的 `string` 字段 → **也必须变红**（这是字段清单相对「只查类型」的全部价值所在）。两次都改完即还原。

## G5 — 复现不回写偏好

```bash
cd server && go test ./internal/content/diagnostics -run 'TestReproduce' -count=1 -v
```

通过标准：
- 原运行的快照偏好用**非夹具值**（不能是 `"all"`）——否则断言分不清「真的没改」和「碰巧相等」，FR-015。
- 复现后重读原运行，断言其偏好与其它快照输入字段均未变。

**变异验证**：让复现路径写回原运行的偏好 → 必须变红。

## G6 — 跨层链条端到端

```bash
cd server && go test ./internal/handler -run 'TestContentDiagnosticLinkage' -count=1 -v
```

**必须带 `-v` 并逐条核对用例名。** `handler_test.go` 的 `TestMain` 在数据库连不上时执行 `os.Exit(0)`，整个包会以退出码 0「通过」而一个用例都没跑。只看退出码会得到假绿。

通过标准：
- 三跳（审计→追踪→技术事件、技术事件→运行、运行→原故障运行）逐跳断言标识符对得上。
- **空标识符不当通配**的负例存在且通过——`store.go:163` 的 SQL 是 `($4='' OR ...)`，空值即不加条件，拿空 trace 查会返回全部事件。没有这条负例，「链条走通」可能只是每跳都匹配到了一堆无关记录。

## 全特性回归

```bash
pnpm check:content-boundaries      # 新增 core 文件不得越界
pnpm check:diagnostics-contract    # 接入合同未被破坏
pnpm typecheck
cd server && go test ./internal/content/diagnostics ./internal/handler -count=1 -race -v
```

`middleware` 包有 17 条与本特性无关的 SKIP（需要本地未起的 Redis），属预期，不要当作失败，也不要写成「无 SKIP」。

## 不在本指南内

- **浏览器行为**：J-1 ～ J-4、O-1 ～ O-3 见 `manual-ui-todo.md`，按原则 II 只能由用户手动确认，**不写自动测试、不用浏览器自动点击**。
- **真实执行器**：按原则 IX 保持禁用，本特性全部走模拟器。
