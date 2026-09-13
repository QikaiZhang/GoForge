# 07 — Process：`goforge dev` 与子进程管理

> 手写说明书。先读本文，完成 Handwriting Task，再看 internal/process 参考实现。
> 本阶段有三个"标准库默认行为是错的/不够用"的知识点，全部有测试钉死。

## 1. Background

脚手架的最后一环是"把项目跑起来"：`goforge dev` 应该像用户手敲
`go run ./cmd/server` 一样工作——日志实时可见、Ctrl+C 能停、退出码如实上报——
但又比手敲多一点：读配置决定端口、在非项目目录给出可理解的错误。

这是本项目第一次**管理另一个进程**，exec 包的每个默认行为都值得审一遍。

## 2. Problem

裸用 `exec.Command` + `cmd.Run()` 会在三个地方翻车：

1. **退出码当错误**：`cmd.Run()` 返回 `*ExitError` 当子进程退出码非零。
   如果 CLI 把它当"工具失败"，`goforge test`（下一阶段）永远报"内部错误"而不是"测试没过"。
2. **Ctrl+C 变成强杀**：`exec.CommandContext` 在 ctx 取消时默认发 **SIGKILL**。
   服务器没机会 graceful shutdown，正在处理的请求全部腰斩。
3. **信号到不了孙子进程**：`go run` 会编译出一个**孙进程**。把 SIGINT 发给 `go run`
   这一个进程，真正监听端口的 server 根本收不到——孤儿进程继续占着端口。

## 3. Requirements

```bash
cd user-service
goforge dev                # 起 go run ./cmd/server
goforge dev --port 9090    # flag 覆盖端口
GOFORGE_PORT=7070 goforge dev   # env 覆盖（flag > env > goforge.yaml > 默认）
```

行为契约：

- 子进程输出**流式透传**（不缓冲、不吞）；
- Ctrl+C / SIGTERM → 子进程收到 SIGINT → server 优雅关闭 → goforge exit 0；
- 子进程自行崩溃 → 退出码**透传**（不是 goforge 的 ExitError）；
- `go` 不存在 / 不在项目内 / 配置损坏 → goforge 自己的错误，exit 1 或 2。

## 4. Scope

不做：文件变更自动重载（air 的领域）、`-- build` 缓存管理、多服务编排、
Windows 进程组（本项目的信号语义是 POSIX 的，文档明确声明）、
stdin 的交互式转发测试。

## 5. Design

### process 包的三个契约

```go
func Run(ctx context.Context, name string, args []string, opts Options) (int, error)
```

| 情形 | 返回 |
|---|---|
| 子进程正常运行并退出 N（含非零） | `(N, nil)` —— 退出码是**结果** |
| 二进制不存在 / 起不来 | `(0, err)` —— 这才是**工具失败** |
| 被信号杀死 | `(128+n, nil)` —— POSIX 惯例 |

### 优雅停止的三层设计

```text
ctx 取消
   ↓ cmd.Cancel（覆盖默认的 SIGKILL）
syscall.Kill(-pid, SIGINT)      ← 负数 pid = 杀整个进程组！
   ↓ 子进程 ignores？等 KillDelay（默认 10s）
SIGKILL 兜底
```

关键：**Setpgid**。子进程放进自己的进程组后，`kill(-pid)` 才能把信号
同时送达 `go run` 和它编译出的 server 孙进程。没有 Setpgid，
信号只到 `go run` 一个进程——这就是 Problem #3 的解。

### dev 的退出码语义（本阶段最微妙的一处）

实测（docs 里留了实验过程）：组 SIGINT 之后，server 正确打出
`shutting down` 并优雅退出，但 `go run` 自己退出码是 1（或 130，取决于信号路径）。
结论：**当停止是 goforge 主动发起时，子进程的退出码不反映优雅与否**。

```text
ctx.Err() != nil（我们自己要求的停止）→ exit 0（请求停止 = 成功）
否则（子进程自己死了）              → 透传子进程退出码
```

### dev 与 config 的关系（对照 06 的预告）

`generate` 对坏配置只警告（它不需要配置也能干完活）；
`dev` 对坏配置直接失败（它的行为完全由配置决定）。**同一份配置，两种消费深度**。

## 6. Core Concepts

- **exec.Cmd 的字段全是指针语义的"覆盖点"**：Stdout 写成 io.Writer 就自动建管道并
  io.Copy 转发；写成 os.File 则直接继承 fd。Env=nil 继承父进程。
