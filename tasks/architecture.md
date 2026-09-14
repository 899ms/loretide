# Loretide · 模块化前置任务

状态：ARCH-01/02 已由 PR [899ms/loretide#19](https://github.com/899ms/loretide/pull/19) 交付并通过主任务审查（2026-09-14），待合并与验收；证据见该 PR 及 app 的 `docs/development/content-boundary-checks.md` → Acceptance mapping。关联 [模块边界约束](../docs/12-模块边界与变更回归约束.md) 与 [主任务清单](todo.md)。这里保存两项新增前置任务的唯一状态；主清单只引用，不重复勾选。

## ARCH-01 · 定义可执行的模块依赖清单
- [ ] **状态：DELIVERED，已合并，待用户验收**（PR [899ms/loretide#19](https://github.com/899ms/loretide/pull/19) 经 [#20](https://github.com/899ms/loretide/pull/20) 合入 app-main，2026-09-14）；依赖：LT-001、LT-002；规模：S；关联：NFR-03、任务计划共同完成标准。证据：`scripts/content-boundaries.json` 12 模块 = docs/12 §2 的 11 个 + diagnostics，逐名比对差异 0。
- **交付**：在实际 app 工程定位新增模块的公共出口、存储所有权、允许依赖和少量必要上游适配入口，形成机器可读依赖清单及说明。新增模块可以暂为空，不能通过全允许规则伪造边界。
- **验收/验证**：对照 docs/12 检查无循环设计；标出公共类型和授权消费者；不要求重构整个上游。
- **预计文件**：app 的模块依赖配置与开发说明，约1～2文件。实际路径在实施时确定。

## ARCH-02 · 将边界检查接入开发验证
- [ ] **状态：DELIVERED，已合并，待用户验收**（PR [899ms/loretide#19](https://github.com/899ms/loretide/pull/19) 经 [#20](https://github.com/899ms/loretide/pull/20) 合入 app-main，2026-09-14）；依赖：ARCH-01、LT-007；规模：M；关联：NFR-03、D10-V01、docs/12。证据：检查器 13 用例含私有路径 / 反向依赖 / electron / 循环负例；CI `loretide-content.yml`；本地入口 `pnpm check:content-boundaries` 与 `make check` 接入；PR 模板存储所有权项。未覆盖：SQL 层所有权、content 根目录以外代码（对照节已写明）。
- **交付**：实现 Go/前端新增模块的允许导入、私有入口及循环检查，接入验证命令与应用 CI；存储所有权审查规则纳入任务/PR检查表，不声称静态检查可证明所有运行时行为。
- **验收/验证**：合法示例通过；故意跨模块私有导入、反向依赖和核心引用 Electron/插件的独立测试夹具使检查失败；移除违规后恢复通过。不能仅依赖人工阅读目录。
- **预计文件**：边界检查脚本/配置、负例夹具、验证入口/CI、说明，目标3～5手写文件，超过时继续拆分。

LT-009 开始前须完成 ARCH-02。这里的依赖为 LT-001 → LT-002 → ARCH-01，与已有环境任务并行准备；LT-007 → ARCH-02 → LT-009，不形成循环。后续各功能仍需行为/契约/流程回归，边界检查不代替应用测试。
