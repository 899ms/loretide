# Claude 停止 · 交接（2026-09-21）

用户于 2026-09-21 指示「停止」，随后决定在 Orca 里新起一个主控会话继续开发。本文件是交接入口。

## 停止时的状态（GitHub 为准）
- app-main `6b4fb2c0c`；docs main 见本文件所在提交；迁移到 531；落地模块 8/12（diagnostics、workspace-core、ip-profile、topic-planning、work-editor、review-delivery、feedback-learning、source-inbox）。
- 两个 Claude 云会话（001 `session_016ncTaX7LDCMY1PQ315Pams`、002 `session_01VRKTVAr7o8WwQTs3Fzgi1X`）已收到暂停指令（提交 WIP 并推送后停下），不再由它们开发，除非新主控重新启用。
- 开放 PR：
  - **#216**（Issue #208）`claude/fix-029-workspace-tab-test` @ `788ed0d48`：把 #199 硬接进上游 `workspace-tab.tsx` 的运营规则分节改为插槽注入，上游测试文件零改动，8 条红转绿。前任主控本机复核已通过（boundaries、views settings+workspace-core 457、apps/web 274），**尚未发放行评论、未合并**。
  - **#217**（Issue #214）`claude/impl-031-historical-import-api` @ `4bc3a5802`：031 历史导入的存储与接口。**尚未审查**。
  - #6、#3：Codex 早期的 Windows/诊断 Draft，搁置。
- 开放 Issue：#214（对应 #217）、#211（030 实施，未开始）、#208（对应 #216）；搁置：#115、#151、#153（上游）、#1、#2。
- 待派：031 PR 2（页面：导入向导 + 「暂无个人表现数据」）、030 实施（#211）。

## 本轮里程碑（2026-09-20～21）
SOP 不依赖执行器的环节全部落地：§2 今日工作台（/today，六+第七区块，侧栏「内容」分组）、§3.1 账号表达配置、§3.2 运营规则（除团队复核）、§4 粘贴/URL 素材收件箱、§5.2/5.3 选题卡与冻结简报（含账号关联）、EP-04b 开始与快照、§7 作品/文档/版本、§8–9.3 审核/交付/发布记录、§10.1 指标与反馈。规格已合入待实施：030、031。逐条记录见 `records/2026-09-20-开发转回Claude.md`。

## 用户侧待办（主控不代做）
- 本机开发库迁移 481→531（备份 → `go run ./cmd/migrate up` → 重复执行验证 → 更新 `docs/development/native-windows.md`）。
- 手验累计 232+ 条：各 `app/specs/*/manual-ui-todo.md`。

## 必须保留的限制
1. UI 由用户人工检测；禁止 UI 单元测试与 computer use 验收；不主动处理 UI 缺陷或样式，只提供受影响 UI 与手验 Todo；UI 整套继承上游 Multica 设计系统与既有组件，不新增控件。
2. 真实 AI 执行器保持禁用（宪法 IX）。
3. 用户本机 DB（`127.0.0.1:15332/loretide_dev`）禁止任何连接/测试/迁移/服务操作；绝不连 `localhost:5432/multica`。测试只用隔离库（`app/docs/development/testing-database-suites.md`，`LORETIDE_DB_TEST_*` opt-in）。
4. Windows/W-03/文件输入、上游缺陷继续搁置；「来自上游的先不处理」。
5. 上游改动只在 SOP 需要时最小改动：单列 `upstream:` 提交、既有行零删除、PR 正文「上游改动」节写理由/替代/回滚（`spec-kit-workflow.md` 第 13 步）；风格/现代化永远不是理由；`scripts/content-boundaries.json` 的 `modules` 依赖表不改。
6. 第 12 步：带路径参数端点要有穿过真实中间件的用例，参数只经 `chi.URLParam`；路由必须真实挂载在 `router.go`。
7. 迁移 R1–R6：无外键/级联、索引单文件 CONCURRENTLY、建表无内联 PK/UNIQUE、每个并发索引登记 `concurrentIndexCleanups`、每表 `workspace_id` 打头索引、新表进删除清单与删除链。写路径持 `LockWorkspaceForContentDiagnosticWrite` 栅栏。
8. CONTRIBUTING：Issue 预占、独立分支、Draft PR、主控审查后 squash 合并；规格 PR 合并后必须主动通知执行者再起实施分支。
9. 审查铁律（前任主控踩过的坑）：`pnpm typecheck --force`（缓存命中不算证据）；handler 测试绿不等于路由已挂；`check:content-boundaries` / `check:diagnostics-contract` / `check:diagnostics-no-upload`；改 `packages/*/package.json` exports 必跑 `rich-content/package-exports.test.ts`；改 `packages/views/<dir>` 必跑该目录 vitest；改 `paths.ts` 会连锁触发图标/文案/命令面板/诊断掩码登记；`cmd/migrate` 与 `internal/migrations` 策略测试；空值≠0；只插不改守卫要同时断言有 INSERT。

## 提示词
新主控的接手提示词见同目录 `2026-09-21-Codex接手提示词.md`（对 Claude/Codex/Orca 会话通用）。