- **exec.CommandContext + cmd.Cancel（Go 1.20+）**：ctx 取消时先调 Cancel（我们换成组 SIGINT），
  等 WaitDelay 再 SIGKILL。这是标准库专门为"优雅停止"开的口子。
- **Setpgid / kill(-pid)**：Unix 进程组。子进程成为组长（pgid=pid），
  负数 pid 的 kill 广播整组。现实对应：终端 Ctrl+C 就是发给前台进程组的。
- **ExitError / ExitCode / WaitStatus**：`ExitCode()` 对信号死亡返回 -1；
  `Sys().(syscall.WaitStatus)` 才能拿到信号编号 → `128+n` 惯例。
- **signal.NotifyContext**：把 SIGINT/SIGTERM 翻译成 ctx 取消——"信号世界"到
  "context 世界"的桥。defer stop() 释放信号通道（不然信号处理泄漏）。
- **`go run` 的信号行为是经验品**：它会转发信号给编译出的二进制、被打断时自己
  退出码也不稳定（1 或 130）。所以测试断言的是"server 打出了 shutting down
  日志"（真证据）而不是"退出码是某个值"。

## 7. Design Decisions

1. **process 包独立于 dev 命令存在**：下一阶段的 `goforge test`、再下一阶段的
   `goforge git` 全是"起子进程"。第一次需要就抽包，不是过度设计——三个消费者就在前面。
2. **`(int, error)` 双通道返回**：语义上是"结果 vs 异常"。用 `(code, err)` 而不是
   `Result{Code, Err}` 是因为调用方的常见路径只需要 code；error nil/非nil 的
   区分恰好对应"要不要打印 goforge 自己的错误"。
3. **dev 的 ctx 从 signal.NotifyContext 来，但核心逻辑是 runDevContext(ctx,...)**：
   信号处理留在薄壳里，可测的内核接收 ctx 参数。集成测试因此能"模拟 Ctrl+C"
   而不用真的 kill 测试进程。
4. **KillDelay 默认 10s 且可注入**：grace 期限是策略不是机制，测试里 1-2s，
   生产 10s，都从 Options 进。
5. **dev 打印的端口来自合并后的配置**：让用户看见生效值（"starting server on :9090"），
   配置对了没、flag 起没起效，一眼可验。

## 8. Alternatives

- **直接 exec.Command + Run，不管信号**：服务器被 SIGKILL、请求被腰斩；生产事故模板。
- **发 SIGINT 给单个进程（不设进程组）**：`go run` 死了、孙进程成孤儿继续占端口——
  测试里真实出现过孤儿占住 18123 端口导致后续测试假通过。进程组是唯一正解。
- **用 channel + cmd.Process.Signal 手写 ctx 联动**：CommandContext + Cancel 已经封装好
  这套联动，手写只会引入更多竞态。
- **sh -c "go run ..." 包装一层**：多一个 shell 进程、信号多一跳、退出码更浑浊；零收益。
- **air/fresh 等热重载工具集成**：超 scope，且把学习对象（exec 语义）藏在了第三方后面。

## 9. Implementation Plan

1. `internal/process/process.go`：Options、Run（Setpgid → Cancel=组SIGINT → WaitDelay
   → Start → Wait → exitCode 翻译）。
2. process 单测六连：输出捕获、exit 3 ≠ err、信号死 128+n、二进制缺失 = err、
   ctx 取消即时停止、忽略 INT 时 SIGKILL 兜底（timing 测试标注 short skip）。
   **注意**：用 `sh -c "exec sleep 30"` 而不是 `sh -c "sleep 30"`——
   不加 exec 的话信号被 sh 吞掉，测试假失败（真实的坑，写进 Failure Cases）。
3. cli：`runDev`（装信号壳）+ `runDevContext`（flag 解析、config 合并、项目检查、
   process.Run、退出码语义）。
4. 集成测试：scaffold → generate → dev(--port 18123) → 轮询 /healthz → cancel →
   断言 exit 0 + 输出含 "shutting down" + 无孤儿进程（端口已释放）。
5. 手工验证完整链路，包括 Ctrl+C 的终端体验。

## Handwriting Task

不看参考实现，完成：

1. `process.Run`：满足"六连"测试。先写最朴素的版本（无 Setpgid、默认 Cancel），
   让信号测试红掉，再逐步修——**按发现问题的顺序体验设计动机**。
