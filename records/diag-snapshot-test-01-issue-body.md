## 目标
现有快照测试仅修改 Hashes 一个字段并检查缺口数量。补齐快照隔离与撤权/文件变化的可解释结果。
## 文件边界
- server/internal/content/diagnostics/snapshot_regression_test.go（新增）
- docs/development/diagnostics-snapshot-regression.md（新增）
- records/diagnostics-snapshot-regression.md（新增）
## 验收 Todo
- CloneSnapshot 的 Required/Excluded/Grants/Hashes 修改双向隔离，标量与版本/范围/偏好字段保持，覆盖空集合。
- ReproductionGaps 覆盖无变化、缺失、hash变化、授权撤回以及同时发生；校验缺口内容而非仅数量，忽略 map 遍历顺序。
- 缺口计算不得修改保存快照、当前 hash 或授权输入；撤权不能被旧 Grants 自动恢复。
- JSON 往返保留必要快照字段；未知字段兼容的断言与当前契约一致。
- 用明确 TestSnapshotRegression 前缀定向运行。范围不包含数据库事务、HTTP 导出或权限集成验收（归 #2）。

## 基线与领取
仓库 899ms/loretide；应用分支 app-main，起始核验基线 5716a4f15aa9ffc20b5a84137795d9938fed9c5a。先阅读 main 的 AGENTS.md、CONTRIBUTING.md 和应用分支 LORETIDE.md、CLAUDE.md。领取前重新检查 ready、负责人及认领评论，登记唯一工具/会话、分支、独立 worktree 路径，设置 assignee 和 in-progress 并移除 ready；冲突则停止领取。无须等待 #1/#2 的真实服务环境。

## 通用边界与交付
仅新增本任务列出的测试和专属文档，不编辑现有测试、生产代码、CI、依赖锁文件、数据库、服务或端口；不触碰 #1/#2/#11 的文件。发现生产缺陷时保留复现证据并报告阻塞，由主任务另行划定修复范围，不把错误行为写成正确断言。禁止 UI 单元测试和 computer use 验收；只运行定向 Go 测试，可用 -race 验证。不得启用真实执行器或调用外部模型，不读取凭据。
提交 Draft PR，base app-main，正文关联本 Issue，列出真实命令、结果、未验证项；完成后 review-needed，不自行合并或关闭。UI 影响：无直接页面改动；手动 UI Todo：本任务无需，不能声称 UI 已验收。更新专属文档和交付记录，避免多人修改共享 README。
