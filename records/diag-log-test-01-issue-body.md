## 目标
现有 log_test.go 只有一项组合用例。补齐 Sanitize、SlogHandler、LogBuffer 的独立反例和并发保证，不扩大日志采集字段。
## 文件边界
- server/internal/content/diagnostics/log_regression_test.go（新增）
- docs/development/diagnostics-log-regression.md（新增）
- records/diagnostics-log-regression.md（新增）
## 验收 Todo
- 表驱动覆盖未知错误码/枚举、不合法 trace/span 等标识、含路径和 URL 的 Step/Build/Version，以及消息替换；区分业务身份字段与技术消息，不能假设全部字符串均删除。
- slog 消息、嵌套属性、WithAttrs/WithGroup 中植入合成敏感标记，序列化输出不泄漏这些标记；合法允许字段仍可用。
- 容量 0/负值/1、越界追加、淘汰顺序与 Dropped 计数；Events 返回副本不允许调用方修改内部数据。
- 并发 Append/Events 在 -race 下通过，容量与总丢弃数正确；不依赖 goroutine 调度顺序。
- 用明确 TestLogRegression 前缀定向运行；记录现有覆盖和新增覆盖差异。

## 基线与领取
仓库 899ms/loretide；应用分支 app-main，起始核验基线 5716a4f15aa9ffc20b5a84137795d9938fed9c5a。先阅读 main 的 AGENTS.md、CONTRIBUTING.md 和应用分支 LORETIDE.md、CLAUDE.md。领取前重新检查 ready、负责人及认领评论，登记唯一工具/会话、分支、独立 worktree 路径，设置 assignee 和 in-progress 并移除 ready；冲突则停止领取。无须等待 #1/#2 的真实服务环境。

## 通用边界与交付
仅新增本任务列出的测试和专属文档，不编辑现有测试、生产代码、CI、依赖锁文件、数据库、服务或端口；不触碰 #1/#2/#11 的文件。发现生产缺陷时保留复现证据并报告阻塞，由主任务另行划定修复范围，不把错误行为写成正确断言。禁止 UI 单元测试和 computer use 验收；只运行定向 Go 测试，可用 -race 验证。不得启用真实执行器或调用外部模型，不读取凭据。
提交 Draft PR，base app-main，正文关联本 Issue，列出真实命令、结果、未验证项；完成后 review-needed，不自行合并或关闭。UI 影响：无直接页面改动；手动 UI Todo：本任务无需，不能声称 UI 已验收。更新专属文档和交付记录，避免多人修改共享 README。
