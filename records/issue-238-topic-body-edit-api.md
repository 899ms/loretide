# Issue #238 · 已建选题卡五项正文编辑 API

## 任务与基线

- **Issue**: #238 `feat: 已建选题卡五项正文编辑 API`
- **基线**: `19fd36ab61262338b858fc6d8da0162000902e5c`
- **分支**: `codex/topic-body-edit-api`
- **所属模块**: `topic-planning`
- **范围**: 仅后端合同、存储、handler/router、定向测试、规格增补和本记录；不改前端、迁移、桌面端、执行器或诊断模块。
- **Draft PR**: [#240](https://github.com/899ms/loretide/pull/240)，目标 `app-main`；回滚只需 revert 本记录所述的应用提交。

## 冻结的 HTTP 合同

`PATCH /api/content-topics/{id}/body`

请求体只允许下列五个可选字符串字段：

```json
{
  "audience_problem_judgment": "",
  "ip_fit": "",
  "timing": "",
  "existing_content_relation": "",
  "evidence_gaps_and_investment": ""
}
```

- 字段**省略**：保留该列的现有值。
- 字段为**空字符串**：将该列清空；与创建合同相同，允许如实说明没有。
- `null`、非字符串、未知字段、空对象或多余 JSON 值：400，且不写入任何列或审计成功事件。
- 成功：200，返回完整既有 `TopicCard` 形状；响应不新增字段。
- 路径只使用 `chi.URLParam(r, "id")`；工作区和操作人仍只由既有 `topicScope` / `workspace-core.Authorize` 决定。
- 外工作区或不存在对象保持既有不可区分的 404 形状。

## 存储与并发语义

- 新 `PatchBody` 只允许上述五列；不得接受或写入 `account_id`、`fit_source_ids`、`evidence_source_ids`、`channels`、`recommended_action`、状态/decision 字段，或任何简报/开始快照字段。
- 在既有 workspace-delete 栅栏事务中先写审计，再以一条 `UPDATE` 做每列的条件赋值；不进行读出整行再全量回写。
- 两个请求分别更新不同字段时，后提交者基于锁后当前行只写它提供的列，因此不得覆盖先提交者已经写入的其他字段。
- 审计写入、正文更新与提交同事务；审计失败或更新失败均回滚。

## 验证与人工边界

- 纯逻辑合同测试本机可运行：五字段各自编辑/清空/省略，以及 `null`、类型错误、未知键、空对象和尾随 JSON。
- Store/handler/真实路由/并发与快照不变测试使用 CI 托管隔离 PostgreSQL；本机不连接数据库、不运行迁移或服务。
- 无页面改动、无 UI 单测、无 computer use 验收。后续页面卡自行提供人工 TODO。

## 已交付文件、用途与回滚

| 文件/区域 | 用途 |
| --- | --- |
| `server/internal/content/topic-planning/contract.go` | `TopicBodyPatch` 的严格五字段、部分更新合同 |
| `server/internal/content/topic-planning/store.go` | 删除栅栏事务内的条件 `UPDATE` 与同事务审计 |
| `server/internal/content/topic-planning/body_patch_test.go` | 五字段各自编辑/清空/省略与畸形请求的纯逻辑合同测试 |
| `server/internal/content/topic-planning/store_test.go` | 写栅栏的纯逻辑回归覆盖 PATCH |
| `server/internal/content/topic-planning/store_integration_test.go` | CI PostgreSQL 下的冻结对象、审计失败回滚和不同字段并发写入 |
| `server/internal/handler/content_topic.go` | 严格请求解析及受权 HTTP 入口 |
| `server/internal/handler/content_topic_fence_test.go` | CI 下删除栅栏拒绝 PATCH 后无写入 |
| `server/cmd/server/router.go` | 专用 PATCH 路由挂载 |
| `server/cmd/server/content_topic_routes_test.go` | 真实 router 的挂载、401/404、路径 id 与部分更新回归 |
| `server/scripts/verify-db-test-target.sh` | Go 启动前的数据库身份门禁；允许三个已由 CI 矩阵派生的 suite，仍拒绝未知 suite。 |
| `scripts/test-go-db.sh`、`scripts/test-go-db.test.sh`、`.github/workflows/loretide-content.yml` | 将 `topic-planning` 接入第三个 GitHub 派生隔离数据库套件；CI 必须看到三条新增 store 用例各自 `pass`，skip/缺失不能由纯逻辑 pass 掩盖 |
| `docs/development/testing-database-suites.md` | 区分 handler/cmd-server 的 Go fail-closed 守卫与 topic-planning 仅经 wrapper 才受保护、直接运行会 skip 的历史夹具行为 |
| `specs/030-topic-source-refs/` | 将 #238 的正文编辑切片与既有 Out of Scope 记录关联 |

回滚方式：revert 本 Issue 的应用提交；不涉及数据迁移。

## 验证记录

- 先红：`go test ./internal/content/topic-planning -run '^TestTopicBodyPatch' -count=1` 在合同类型尚不存在时编译失败；实现后同一合同与 `TestWritesFailClosedWithoutTheWorkspaceFence` 已通过。
- 已通过：`go vet ./internal/content/topic-planning ./internal/handler ./cmd/server`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`、`git diff --check`。
- 收尾审计已补强：PATCH 的第二 JSON 值检查收敛到专用 decoder，避免扩大既有 topic 入口的拒绝行为；纯逻辑测试逐一覆盖五字段编辑、清空和其余字段省略；CI PostgreSQL 套件新增审计写入失败时卡片更新回滚的必经 `pass` 断言。
- 未在本机运行：`cmd/server` 的测试二进制会在未配置 `LORETIDE_DB_TESTS=1` 时 fail-closed，故没有绕过隔离数据库保护；PostgreSQL store、handler、真实路由、冻结与并发用例由 GitHub Actions 新增的 `topic-planning` 独立 suite 与既有 `cmd-server` suite 实跑，PR 创建后以实际 CI 为准。
- CI 运行 `35694778730`：`boundaries` 已通过；三个数据库 job 在 runner 启动前被 GitHub 以账户付款/消费额度限制拒绝（包括未改动的 handler 与 cmd-server），因此**没有**数据库用例的通过或失败结论。这是外部账户阻塞，不能替代托管隔离验证；恢复额度后应重跑该 PR 的 CI。
- CI 运行 `35698601257`：`boundaries`、`handler` 与 `cmd-server` 已通过；`topic-planning` 在执行测试前失败。实际日志显示 `server/scripts/verify-db-test-target.sh` 把 CI 已派生的 `topic-planning` 误判为 unknown suite，故 JSON 证据文件不存在是早期门禁失败的结果，不是被弱化或绕过。已将它加入同一白名单；Git Bash 假 `psql` 身份模拟确认该 suite 通过原有身份比对，未知 suite 仍被拒绝。本机没有连接数据库。

## 后续页面切片（仅计划，未实施）

**依赖**：本 PR 合入且后端路由可用后另立页面卡；本卡不提前改前端、不写 UI
单测、不做 computer use 验收。

| 文件 | 后续职责 |
| --- | --- |
| `packages/core/content/topic-planning/contract.ts` | 新增仅含五个可选字符串键的 `TopicBodyPatchInput` 及 wire 转换；只序列化 `!== undefined` 的键，保留 `""` 作为清空，绝不夹带账号、来源、状态、动作或简报字段。 |
| `packages/core/api/client.ts`、`packages/core/api/client.test.ts` | 新增 `patchContentTopicBody(topicCardId, body)`，使用 `PATCH /api/content-topics/{id}/body` 与 `encodeURIComponent`；断言方法、路径和五键 wire 形状。 |
| `packages/core/content/topic-planning/queries.ts` | 新增 `usePatchContentTopicBody`：成功后失效 `topicPlanningKeys.all(workspaceId)`，不做乐观写入。 |
| `packages/views/content/topic-planning/index.tsx` | 在 `TopicCardDetail` 旁新增独立正文编辑区，只呈现五项；以当前详情初始化草稿，未改字段省略，提交中禁用保存，成功才重置草稿，失败保留草稿。 |
| `packages/views/locales/{en,zh-Hans,ja,ko}/common.json` | 为编辑区、保存中、保存成功与通用失败状态补齐四语言文案。 |

**错误恢复合同**：接口成功响应仍走 `parseTopicCard`；不允许前端猜测成功或本地改写
缓存。`400`（客户端请求合同错误）、`404`（既有不可区分的不可访问/不存在）与 `5xx`
都要沿用 `saveOutcome(error)` 显示失败、保留用户草稿并不清空当前详情；只有成功回调才
失效查询并显示保存成功。页面不得提供 `channels`、`recommended_action`、账号、来源、
状态、decision 或简报/快照的编辑控件。

**后续人工验收 TODO（未执行）**：

- [ ] 修改五项中任意一项并保存；刷新后只有该项变化。
- [ ] 清空任一项为 `""` 后保存；刷新后仍为空；不提交的其余四项保持原值。
- [ ] 以错误输入/断网等可复现失败路径保存；出现失败提示，草稿不丢、详情不伪更新。
- [ ] 访问无权工作区的卡；沿用既有不可访问提示，不泄露卡是否存在。
- [ ] 检查账号、两组来源、渠道、推荐动作、状态/decision、简报和开始快照均没有编辑入口且值未变。
- [ ] 在英文、简体中文、日文、韩文检查字段标签、保存中和失败文案；由主任务在浏览器逐项确认。
