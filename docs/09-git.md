# 09 — Git：CLI 组合而非库链接

> 手写说明书。先读本文，完成 Handwriting Task，再看 internal/git 参考实现。

## 1. Background

脚手架管的是"项目的第一次提交之前"。项目一旦存在，版本控制就是日常——
`goforge git status` 是 goforge 触达版本控制的最小切口。

更重要的是本阶段的教学主题：**一个 CLI 工具如何通过调用另一个 CLI 组合能力**。
Unix 哲学的"do one thing well"落到工程实践，就是 `plumbing`（管道、子进程、退出码协议）——
07 阶段的 process 包就是为本阶段准备的最后一节管道。

## 2. Problem

如果不用组合，替代方案各有代价：

- **链接 go-git 库**：二进制变大、API 追着 git 内部实现跑、用户看到的输出格式
  和原生 git 不一致（"为什么 goforge 的 status 和我的 git 长得不一样？"）；
- **什么都不做**：用户离开 goforge 的语境去敲 git，工具丧失工作流的完整性；
- **完整透传 `goforge git <anything>`**：那不是集成，是给 git 起了个别名——
  没有增加任何价值，还多了一层间接。

## 3. Requirements

```bash
cd user-service
goforge git status     # 等价于 git status，输出原样透传
```

- 目录不是 git 仓库 → git 自己的报错照常显示（exit 128），
  goforge 追加一句可执行的提示（"run 'git init' first"），整体 exit 1；
- git 未安装 → "command not found" 的 goforge 式错误；
- `goforge git log` / 裸 `goforge git` → exit 2 + 明确说明只支持 status；
- 其他退出码一律透传（不解释 git 的世界）。

## 4. Scope

不做：status 以外的任何子命令、输出解析/美化（porcelain 模式）、
`new --git` 自动 init、提交生成代码的自动化（"generate 后自动 commit"是真实需求，
留给练习）、go-git 等库的任何使用。

## 5. Design

```text
cli.runGit（信号壳）
   └─ runGitContext(ctx, args)
        ├─ 子命令白名单：只有 "status"，其余 → ExitUsage + 指路 git 本体
        └─ git.Status(ctx, ".", stdout, stderr)
             └─ process.Run(ctx, "git", ["status"], {Dir, 流透传})
```

三层职责：

| 层 | 知道什么 | 不知道什么 |
|---|---|---|
| git 包 | git 的调用方式、退出码含义（128=非仓库） | CLI 的 exit code 约定、用户提示措辞 |
| cli 层 | 子命令路由、提示语、exit code 翻译 | git 怎么被起进程 |
| process 包 | 子进程通用机制 | git/go 的任何语义 |

注意 git.Status 和 runTest 的对称性：**同一个 process.Run、不同的参数**。
这就是 07 说的"第一次需要就抽包"的回报时刻。

### 退出码协议

组合 CLI 的核心是退出码即接口。git 的约定：0 成功、128 致命错误（非仓库是典型）。
goforge 只对 128 做语义翻译（因为只有它知道"该 init"），其余全部放行——
**翻译你知道的，放行你不知道的**。

## 6. Core Concepts

- **组合 vs 集成**：组合（composition）= 调用既有工具；集成（linking）= 把能力编译进来。
  组合的产物边界清晰（进程隔离、版本隔离——用户升级 git 不用等 goforge 发版）。
- **porcelain 输出**：`git status --porcelain` 是 git 给程序的稳定接口；
  人读输出是给人的。本阶段透传人读输出，**如果**要解析，必须切 porcelain——
  这是所有 CLI 组合的铁律（解析人读输出 = 绑定未声明的格式契约）。
- **Dir 选项**：process.Options.Dir 让"在哪个目录跑 git"成为参数而不是全局 cwd 假设——
  这是 process 包注入设计的又一次回报。
- **退出码作为跨进程 API**：exit code 是子进程唯一的"返回值"。
  128 这个约定从 git 流向 goforge，goforge 再映射成自己的 ExitError。

## 7. Design Decisions

1. **git 包独立而不是塞进 cli**：和 project/generator 一样的理由——语义归语义，
   路由归路由。哪怕现在只有一个函数，"git 的知识"值得一个明确的归所。
2. **白名单而不是透传**：`goforge git log` 指路原生 git 而不是默默转发。
   透传会让 goforge 成为"没有文档的 git 代理"（每个子命令都要考虑是否特殊处理），
   白名单把边界钉死在代码里，也钉死在报错信息里。
3. **只追加提示，不复述错误**：非仓库时 git 已经把 `fatal: ...` 打到 stderr，
   goforge 不重复、只补充"下一步做什么"。工具之间的输出要像对话，不要像抢话。
4. **不要求在 goforge 项目内**：`git status` 本身不依赖 go.mod——命令的先决条件
   跟着语义走，不跟着"工具的主题"走。

## 8. Alternatives

