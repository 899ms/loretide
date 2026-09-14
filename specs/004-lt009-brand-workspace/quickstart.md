# Quickstart: 004 验证指引

前置：LT-008、ARCH-02、DG-01 已完成（实施前置）；Windows 实例已启动；两个测试用户 U1（A、B 的 owner）、U2（非成员）。

## 自动检查

```bash
pnpm typecheck
pnpm --filter @multica/core test -- workspace/timezone
pnpm --filter @multica/views test -- locales/parity      # 四语言键一致
(cd server && go test ./internal/handler -run 'Workspace' -count=1)
pnpm check:content-boundaries
```

预期：全部通过；Go 用例含合法 / 非法 / 缺省 / 透传 / 非成员拒绝；TS 用例含缺失 / 非字符串 / 非法 → 默认。

## 手动验证（用户执行）

### 1. 时区保持（SC-001）

1. 以 U1 创建工作区 A，时区 `Asia/Shanghai`；创建 B，时区 `America/Los_Angeles`。
2. 刷新页面；退出重新登录。预期：A、B 各自时区不变。
3. `curl -H "Authorization: Bearer <U1>" http://127.0.0.1:18000/api/workspaces/<A>` → `settings.loretide.timezone = "Asia/Shanghai"`。
4. 设置页把 A 改为 `Europe/London`，保存，刷新。预期：显示 London；`curl` 同上返回新值。
5. 用 `curl -X PATCH ... -d '{"settings":{"loretide.timezone":"Mars/Olympus"}}'` → 400，再 GET 仍为 London。

### 2. 切换隔离（SC-002）

1. 在 A 创建一条任务。切换到 B。
2. 网络面板抽样 5 个空间范围请求（任务列表、成员、诊断概览、收件箱、标签），核对响应 `workspace_id` 全为 B；A 的任务不出现。
3. 打开诊断实时流，事件 `workspace_id` 全为 B；切回 A，A 的任务恢复显示。

### 3. 非成员拒绝（SC-003）

```bash
curl -i -H "Authorization: Bearer <U2>" http://127.0.0.1:18000/api/workspaces/<A>
curl -i -X PATCH -H "Authorization: Bearer <U2>" -H "Content-Type: application/json" -d '{"settings":{"loretide.timezone":"UTC"}}' http://127.0.0.1:18000/api/workspaces/<A>
```

预期：403 或 404；响应 body 不含 `name` / `timezone` / `settings`。

### 4. 术语（SC-005）

中文 locale 下打开创建工作区、工作区切换器、设置 → 工作区三处，术语均为「工作区」，无「品牌空间」。

## 手动 UI Todo（用户确认）

1. 创建工作区表单出现时区选择，默认预选 `Asia/Shanghai`；提交后详情显示该时区。
2. 设置页显示当前时区并标注「默认」（未显式设置时）；修改保存后刷新为新值。
3. 从 A 切到 B，任务列表与诊断页只显示 B 的内容；切回 A 恢复。
4. 中文 locale 三处术语一致为「工作区」。
