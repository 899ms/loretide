# Issue #244 · 选题详情五项正文编辑页面

## 任务与边界

- **Issue**: #244 `选题详情：五项正文编辑页面`
- **基线**: `cea2bb43befa3b24aa70af86f5d3dcbea3a81f81`
- **分支**: `codex/topic-body-edit-ui`
- **依赖**: #238 的 `PATCH /api/content-topics/{id}/body` 已合入。
- **范围**: 只编辑 `audience_problem_judgment`、`ip_fit`、`timing`、`existing_content_relation` 与 `evidence_gaps_and_investment`；不编辑账号、来源、渠道、动作、状态、冻结简报或快照。

## 交付文件与用途

| 文件 | 用途 |
| --- | --- |
| `packages/core/content/topic-planning/contract.ts` | 定义五个可选 camelCase 正文键，并仅为 `!== undefined` 的键生成 snake_case PATCH wire body；`""` 保留为清空意图。 |
| `packages/core/api/client.ts` | 新增 `PATCH /api/content-topics/{id}/body`，对 card id 使用 `encodeURIComponent`。 |
| `packages/core/api/client.test.ts` | 纯 Node API 边界测试：断言 PATCH 方法、编码路径与只含本次修改键的 payload。 |
| `packages/core/content/topic-planning/queries.ts` | 新增非乐观的 `usePatchContentTopicBody`；成功后失效该工作区的选题查询前缀。 |
| `packages/views/content/topic-planning/index.tsx` | 在既有 Settings 组合中增加五项正文编辑区；草稿只保留实际变化键，保存中锁定，成功清除草稿，失败保留草稿，并按 card id 隔离草稿。 |
| `packages/views/locales/{en,zh-Hans,ja,ko}/common.json` | 为编辑区标题、说明和保存按钮增加四语言文案。 |

## 错误恢复与兼容性

- 不做乐观缓存写入，也不以 PATCH 响应覆盖页面中的整张卡；成功仅失效查询，避免未知或新响应形状覆盖未编辑字段。
- `400`、`404`、`5xx` 和传输失败均复用既有 `saveOutcome(error)`，保留本地草稿并显示失败状态；只有成功才清除草稿。
- 某字段改回当前卡的值会从 patch 删除，因此不会发送无实际变化的字段。空字符串仍会保留在 patch 中，表示清空。

## 验证与人工验收边界

- 已通过：`pnpm --filter @multica/core exec vitest run api/client.test.ts`（纯 Node，105 项）、`pnpm check:content-boundaries`、`git diff --check`。
- 未运行 UI 单测、Playwright、computer use、服务、迁移、真实执行器或任何本机数据库连接；全量 `pnpm typecheck` 仅检查 TypeScript，不启动应用或数据库。

**用户手验 TODO（未执行，未完成前不得关闭 Issue）**：

- [ ] 分别编辑五项中的任意一项并保存；刷新后只改动该项。
- [ ] 分别将五项清空为 `""` 后保存；刷新后该项仍为空，其他四项不变。
- [ ] 修改后再改回原值；保存按钮应恢复禁用，不能发送无变化字段。
- [ ] 制造可复现的保存失败（400、404、5xx 或断网）；失败提示出现且草稿保留。
- [ ] 在两张不同选题卡之间切换；草稿不得串到另一张卡。
- [ ] 另一会话改动未编辑字段后再保存本卡一项；未编辑字段不得被覆盖。
- [ ] 用无权限工作区访问；沿用不可区分的拒绝表现，不泄露卡是否存在。
- [ ] 检查英文、简体中文、日文、韩文的编辑区文案与既有 Settings 布局；账号、来源、渠道、动作、状态、简报和快照没有新增编辑入口。

## 回滚

仅需 revert 本卡应用提交；没有迁移、数据库数据、服务或生产配置变更。
