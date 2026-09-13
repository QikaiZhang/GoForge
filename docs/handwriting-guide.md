# Handwriting Guide — 如何用这个仓库练手

> 本仓库的唯一用途：让你在几周内，**不看 AI 写的实现**，亲手把 goforge 复刻一遍。
> 本文是操作手册。先读它，再 checkout 第一个 branch。

## 声明

本仓库的 Git 历史是为学习设计的 **staged history**（模拟的工程演进顺序），
不是真实公司项目历史；不包含真实用户、业务数据、性能数据或生产事故。
docs 里所有"这个 bug 真实踩过"指的是"设计这个学习材料时真实踩过"。

## 核心循环（每个 branch 一轮）

```text
① checkout        git checkout 01-cli
② 读文档          通读 docs/01-cli.md（15 个小节 + Handwriting Task）
                   ——刻意不看 internal/ 和 cmd/ 的实现
③ 隐藏参考实现     把 AI 代码移出视线（方法见下节）
④ 手写            只凭文档 + 测试，写出该阶段能力
⑤ 跑测试          go test ./...（测试是规格，不是答案）
⑥ 对比            git diff 看参考实现 vs 你的实现
⑦ 记录            在笔记里回答三个问题（见"记录模板"）
⑧ 下一个          git checkout 02-project-init
```

一轮预计 2~6 小时。全程约 40~60 小时。

## 第③步的具体做法（三选一）

**方法 A：原地删除（最简单，推荐）**

```bash
git checkout 01-cli
# 只删实现，留下测试、文档、templates/、go.mod
rm -rf cmd/goforge internal/cli
go test ./...   # 确认测试变红（红了才说明测试有效）
# ……开始手写，直到 go test ./... 变绿
```

每个阶段"删什么、留什么"不同（下表）。原则：**测试永远留**——它们是规格；
`templates/` 从 03 阶段起算作"内容资产"可以留（模板语法是文档 03 的 Core Concepts），
想练更狠的也可以连它一起删，按 docs/03 重建。

**方法 B：双目录对照**

clone 两份，A 目录读代码，B 目录手写。适合喜欢"随时能偷看一步"的人——
但每偷看一步，就在笔记里记一笔"偷看了什么"。

**方法 C：worktree**

```bash
git worktree add ../goforge-ref 10-refactor   # 只读参考
# 在本仓库工作目录里手写
```

适合方法 A 但想随时 diff 参考实现的人。

## 各阶段"删什么、留什么"

| Branch | 删除（你要写的） | 保留（规格与资产） | 预估 |
|---|---|---|---|
| 01-cli | `internal/cli/cli.go`（留 `cli_test.go`？不，测试也要自己读懂后留用） | docs、go.mod | ★ |
| 02-project-init | `internal/project/*`（project.go/files.go→需按 docs 重建文件表） | cli_test.go、docs | ★★ |
| 03-template | `internal/template/`、`internal/project/project.go` 的渲染部分 | templates/、docs、ADR | ★★ |
| 04-code-generation | `internal/generator/*` | templates/、docs | ★★★ |
| 05-ast | `internal/astedit/*`、`internal/generator/wiring.go` | templates/、docs、ADR-003 | ★★★★ |
| 06-config | `internal/config/*` | docs | ★★ |
| 07-dev | `internal/process/*`、cli 里的 runDev | docs、ADR-004 | ★★★★ |
| 08-test | cli 里的 runTest | docs | ★ |
| 09-git | `internal/git/`、cli 里的 runGit | docs、ADR-005 | ★ |
| 10-refactor | `internal/cli` 的 Run/usage（保留全部 runXxx） | docs、不变量测试 | ★★ |

注：测试文件（*_test.go）**保留不删**。它们是每阶段的机器规格。
手写时先读测试，测试读不懂的地方回文档找解释。

## 第⑥步：diff 的正确姿势

```bash
# 你的实现 vs 参考实现（方法 A 原地手写时）
git diff 01-cli -- internal/cli/

# 阶段间的演进（理解"为什么变成这样"）
git diff 02-project-init 03-template        # 拼接 → 模板
git diff 03-template..03-template           # 单个 branch 内部的演进
git log --oneline 02-project-init..03-template
```

看 diff 时按顺序问三个问题：
1. **结构差异**：参考实现多了/少了哪些函数、哪些层？为什么它需要我没需要的？
2. **命名差异**：函数/变量/错误的名字，哪个更准确？
3. **测试差异**：参考实现的测试覆盖了我没想到的失败模式吗？

## 第⑦步：记录模板

每阶段结束，在你的笔记里写四行：

```text
## 0X-<阶段名>
我的设计：<一句话概括你的结构选择>
AI 的设计：<一句话概括参考实现的结构选择>
关键差异：<哪一处最值得记住，为什么>
没写出来的原因：<文档里哪一段其实已经给了答案，我当时没读懂/没读完>
```

几周后回看这份笔记，比重读任何教程都有用。

## 推荐学习顺序

**严格按 branch 顺序**（01→10）。这个顺序是精心排的：

- 01 的 `Run` 签名是后面一切可测试性的地基；
- 02 故意用字符串拼接，03 才有"为什么需要模板"的痛感；
- 04 故意留下"没接线"的缺口，05 的 AST 才有出场理由；
- 07 的 process 包先被 dev 使用，08/09 的 test/git 才能薄到不可思议；
- 10 的重构债是 01~09 一起积累的，最后一起还。

跳阶段 = 跳过动机 = 只背结论。

**可选的高阶玩法**（第二轮复刻时）：

1. 在 02 就直接用模板，然后对比：你的 02 和参考的 02+03 差多少？
   提前抽象省掉的痛感值多少学习量？
2. 在 05 用 x/tools/astutil 替代手写 import 插入，对比边界处理。
3. 给 goforge 加一个参考实现没有的命令（比如 `goforge remove user`），
   完整走一遍 docs/04 的 Design 流程。

## 常见问题

**Q：卡住了，能看参考实现吗？**
先回文档重读该阶段的 Design 和 Implementation Plan（文档写足了推导材料）；
再看测试的断言（机器规格）；还不行就看**签名**不看函数体（`go doc` 或 IDE 只展开到签名）。
最后才允许看实现——看完立刻关掉，凭记忆写。

**Q：测试失败但我确定自己是对的？**
测试是这个仓库的法律。99% 是你对 Expected Behavior 的理解有偏差；
剩下 1% 确实是测试的问题（比如 -run 测试需要 -v 才断言可见），以 docs/08 的说明为准。

**Q：阶段间的集成测试很慢？**
`go test -short` 跳过所有真实编译/子进程集成测试。但提交前至少完整跑一次。

**Q：我想改架构（比如不用 embed）？**
可以——这正是练习。改之前在笔记里写下 ADR（照抄 docs/adr/ 的四段格式：
Context/Decision/Alternatives/Trade-offs）。写得出 ADR 的改动才值得做。

## 学完之后

- 把 docs/interview.md 当口试题库自测，答不上来的回对应阶段文档；
- docs/architecture.md 的"第 6 节：死亡点"列了八个进阶方向，每个都值得一个新项目；
- 最有价值的输出物不是这个复刻的 goforge，而是你笔记里的那份"差异账本"。
