# 01 — CLI：一个进程从 argv 到 exit code

> 本文档是"手写说明书"。请先读完本文、完成文末 Handwriting Task，再去看 `internal/cli` 的参考实现。
> 刻意不贴参考源码，只给你推导代码需要的所有信息。

## 1. Background

任何 CLI 工具的第一性问题：**它就是一个普通的操作系统进程**。
操作系统只给它三样东西：argv（参数数组）、三个标准流（stdin/stdout/stderr）、
以及结束时归还的一个整数（exit code）。

GoForge 要成为 `goforge new user-service` 这样的工具，第一步不是"实现功能"，
而是把这个进程骨架搭对：接收 argv、分发命令、把正常输出和错误分流、返回正确的 exit code。

## 2. Problem

如果不先把 CLI 骨架想清楚，典型后果：

- 所有逻辑直接写在 `main()` 里 → 没法写测试（测试没法控制 os.Args，也没法捕获 os.Stdout）；
- 错误信息打到 stdout → 用户的 shell 管道 `goforge x 2>/dev/null` 失效；
- 忘记返回非零 exit code → CI 里 `goforge ... && next-step` 在失败时照样继续执行；
- help 文本和命令实现分离在两个地方 → 每加一个命令要改三处。

## 3. Requirements

本阶段只实现 CLI 骨架，功能都是"占位"的：

```bash
goforge                 # 打印 usage，exit 0
goforge help            # 同上
goforge -h / --help     # 同上
goforge version         # 打印 "goforge version 0.1.0"，exit 0
goforge --version       # 同上
goforge new <name>      # 校验"恰好一个参数"，打印计划，exit 0（真正创建项目在 02 阶段）
goforge new             # 缺参数 → stderr 报错，exit 2
goforge frobnicate      # 未知命令 → stderr 报错 + usage，exit 2
```

## 4. Scope

本阶段明确**不做**：

- 不真正创建任何文件/目录（那是 02）；
- 不引入任何第三方 CLI 框架（cobra 等）；
- 不做子命令嵌套（`goforge git status` 是 09 的事）；
- 不做 flag 组合解析（`-abc` 合并、`--name=value` 分割等），本阶段只有全局 `-h/--help/--version`；
- 不做彩色输出、交互式 prompt。

## 5. Design

先回答三个结构问题，代码自然就出来了。

**Q1：测试怎么写？** 如果 `Run` 直接读 `os.Args`、写 `os.Stdout`，测试就得改全局状态。
所以把"进程"和"逻辑"分开——这是整个项目最重要的一个签名：

```text
Run(args []string, stdout, stderr io.Writer) int
```

参数是要执行的 argv（**不含**程序名，因为 `os.Args[0]` 是 `goforge` 自己）、
两个注入的输出流、返回值是 exit code。

**Q2：main.go 里剩什么？** 只剩进程世界的胶水：

```text
os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
```

**Q3：命令怎么分发？** 本阶段命令一共三个，直接 `switch args[0]`。
分发顺序：无参 → help；help 类；version 类；`new`；其余 = 未知命令。

**Exit code 约定**（要在包里用常量命名，不许裸写数字）：

| code | 含义 | 本阶段触发场景 |
|---|---|---|
| 0 | 成功 | help / version / new（参数合法） |
| 1 | 命令运行了但失败 | 本阶段还没有（02 的文件系统错误开始出现） |
| 2 | 用法错误（命令行本身写错了） | 缺参数、未知命令 |

错误 → stderr；正常输出（包括 usage）→ stdout。usage 算"正常输出"，
因为它是对 `goforge help` 的正常应答；但"未知命令"场景下 usage 要跟着错误打到 stderr。

## 6. Core Concepts

必须掌握的 Go / OS 知识：

