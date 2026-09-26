# Issue #288 · 036 T091 搜索主题至发布观测服务端闭环

日期：2026-09-27  
仓库：`https://github.com/899ms/loretide`  
基线：`18882b61f9ae940aff4f2df44b3085b4f00af6c0`（`app-main`）  
分支：`codex/036-search-lifecycle`  
关联：GitHub Issue #288

## 范围与权限边界

- 本次只完成 036 T091：在 handler 集成测试中覆盖 SC-014 的搜索主题 → 作品版本 1 → 建议采用版本 2 → 版本 2 人工审核与批准 → 交付和人工发布记录 → 搜索曝光指标与单次排名观察的真实服务端链路。
- 按当前 Issue 卡，主控已确认唯一执行者并领取 ready 任务；PR 供主控审查。此执行未合并 PR、未关闭 Issue，也未更改其他分支。
- 附图描述的是 Issue #9 的执行器禁用门禁回归任务；它不是本次 #288 的新实现范围。本次遵循其共同约束：不接触生产代码、不运行生产执行器、不使用 UI/computer use 验收。
- 远程隔离数据库验收由主控执行；本机不连接数据库，不运行迁移/服务。用户已免除手动 UI 验收；这只记为豁免，不代表浏览器功能已通过。

## 文件及用途

| 文件 | 与任务的关联 / 用途 |
| --- | --- |
| `server/internal/handler/content_search_observations_test.go` | 新增 `TestContentSearchLifecycleFromThemeToPublicationObservation`。使用真实 handler 与隔离数据库 fixture，验证搜索主题关联账号/选题卡；创建作品、文档与 v1；采用建议后形成 v2；对 v2 提交并批准审核；创建交付、人工登记发布；登记并读取搜索曝光及关联主题/发布记录的排名观察。检查发布解析器仍解析到 v2，观察值/来源正确，非成员读取被拒绝。无跨模块 fake、无生产实现修改。 |
| `records/issue-288-search-lifecycle.md` | 本次对话记录：固定仓库与基线、范围、证据、未执行事项及关联文件。 |

## 已实施、验证与待办

- 已实施：新增真实 handler/DB 链路用例；测试清理按关联表顺序删除本次 fixture 数据。
- 已验证：`gofmt`；`go test -c ./internal/handler` 成功（仅编译测试二进制，不执行测试）；`git diff --check` 成功。
- 未验证：T091 的实际数据库断言尚未执行；不能将编译成功或 Skip 当作行为通过。等待主控在隔离 CI/数据库串行执行该用例。
- 未执行：本机数据库/迁移/服务、浏览器 UI 与 computer use 验收。UI 项按用户豁免记录；远程数据库结果待回填。
- 限制：仅静态/handler 接口层服务端闭环证据，不证明真实执行器运行时越权或浏览器流程验收（与 LT-004 的边界一致）。

## 回滚

撤回本分支的测试文件变更及本记录即可；没有生产代码或数据库结构变更。