- **go-git（纯 Go 实现的 git）**：可编程性强（读树、改 index）、零外部依赖。
  没选：本阶段只需要 status 的透传，库的二进制体积/维护面完全用不上；
  且输出统一性丧失。如果将来做 `goforge commit`（需要构造提交对象），重审。
- **os/exec 直调，不经过 process 包**：复制粘贴 07 的信号/退出码逻辑，
  或更糟——不带 ctx 直接 Run（Ctrl+C 无法中断）。上一阶段的抽象就是为此准备的。
- **git status --porcelain + 自定义渲染**：为"好看"引入格式契约和解析代码；
  透传人读输出是最诚实的第一版。
- **viper/cobra 之 exec helper**：无必要，标准库 + 自家 process 包已足够。

## 9. Implementation Plan

1. `internal/git/git.go`：Status——注意包注释把"为什么组合"写成文档。
2. git 包测试三连：仓库内（git init 临时目录）0 + "On branch"；
   仓库外 128 + git 的诊断在 stderr；PATH 置空 → goforge 的 not-found 错误。
3. cli：`runGit`/`runGitContext`（第 N 次壳核分离，应该是肌肉记忆了）、白名单、
   128 → 提示 + ExitError。
4. cli 测试：裸 git / git log / 仓库外 / 仓库内。
5. 手工验证三条路径的退出码。

## Handwriting Task

不看参考实现，完成：

1. `git.Status(ctx, dir, stdout, stderr) (int, error)`，走 process 包。
2. 白名单路由：非 status → exit 2 + "use the git binary directly"。
3. 128 → 追加 `git init` 提示 + exit 1；其他码透传。
4. 测试覆盖"git 未安装"（t.Setenv("PATH", "")）——注意这测的是 goforge 的错误通道，
   不是 git 的退出码。

**Expected Behavior**：

- 仓库内：输出与 `git status` 完全一致（diff 两个输出为空），exit 0。
- 仓库外：能看到 git 的 `fatal: not a git repository` **和** goforge 的
  `run 'git init' first`，exit 1。
- `goforge git log`：exit 2，提示里出现 "status" 和 "git binary" 两个词。
- `PATH= goforge git status`：exit 1，错误含 "not found"。

## 10. Thinking Questions

1. 为什么"解析 `git status` 的人读输出"是坏主意，而"调用 git CLI"是好主意？
   两者都依赖 git 的行为，边界在哪？
2. 128 是 git 的约定而非 POSIX 标准。goforge 硬编码 128 做翻译，
   这个耦合可接受吗？有没有更稳的探测方式（`git rev-parse --is-inside-work-tree`）？
   各自的代价？
3. 如果要支持 `goforge git <任意子命令>` 的完整透传，正确实现是什么？
   实现之后，goforge git 相对于裸 git 还剩什么价值？
4. process.Run 对 git 的输出做了流式透传。如果把 stdout/stderr 合并成一个 writer，
   会有什么顺序问题？（提示：两个管道、一个 buffer、无锁。）
5. "generate 后自动 git commit 生成的文件"——画出这个特性的决策树
   （仓库存在？有 user.email？有暂存冲突？），你会实现到哪一层就停手？

## 11. Testing

```bash
go test ./...
cd $(mktemp -d) && goforge git status; echo $?    # 1 + init 提示
git init -q
goforge git status; echo $?                        # 0
diff <(goforge git status) <(git status) && echo IDENTICAL
goforge git push; echo $?                          # 2
```

## 12. Failure Cases

- **解析人读输出**：git 版本一升级（措辞变化）解析就断；porcelain 才是接口。
- **吞掉 git 自己的 stderr 再重新表述**：信息丢失 + 抢话。追加提示，别替换输出。
- **把 128 当成唯一的失败**：git 还有其他非零码（比如 hook 失败），
  白名单翻译 + 其余透传才不误伤。
- **忘了 git 可能不存在**：exec.ErrNotFound 的友好化是 process 包的职责，
  但 CLI 层的测试要钉住这条路径（PATH 注入）。
- **在 cli 包里直接 exec**：绕过 process 包 = 重新引入无 ctx、无组、无注入的裸奔。

## 13. Reference Implementation

- `internal/git/git.go`：25 行。注意包注释与 ADR 的分工——包注释讲"是什么/为什么"，
  ADR 讲"决策与替代方案"。
- `internal/cli/cli.go` 的 `runGitContext`：白名单 + 128 翻译的注释
  （"翻译你知道的，放行你不知道的"）。

## 14. Git Diff

相对 08-test 新增/修改：

```text
internal/git/             新增：Status + 三条路径测试
internal/cli/cli.go       git 命令分发；runGit/runGitContext；usage 增行；version 0.9.0
internal/cli/cli_test.go  参数检查、仓库外、仓库内 三组测试
```

 again 无新依赖。对比 07 的 diff：又一次"复利"——新能力 = 薄薄一层路由 + 一个函数。

## 15. Interview Questions

见 docs/interview.md 的 Architecture 部分（组合 vs 集成）。先自测：
组合和库链接的取舍？退出码作为跨进程 API 是什么意思？
什么情况下必须从组合切换到库？