- **os.Args**：`[]string`，`os.Args[0]` 是程序本身的路径，所以传给逻辑层的是 `os.Args[1:]`。
- **io.Writer**：`os.Stdout`/`os.Stderr` 都实现了它。依赖接口而不是具体类型，才能在测试里换成 `bytes.Buffer`。
- **fmt.Fprintf(w, ...)**：向指定 writer 写格式化文本，`Fprintln`/`Fprint` 同理。
- **os.Exit(code)**：立即结束进程。注意它会**跳过所有 defer**，所以它只该出现在 main 的最后一行。
- **exit code 是 u8 之外的坑**：进程退出码实际是 0-255，`os.Exit(-1)` 会变成 255。
- **stdout vs stderr**：stdout 承载"产品输出"（可被管道接走），stderr 承载"诊断信息"（给人看的）。
- **`%q` 动词**：给字符串加引号并转义，报错信息里显示用户输入时用它，天然防歧义。

## 7. Design Decisions

1. **Run 返回 int 而不是 error** —— exit code 是 CLI 的最终契约，`error` 在 main 里最终也要翻译成 code。让翻译发生在 cli 包内部（谁分发命令谁定 code），main 保持一行。替代方案：`Run(...) error` + main 里 `if err != nil { os.Exit(1) }` —— 这样就无法区分 usage 错误（2）和运行错误（1）。
2. **裸 `goforge` 打印 help 且 exit 0**，而不是报错 exit 2。新手第一反应是敲命令名看有什么用，惩罚性 exit code 不友好。反例：`git` 裸敲是 exit 1。这是口味题，关键是**写进测试、固定下来**。
3. **不用 flag 标准库**：本阶段全局 flag 只有两个，手写 `switch` 更直白，且能精确控制"未知 flag"的报错格式。`flag` 包的价值（自动生成 usage、类型化解析）要到子命令有各自 flag 集时才体现——见 Alternatives。
4. **version 是 var 不是 const**：为了支持 `go build -ldflags "-X goforge/internal/cli.version=..."` 在构建时注入版本号，这是 Go 二进制分发的标准做法。
5. **usage 是 const 字符串**：本阶段只有一个入口需要它。它和命令列表的重复问题留给 10-refactor（在那里你会真实感受到"为什么需要 registry"）。

## 8. Alternatives

- **spf13/cobra**：Go 生态事实标准（k8s、docker 都用）。没选：(a) 本阶段学习目标是"CLI 本质是 argv→exit code 的进程"，框架会把这层包起来反而看不见；(b) 只有 3 个命令时 cobra 的 struct/flag 装配是纯开销。合理引入时机：命令 >10 个、需要 shell completion 时。
- **urfave/cli**：同上，且其 v2/v3 API 变动频繁。
- **flag.FlagSet 每命令一个**：标准库正路，命令多+flag 多时合理。本阶段 flag 极少，引入 FlagSet 反而让"分发"这一层被淹没。
- **直接在 main() 里 switch**：无法测试。哪怕 stage 01 也要坚持 Run 注入流的设计——测试成本会惩罚每一个偷懒的决定。

## 9. Implementation Plan

手写步骤（自上而下）：

1. `go mod init goforge`（go.mod 用 `go 1.22`）。
2. 写 `cmd/goforge/main.go`：一行调用 + os.Exit。先让编译通过。
3. 写 `internal/cli/cli.go`：定义三个 Exit 常量、version var、usage 常量。
4. 实现 `Run`：按"无参 / help 类 / version 类 / new / 默认"写 switch；每个分支想清楚：写 stdout 还是 stderr？返回哪个 code？
5. 实现 `runNew(args, stdout, stderr) int`：参数个数校验（恰好 1 个），合法则打印计划信息。
6. 写 `internal/cli/cli_test.go`：表驱动测试，每个用例断言 exit code + stdout 子串 + stderr 子串；再写一个"流分离"测试证明错误只进 stderr。
7. `go test ./...`，然后手工跑本节 Requirements 里的每条命令。

## Handwriting Task

不看参考实现，完成：

