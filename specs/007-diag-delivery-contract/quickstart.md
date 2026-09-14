# Quickstart: 007 验证指引

前置：Node 22、`pnpm install` 完成。**本功能无需数据库、无需启动应用、无 Go 改动。**

## 自动检查（可执行部分）

```bash
# 检查脚本自身的测试（含三条缺项负例）
node --test scripts/check-diagnostics-contract.test.mjs

# 对当前仓库扫描：应通过，且对 11 个尚无目录的模块不产生任何输出
node scripts/check-diagnostics-contract.mjs

# 合并入口
pnpm check:diagnostics-contract

# 既有边界检查必须不受影响
pnpm check:content-boundaries
```

预期：全部退出码 0。扫描摘要应显示**检查了 1 个已落地模块（`diagnostics`）、跳过 11 个未落地**。

## 手动核对（开发者执行，非 UI 验收）

1. **缺项粒度（SC-002）**：在测试里三个夹具分别只缺 E1 / E2 / E3，确认输出点名的是**对应那一条**，不是笼统的「无证据」。
2. **沉默（SC-003）**：确认扫描输出里不出现 `source-inbox` 等 11 个未落地模块的任何字样。
3. **CI 触发条件未变（SC-005）**：
   ```bash
   git diff origin/app-main -- .github/workflows/loretide-content.yml | grep -E "^[+-]" | grep -v "^[+-][+-]"
   ```
   diff 里**只应出现新增的 run 步骤**，`on:` 段无任何增删行。
4. **无迁移（SC-007）**：
   ```bash
   ls server/migrations | wc -l    # 与 origin/app-main 一致
   ```
5. **合同可判定性（SC-001）**：拿合同表格第三列逐条对照 `diagnostics` 模块，确认每条都能指到真实代码；错误码枚举那一行应明写「暂无公共入口」。
6. **模拟 ≠ 真实（SC-006）**：确认合同的证据表第二栏在执行器禁用期间为「未执行」，且 mapping 里 D13-V12 未被标为整体通过。

## 变异验证（SC-004）

删掉检查脚本里 E1 / E2 / E3 任一条规则，对应负例必须变红：

```bash
# 例：临时移除 E2 判定后
node --test scripts/check-diagnostics-contract.test.mjs   # 应有用例失败
```

「写完看绿」不算验证——SC-004 要的是删掉规则会红。

## 未执行项的记录方式

任何未执行的项在交付记录中标「按策略未执行，等待用户验证」，不标通过。**本功能无手动 UI 项**——没有页面改动。