2. `runDevContext`：`--port` 两种写法（`--port 9` / `--port=9`）、config 四层合并、
   `go.mod`/`cmd/server` 预检查。
3. 集成测试：起服务 → healthz → cancel → 优雅关闭断言。
4. 让一个子进程 `trap '' INT`，验证 KillDelay 的 SIGKILL 兜底真的触发。

**Expected Behavior**：

- `goforge dev` 后另开终端 `curl localhost:8080/healthz` → `ok`；
  回终端按 Ctrl+C → 立即返回 shell，`echo $?` 是 0，
  server 日志里有 `shutting down`，`lsof -i :8080` 为空（无孤儿）。
- `goforge dev --port=9090` 的启动行显示 `starting server on :9090`。
- 在空目录 `goforge dev` → exit 1，stderr 提到 go.mod。
- 生成项目里把 main.go 改成 `log.Fatal("boom")` 启动即死 → dev exit 1（透传）。

## 10. Thinking Questions

1. 为什么退出码必须是 `Run` 的返回值而不是 error 的一部分？
   试着用 `Run(...) error` 设计一下，`goforge test` 该怎么区分"测试失败"和"go 没装"？
2. SIGINT、SIGTERM、SIGKILL 三者的可捕获性差异，决定了它们在 graceful 链条里的角色。
   谁能被忽略？谁不能？
3. `sh -c "sleep 30"` 和 `sh -c "exec sleep 30"` 在信号测试里行为完全不同，为什么？
   这对"包装一层 shell 再起进程"的工具意味着什么？
4. dev 把"自己发起的停止"映射为 exit 0。 反过来设计（永远透传 130）会怎么影响 CI 脚本？
5. 如果用户的项目 server 不处理 SIGINT，goforge dev 的行为链条是什么？
   用户会先观察到什么、后观察到什么？（10s 之后发生了什么？）

## 11. Testing

```bash
go test ./...
cd $(mktemp -d) && goforge new demo && cd demo
goforge dev &
sleep 2 && curl -s localhost:8080/healthz
kill -INT %1           # 模拟 Ctrl+C
wait %1; echo $?       # 0
lsof -i :8080          # 空：无孤儿进程
GOFORGE_PORT=7777 goforge dev &   # 观察启动行的端口
```

## 12. Failure Cases

- **孤儿进程**：不设进程组时，SIGKILL 只杀 go run，server 存活并占端口。
  本阶段的测试真实踩到：上一次失败的测试留下的孤儿让下一次 healthz 检查假通过。
- **把退出码当错误**：`server exited 1` 被打印成 `goforge dev: internal error`，
  用户以为工具坏了。
- **WaitDelay 忘了设**：CommandContext 的默认 WaitDelay 是 0 = 无限等——
  忽略 SIGINT 的子进程会让 goforge 永远挂着。
- **两个流写同一个 bytes.Buffer**（测试里）：bytes.Buffer 非并发安全，stdout/stderr
  两个 io.Copy goroutine 会竞争。测试需要带锁的 writer——这也是生产代码用
  "同一个 writer"时必须面对的问题。
- **测试里先 cancel 后起的进程**：ctx 已取消时 Start 直接失败，错误是
  "exec: not started" 一类的误导信息（实验中真实出现）。

## 13. Reference Implementation

- `internal/process/process.go`：包注释就是三个契约；Run 里 Setpgid/Cancel/WaitDelay
  三件套的顺序与注释。
- `internal/cli/cli.go`：`runDev`（信号壳）与 `runDevContext`（可测内核）的分离；
  `ctx.Err() != nil → ExitOK` 的语义注释。
- `internal/process/process_test.go`：exec 前缀的注释解释了为什么必须 `exec sleep`。

## 14. Git Diff

相对 06-config 新增/修改：

```text
internal/process/         新增：子进程运行器（三契约 + 六测试）
internal/cli/cli.go       dev 命令分发；runDev/runDevContext/parseDevPort；version 0.7.0
internal/cli/cli_test.go  flag 用法错误、项目外错误、坏配置致命、优雅关闭集成测试
```

## 15. Interview Questions

见 docs/interview.md 的 Process 部分。先自测：
exec.CommandContext 默认杀法是什么？为什么要进程组？128+n 是什么惯例？
context 取消和信号的关系？
