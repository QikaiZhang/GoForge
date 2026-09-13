# ADR 004 — 子进程管理：进程组、优雅信号链、退出码双通道

状态：已采纳（07-dev 阶段）

## Context

`goforge dev` 需要以子进程方式运行 `go run ./cmd/server`，要求：
输出流式透传、Ctrl+C 优雅停止（server 要能 graceful shutdown）、
退出码如实上报。exec 标准库的默认行为（SIGKILL、单进程信号、ExitError 一刀切）
在三点上不满足需求。

## Decision

1. 新建 internal/process 包，签名
   `Run(ctx, name, args, Options) (exitCode int, error)`：
   - 子进程跑完并退出 N → `(N, nil)`（非零码不是工具错误）；
   - 起不来 → `(0, err)`；
   - 信号死亡 → `(128+n, nil)`（POSIX 惯例）。
2. 子进程以 `Setpgid` 放入独立进程组；ctx 取消时 `kill(-pid, SIGINT)` 广播整组，
   使 `go run` 的编译孙进程也收到信号；`cmd.WaitDelay`（默认 10s）后由标准库
   升级为 SIGKILL。
3. `goforge dev` 在 ctx.Err() != nil（自己发起的停止）时返回 exit 0；
   其余情况透传子进程退出码。graceful 的判定依据是 server 的
   "shutting down" 日志（集成测试断言），不是 go run 的退出码。
4. 配置消费深度分级：generate 对坏配置警告，dev 对坏配置致命。

## Alternatives

- **exec.CommandContext 默认行为（SIGKILL 单进程）**：服务器被强杀、请求腰斩；拒绝。
- **单进程 SIGINT（无进程组）**：孙进程成孤儿占端口；测试环境真实复现
  （孤儿 server 让后续测试的 healthz 假通过）；拒绝。
- **自己起 goroutine 定时杀 / 手写 ctx 联动**：cmd.Cancel + WaitDelay 已是标准解，
  手写徒增竞态；拒绝。
- **热重载工具（air 等）集成**：超出学习目标并掩盖 exec 语义；拒绝。

## Trade-offs

- 换来：graceful 链条可验证、无孤儿进程、退出码语义对脚本友好、
  process 包被 test/git 阶段直接复用。
- 付出：POSIX-only（SysProcAttr/Setpgid 不跨 Windows）；`go run` 被打断时的
  退出码不稳定（1/130），迫使 dev 引入"自己发起的停止 → 0"这一层语义；
  WaitDelay 的 SIGKILL 兜底只杀组长，极端情况下孙进程仍可能残留
  （本项目子进程都响应 SIGINT，风险被接受并记录）。
