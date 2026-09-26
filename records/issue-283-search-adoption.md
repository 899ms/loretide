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
| `server/internal/content/topic-planning/{search_ports.go,search_suggestion.go}` | 定义 handler 适配器写端口；在预检后记录采用决定，写版本并追加成功/失败效果；重试从存储的决定恢复。 |
| `server/internal/handler/content_search_suggestions.go`、`server/cmd/server/router.go` | 将跨模块写映射到 `work-editor.ApplyBody`，新增决策重试端点。 |
| `packages/core/content/{work-editor,topic-planning/search}` | 更新受控动作集合；提供非乐观采用/重试 mutation，并在成功时失效建议与作品版本/编辑副本查询。 |
| `packages/views/content/work-editor/index.tsx`、四份 `common.json` | `suggestion_applied` 的历史动作显示文案；未做页面验收。 |
| `server/internal/content/*/*guards_test.go` | 保持版本/建议/效果表仅追加写入的源码守卫。 |

## 已验证

- `go test ./internal/content/work-editor ./internal/content/topic-planning -run 'Test(ApplyBody|Version|SourceAndAction|SearchSuggestion)'`：通过；真实库用例因未配置 `LORETIDE_WORK_TEST_DATABASE_URL` 跳过，未把跳过记为通过。
- `go build ./internal/handler ./cmd/server`、`go vet ./internal/content/work-editor ./internal/content/topic-planning`：通过。
- core 定向 Vitest（工作编辑器契约、搜索建议契约）：42 通过；内容边界与诊断契约检查通过。
- `git diff --check`：通过。

## 未执行与待主控远程验收

- 所有真实数据库集成、并发、迁移规则、handler 路由及“新版本不继承终审”测试；由主控在隔离远程数据库执行。
- 全仓 `pnpm typecheck --force` 与 views typecheck 没有在本机 30 秒命令窗口内取得最终退出结论，不能标记通过；核心包类型检查已完成。 
- 手动 UI：`specs/036-search-optimization/manual-ui-todo.md` 的所有项目仍为“未执行”；本 PR 相关为 U-12、U-13、U-15、U-17、U-18，另有 U-30 闭环。不得以自动测试替代。

## 回滚

撤回本 PR 即可移除应用代码。迁移 627 的 down 在已有 `suggestion_applied` 版本时刻意失败；需先由人工确定如何保留该追加历史，不能自动删除或改写版本行。
