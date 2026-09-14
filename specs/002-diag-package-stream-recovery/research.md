# Research: 002 诊断包下载保真与实时流断线恢复

Phase 0。Technical Context 无 NEEDS CLARIFICATION；以下为技术决策。

## D1. 计划轮换的收尾标记形式

- **Decision**: 服务端在 `deadline` 触发时，再查询并写出一页，该页 JSON 增加 `"rotate": true`，然后正常返回。其他页不带该字段（或为 false）。
- **Rationale**: NDJSON 每行是完整 JSON，加一个布尔字段最省事；客户端已逐行解析 `pageSchema`；HTTP trailer 在浏览器 `fetch` 中不可读，否决。clarify 已定「客户端识别」。
- **Alternatives considered**: 关闭前发独立控制行 `{"control":"rotate"}`——需要新的行类型与 schema 分支；用 HTTP 状态或 header——流已开始，不可改。

## D2. 客户端状态机

- **Decision**: 抽出纯函数 `nextStreamState(state, event)`，事件：`open`、`page(rotate?)`、`end`、`error(status?)`、`pause`、`resume`。规则：`page.rotate` → 保持 `connected`，立即重连（延迟 0，不改 notice）；`end`/`error` 无 rotate → `disconnected`，退避；`error(403|404)` → `denied`，不再重连；成功页 → 退避归零并清 notice。
- **Rationale**: 纯函数可在 `// @vitest-environment node` 下测（constitution II）；hook 只负责副作用。
- **Alternatives considered**: 直接在 hook 内加分支——不可测，现状即如此。

## D3. 退避策略

- **Decision**: 2s 起，每次失败翻倍，上限 30s；任一成功页归零。标签页从后台恢复（`visibilitychange`）时若处于 `disconnected` 则立即重连一次。
- **Rationale**: FR-003；避免服务端不可用时每 2 秒打一次。

## D4. 过滤条件透传

- **Decision**: `useDiagnosticStream(wsId, enabled, filter)` 新增 `filter` 参数；`contract.ts` 增加 `streamQuery(filter, after)` 纯函数生成查询串（键名与 `diagnosticFilter` 服务端解析一致：kind / trace_id / run_id / component / error_code / severity / from / until / after）；`filter` 变化时游标归零并重连。
- **Rationale**: FR-002；服务端已支持这些键，只是客户端没传。

## D5. 下载保真

- **Decision**: `client.ts` 新增 `contentDiagnosticDownload(query, signal): Promise<Response>`，内部 `fetchRaw` POST；`queries.ts` 的 `download` mutation 改为 `const res = await ...; if (!res.ok) throw parsed error; return { blob: await res.blob(), filename: parseContentDisposition(res.headers.get("content-disposition")) ?? "loretide-diagnostics.json" }`。`views` 的 `download` prop 签名改为接收 `{blob, filename}`；web platform 直接 `URL.createObjectURL(blob)`。
- **Rationale**: FR-005；预览路径保留 zod 转换不受影响。
- **Alternatives considered**: 让服务端改为 snake_case 保持并让前端不转换——改动更大且破坏预览；用 `<a href>` 直链——需要 token 进 URL，违反隐私规则。

## D6. 缺口提示保留与 200 条标示

- **Decision**: `gap` 一旦为 true 保持，直到 `wsId` 变化或用户点「清除」；`mergeEvents` 的 200 常量导出为 `STREAM_EVENT_CAP`，页面在事件数达到上限时显示「仅显示最近 200 条」。
- **Rationale**: FR-007、FR-008。

## D7. 测试落点

- **Decision**: Go：`content_diagnostics_test.go` 新增用例——deadline 触发时最后一页 `rotate=true`；过滤键透传到 `Store.Query`；非授权 `account_id` 导出 403。TS：`contract.test.ts`——`rotate` 可选默认 false、畸形响应（缺 cursor）走 fallback、文件名解析、`streamQuery` 序列化；`queries.test.tsx` 或新 `stream-state.test.ts`（node 环境）——状态机全部转移。不写 UI 单测。
- **Rationale**: constitution II；`CLAUDE.md` Testing「每个行为一个规范层」。