1. 创建 `cmd/goforge/main.go`，保证 `go run ./cmd/goforge` 不 panic 且 exit 0。
2. 定义 exit code 常量并实现 `Run` 的五路分发。
3. 实现 `new`：缺参/多参 → exit 2 + stderr；合法 → exit 0 + stdout。
4. 表驱动测试覆盖 Requirements 里列出的每一条命令行为。
5. 写一个测试证明：未知命令时 stdout 为空、错误全部在 stderr。

**Expected Behavior**（写完后你的程序应该满足）：

- `goforge version` 输出形如 `goforge version 0.1.0` 且带换行，exit 0。
- `goforge new`（无参）不产生任何 stdout 输出，stderr 包含用法提示，exit 2。
- `goforge new a b` 与无参行为一致（不是只取第一个参数！参数个数错就整体拒绝）。
- `echo $?` 在 `goforge frobnicate` 后是 2，在 `goforge version` 后是 0。
- `go test ./...` 全绿。

## 10. Thinking Questions

1. 为什么 `Run` 接收的 args 不含程序名？如果含，`new` 的参数下标会怎么变？
2. `os.Exit` 之后 defer 里的文件关闭逻辑还会执行吗？这会如何影响"写完文件再退出"的程序？
3. 为什么 usage 在"裸调用"时走 stdout，在"未知命令"时走 stderr？
4. 如果两个阶段后命令有 15 个，现在的 switch 会出现什么维护问题？你现在能想到哪种数据结构来替代？（10-refactor 会回收这个伏笔）
5. version 用 var + ldflags 注入，对一个从 `go install` 安装的二进制，用户怎么知道版本号是多少？

## 11. Testing

```bash
go test ./...                          # 单元测试
go run ./cmd/goforge version           # 手工冒烟
go run ./cmd/goforge new demo          # 应打印计划
go run ./cmd/goforge new               # 应 exit 2
echo $?                                # 验证 exit code
go run ./cmd/goforge 2>/dev/null       # usage 应该仍可见（在 stdout）
go run ./cmd/goforge xxx 2>/dev/null   # 应该看不到错误（在 stderr）
```

## 12. Failure Cases

- **忘了 `\n`**：shell 提示符和输出黏在一行，`$(goforge version)` 之类的替换也会带脏字符。
- **错误打到 stdout**：`goforge bad 2>/dev/null` 居然还能看到错误，管道语义被破坏。
- **未知命令返回 exit 1**：和使用错误（2）混在一起，脚本无法区分"命令写错了"和"命令失败了"。
- **main 里 log.Fatal**：等价于 exit 1，但跳过 defer 且绕过了 cli 包的 code 约定。

## 13. Reference Implementation

关键思路（细节请直接读 `internal/cli/cli.go`，很短）：

- 包注释第一句就回答"CLI 是什么"——让后来者 30 秒建立心智模型。
- `Run` 是唯一的分发点；每个命令一个 `runXxx(args, stdout, stderr) int` 函数，签名统一。
- 输出全部经过 `fmt.Fprint*` 显式选流，包内不出现 `os.Stdout` 字样（os 只属于 main.go）。

## 14. Git Diff

相对上一阶段（main，只有文档）新增：

```text
go.mod                    模块声明
cmd/goforge/main.go       进程入口：os.Args → Run → os.Exit
internal/cli/cli.go       分发逻辑 + Exit 常量 + usage
internal/cli/cli_test.go  表驱动测试 + 流分离测试
docs/01-cli.md            本文档
docs/adr/001-cli-design.md 架构决策记录
```

之后每个阶段的 "Git Diff" 小节都会这样帮你定位"这一步到底动了什么"。

## 15. Interview Questions

见 [docs/interview.md](interview.md) 的 CLI 部分（该文档在 10-refactor 阶段落地）。
先自测：argv 是什么？stdout/stderr 区别？exit code 为什么重要？command/subcommand/flag 的边界在哪？
