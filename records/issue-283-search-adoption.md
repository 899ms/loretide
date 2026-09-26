# Issue #283 · 036 PR3 搜索建议采用与幂等重试

日期：2026-09-26  
分支：`codex/036-search-adoption`  
基线：`09f17a285e129d19b27001d5d2711d2e8fdc7bbd`（`app-main`）  
关联：GitHub Issue #283；依赖 PR #282 已合并。

## 已确认需求

- 仅实现 036 PR3（T056–T079）：`work-editor.ApplyBody`、搜索建议采用与重试、动作值 `suggestion_applied`、迁移 627，以及对应 core 契约与工作编辑器动作文案。
- 幂等声明必须先于基础版本/草稿检查；效果未记录时重试必须复用首次版本，且不能额外创建版本或完成效果。
- 删除工作区映射为 404；基础版本移动、未保存草稿、修订不一致以精确 409 字段回应。新版本不得复制审核或改写既有交付。
- 本轮只允许工作编辑器的一条动作文案及英文、简中、日文、韩文四个翻译键；经营诊断命名空间不在范围内。
- 不在本机连接数据库、执行迁移服务或数据库集成测试；不写 UI 单测、不使用 computer use 验收、不启动真实执行器、不发送 Multica 消息。

## 已实施（待完整验证）

| 文件/范围 | 关联与用途 |
| --- | --- |
| `server/migrations/627_content_artifact_version_action_suggestion_applied.*.sql` | 原子替换版本动作 CHECK；down 在已有新动作行时按设计失败，避免改写历史。 |
| `server/internal/content/work-editor/{contract.go,version.go,apply_integration_test.go}` | 新增 `ApplyBody`、三种冲突哨兵与 `suggestion_applied`；沿用唯一 `appendVersion` 路径，幂等 Claim 在状态检查之前；隔离数据库测试覆盖正常写入、重放与三类拒绝。 |
| `server/internal/content/topic-planning/{search_ports.go,search_suggestion.go}` | 定义 handler 适配器写端口；决定已存在时先报 `suggestion_id` 冲突、不会读取文档；在预检后记录采用决定，写版本并追加成功/失败效果；重试从存储的决定恢复。 |
| `server/internal/content/topic-planning/{search_suggestion*_test.go}` | 增补采用预检顺序、删除工作区 fence与失败效果的覆盖；数据库部分只由隔离 CI 执行。 |
| `server/internal/handler/content_search_suggestions.go`、`server/cmd/server/router.go`、相关测试 | 将跨模块写映射到 `work-editor.ApplyBody`，新增决策重试端点；数据库用例覆盖效果未记后的真实版本单次/并发重试且断言重用 ApplyBody 首次提交的确切 version ID（T067/T068）、决策后真实 `SaveVersion` 抢先（T069）、正确 retry 路由路径、嵌套决策响应取值及越权顺序（T070）、工作区删除时 `Apply` 为 404（T071），以及采用后 revision 4、v3 审核与交付整行 JSON 不变、v4 无继承审核且新提审为 pending（T072）。上述数据库用例仅编译，待隔离 CI 执行。 |
| `packages/core/content/{work-editor,topic-planning/search}` | 更新受控动作集合；提供非乐观采用/重试 mutation，并以纯 Node 的 mutation-options 测试验证请求与成功后的缓存失效（不使用 jsdom 或 renderHook）。 |
| `packages/views/content/work-editor/index.tsx`、四份 `common.json` | `suggestion_applied` 的历史动作显示文案；未做页面验收。 |
| `server/internal/content/*/*guards_test.go` | 保持版本/建议/效果表仅追加写入的源码守卫。 |

## 已验证

- `go test ./internal/content/work-editor ./internal/content/topic-planning -run 'Test(AdoptPreflight|ApplyBody|Version|SourceAndAction|SearchSuggestion|SuggestionDecision|AbandonRefusals)'`：通过；真实库用例因未配置 `LORETIDE_WORK_TEST_DATABASE_URL` 跳过，未把跳过记为通过。
- `go build ./internal/handler ./cmd/server`、`go vet ./internal/content/work-editor ./internal/content/topic-planning`：通过。
- core 定向 Node Vitest（工作编辑器契约、搜索建议契约和 mutation 配置）：44 通过；`pnpm --filter @multica/core typecheck` 与全仓 `pnpm typecheck --force` 均取得退出码 0；全仓强制检查 9/9 包成功，包含 views/web/desktop。
- `go test -c` 仅编译 `internal/handler` 和 `cmd/server` 测试二进制后删除临时文件：本轮 T067/T068/T070/T072 断言修订后重新编译通过；没有执行数据库/handler 测试。
- 内容边界与诊断契约检查、`git diff --check`：通过。
- `git diff --check`：通过。
- 远程隔离 CI `36253650381`：boundaries、topic-planning、cmd-server 成功；handler 唯一失败为 T067/T068 并发重试断言。证据显示两个同时开始的请求都可返回 Adopted，但仍只保留 2 个版本；这符合 in-flight 请求完成时读取已提交效果的行为。修正并发结果断言与对应注释后的新 head `3ad3234e`，由远程隔离 CI `36254189731` 全部作业成功；日志确认 T067/T068 retry recovery、T070 retry route/permission 和 T072 review/delivery isolation 均 PASS。本机仍未运行数据库测试。
- GitHub Actions 运行 `36251869254`（旧 head `1e5d6956`）：boundaries 与 cmd-server 成功；topic-planning、handler 失败。日志定位为旧用例仍断言 `adopt` 不可用／应为 400，现已改为新契约；本次修订尚未推送，不能把该运行记为已修复或通过。

## 未执行与待主控远程验收

- 所有真实数据库集成、并发、迁移规则、handler 路由及“新版本不继承终审”测试；由主控在隔离远程数据库执行。`go test ./cmd/server ...` 被项目的数据库保护器在启动前拒绝（未设 `LORETIDE_DB_TESTS=1`），因此没有运行任何路由测试，也不是失败证据。
- 本地未运行真实数据库、handler、路由、迁移或并发集成用例；新增 T067–T072 的隔离用例由主控在专用远程测试数据库执行。路由包的本地测试启动保护器会因未设置 `LORETIDE_DB_TESTS=1` 在任何测试开始前拒绝，因此本地未取得其运行结果。
- 曾有一次以 `127.0.0.1:1` 伪地址尝试数据库迁移测试，因连接失败且违背本轮“不得连接数据库”约束，不作为任何验证证据；后续未再执行该命令。
- 手动 UI：`specs/036-search-optimization/manual-ui-todo.md` 的所有项目仍为“未执行”；本 PR 相关为 U-12、U-13、U-15、U-17、U-18，另有 U-30 闭环。不得以自动测试替代。

## 回滚

撤回本 PR 即可移除应用代码。迁移 627 的 down 在已有 `suggestion_applied` 版本时刻意失败；需先由人工确定如何保留该追加历史，不能自动删除或改写版本行。
