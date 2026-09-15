# Phase 0 Research: 计划轮换页必须在窗口内送达

核实基线：`app-main` @ `1e8bccd`。每条给出实测或代码依据。

---

## D1 — 现有测试为什么是绿的：窗口比 ticker 还短

`content_diagnostics.go:41` 建 `time.NewTicker(time.Second)`，`TestContentDiagnosticStreamClosesPlannedWindowWithRotate` 把窗口设成 **50ms**。

50ms < 1s，所以**截止定时器在 ticker 一次都没跳之前就到期**：

- 第 1 轮：写首页（`rotate=false`）→ `select` 里**只有** `deadline.C` 就绪 → `rotate=true`
- 第 2 轮：写第 2 页（`rotate=true`）→ `return`

两页，末页带 rotate，绿。**这条路径生产环境永远走不到**，因为生产的窗口是 25 个 ticker 周期。

**结论**：这不是一条弱测试，是一条**测错了东西**的测试。它给出的「轮换页有回归覆盖」是假信号，而假信号比没有覆盖更贵——PR #21 据此宣布「计划轮换对用户不可见」已闭合。

---

## D2 — 真实窗口下处理器做了什么：实测

本机实跑，真实 ticker、真实 deadline、无代理，每个窗口跑三轮：

```text
window=2s   pages=4   rotate_pages=1   elapsed=2.003s   last_rotate=true
window=3s   pages=5   rotate_pages=1   elapsed=3.004s   last_rotate=true
window=5s   pages=7   rotate_pages=1   elapsed=5.002s   last_rotate=true
```

两件事：

1. **处理器确实会发出轮换页**，九次全部如此。所以「服务端不发 rotate」这个说法是错的。
2. **页数 = 窗口秒数 + 2**，`elapsed` 一律略大于窗口长度。

第 2 点是关键：`window=3s` 的 5 页是 `t=0,1,2,3` 四页**加上**一页轮换页，而那一页写在 `3.004s`——**窗口边界之后**。

---

## D3 — 外推到 25 秒窗口，与主任务实测严丝合缝

| 按 D2 的规律应该发生 | 主任务实测 |
|---|---|
| `t=0…25` 共 **26 页**（均无 rotate） | **26 行 NDJSON** ✓ |
| 随后第 **27 页**带 `rotate:true`，写在 `25.0s + δ` | **没有第 27 行** |
| δ = 一次 `getWorkspaceMember` + 一次 `Store.Query` 的耗时 | 流结束于 **25011ms**，即 δ ≈ **11ms** |

26 对上了，δ 对上了，只有第 27 行没了。**轮换页确实被写了（或正要被写），但它落在窗口边界之外，而连接在那里已经没了。**

那 9 条 503 是同一件事的另一面：连接在上游仍在跑数据库查询时被切断，代理把这个结束状态记成 503。

---

## D4 — 为什么不是「把超时调大」

主任务的嫌疑 (b) 是代理缓冲/超时。即使它成立，调大超时也**不是**正确的修法：

- 调大超时把赛跑推后，**赛跑本身还在**。今天的 δ 是 11ms，某天数据库慢一点变成 300ms，或者某个部署环境的代理超时正好卡在别处，缺陷就回来——而且症状完全相同，没人会想到是同一件事。
- 修法应当让**结构上不可能晚到**：轮换页落在窗口之内，它就不与任何边界赛跑，无论那个边界是 25s、30s 还是别的值。

**因此本特性不动代理**（FR-010）。代理的确切超时值仍未定位，这一点在 spec 里如实记着；但它不再影响正确性。

**顺带排除的**：Go server 的 `WriteTimeout`。`server/cmd/server/main.go:299` 明写两个超时**故意置零**（为 WebSocket 升级留长连接），`main_test.go:362` 有断言锁住。嫌疑 (a) 的这一半可以划掉。

---

## D5 — 修法：判定提到写之前

现在的形状（`content_diagnostics.go:42-47`）：

```text
for {
  member 复核（DB）
  page := Query(...)（DB）
  page.Rotate = rotate          ← rotate 由上一轮 select 决定
  write + flush
  if rotate { return }
  select { ctx.Done / deadline.C → rotate=true / ticker.C }
}
```

改成：

```text
end := now + window
for {
  member 复核（DB）
  page := Query(...)（DB）
  // 再等一个 ticker 周期就会越过窗口边界，那么这一页就是计划的最后一页。
  page.Rotate = !now().Add(tick).Before(end)
  write + flush
  if page.Rotate { return }
  select { ctx.Done / ticker.C }
}
```

- `deadline` 定时器**不再需要**——窗口边界变成一次比较，不是一个要竞争的 channel。顺带消掉了「deadline 与 ticker 同时就绪时 select 随机选」这个本来就没人想要的不确定性。
- 25s 窗口 / 1s ticker：`t=24` 那一页满足 `24+1 >= 25`，被标为轮换页并结束。**25 页，末页带 rotate，整条流在 `t≈24` 结束，离边界有 1 秒余量。**
- 窗口到期之后**零动作**（FR-003）：不再有那两次数据库查询。

**代价**：每个窗口少一页、窗口实际长度少一个 ticker 周期。这是 Q1 记下的取舍——用一个 ticker 周期的余量，换「结构上不可能晚到」。

**替代（已否决）**：把 deadline 设成 `window - 固定毫秒数`。要新引入一个常量，而那个常量的正确值取决于数据库有多快——正是 D4 说的那种会过期的数字。ticker 周期是现成的，而且它恰好就是「下一页什么时候来」的答案。

---

## D6 — 退化形态：窗口短于一个 ticker 周期

新判据下，若窗口 < 一个 ticker 周期，**首页**就满足「再等一跳会越界」，于是首页即轮换页，流立刻结束。

这是**正确的**：窗口里放不下第二页，那首页就是最后一页。但它会让现有的 50ms 测试的 `len(pages) >= 2` 断言失败。

**处理**：把那条测试的窗口改成 ticker 周期的整数倍（3s），让它走生产路径；另留一条用例显式覆盖退化形态，写明「一页是对的」。这样两种形态都被钉住，而不是让退化形态在断言里蒙混过关。

---

## D7 — 哪些结束路径**不**带 rotate

不能为了让流「安静地结束」而给所有结束都加 rotate。真断线必须让客户端看见（FR-005）：

| 结束原因 | rotate |
|---|---|
| 计划窗口到达 | **真** |
| 成员身份被撤销 | 假 —— `TestContentDiagnosticStreamEndsWithoutRotateWhenMembershipIsRevoked` 锁住 |
| 客户端断开（`r.Context().Done()`） | 假 |
| `Store.Query` 失败 | 假 |

这条既有测试不改，它正是防止「修好轮换顺手把断线也藏了」的那道闸。

---

## D8 — 端到端断言的落地方式

FR-007 要一条断言同时覆盖「末页 rotate 为真」与「客户端不进入断线态」。Go 与 TS 互相调不到，两边各写 mock 又正是 D1 那类错误的重演。

**做法**：一份提交进仓库的真实报文夹具。

1. Go 测试跑完一个真实窗口，取最后一页的**原始 JSON 字节**，与夹具逐字节比对；
2. TS 测试读**同一个夹具**，`JSON.parse` 后经 `pageSchema` 解析，喂给 `nextStreamState`，断言 `status !== "disconnected"` 且 `reconnectInMs === 0`。

服务端报文形状一变，Go 那侧先红，夹具必须更新，TS 那侧随之跟上。这是两侧唯一的共同真相来源。
