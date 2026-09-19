## 目标
现有 simulator_test.go 多数只比较耗时/时间及 Expected/Actual。增加独立行为断言，防止场景名与结果常量同时写错仍通过。
## 文件边界
- server/internal/content/diagnostics/simulator_regression_test.go（新增）
- docs/development/diagnostics-simulator-regression.md（新增）
- records/diagnostics-simulator-regression.md（新增）
## 验收 Todo
- Receiver 的重复、乱序、取消后结果拒收；正常递增序列可接收。
- timeout/cancel/reconnect/duplicate/late 的出错组件、结果类型、重试 attempt 与事件序列符合实际契约，断言不从被测 Scenarios 的 Expected 字段推导。
- 同 seed 的虚拟时间和耗时一致；随机 ID 和真实 Created 不纳入逐字相等；验证唯一运行 ID 与 operation/trace 关联。
- testEnabled=false、未授权账号、未知场景拒绝；正常场景完成。数据库场景需要真实持久层，明确排除并交还 #2，不伪造覆盖。
- 用明确 TestSimulatorRegression 前缀定向运行，无网络、服务或真实执行器调用。

## 基线与领取
仓库 899ms/loretide；应用分支 app-main，起始核验基线 5716a4f15aa9ffc20b5a84137795d9938fed9c5a。先阅读 main 的 AGENTS.md、CONTRIBUTING.md 和应用分支 LORETIDE.md、CLAUDE.md。领取前重新检查 ready、负责人及认领评论，登记唯一工具/会话、分支、独立 worktree 路径，设置 assignee 和 in-progress 并移除 ready；冲突则停止领取。无须等待 #1/#2 的真实服务环境。

## 通用边界与交付
仅新增本任务列出的测试和专属文档，不编辑现有测试、生产代码、CI、依赖锁文件、数据库、服务或端口；不触碰 #1/#2/#11 的文件。发现生产缺陷时保留复现证据并报告阻塞，由主任务另行划定修复范围，不把错误行为写成正确断言。禁止 UI 单元测试和 computer use 验收；只运行定向 Go 测试，可用 -race 验证。不得启用真实执行器或调用外部模型，不读取凭据。
提交 Draft PR，base app-main，正文关联本 Issue，列出真实命令、结果、未验证项；完成后 review-needed，不自行合并或关闭。UI 影响：无直接页面改动；手动 UI Todo：本任务无需，不能声称 UI 已验收。更新专属文档和交付记录，避免多人修改共享 README。
