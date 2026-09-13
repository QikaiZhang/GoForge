# ADR 005 — Git 集成：组合 git CLI，不链接 git 库

状态：已采纳（09-git 阶段）

## Context

goforge 需要触达版本控制。可选路径：调用系统 git 二进制（组合）、
链接 go-git 等纯 Go 实现（集成）、或不做。组合的核心契约是退出码与输出流；
集成的核心契约是库 API 的稳定性。

## Decision

1. 通过 internal/process 调用系统 git：`git.Status` 只负责"在 dir 跑 git status"，
   输出流式透传，git 的退出码原样上抛。
2. CLI 层做白名单路由：仅支持 `goforge git status`；其他子命令报用法错误
   并明确指向 git 本体，不做静默透传。
3. 退出码翻译策略：只翻译 goforge 有 actionable 提示的码
   （128 = 非仓库 → 提示 git init + goforge exit 1），其余透传。
4. 不解析 git 的人读输出；若未来需要结构化数据，切 `--porcelain` 或 go-git，并重开 ADR。

## Alternatives

- **go-git**：纯 Go、可编程。拒绝理由：透传场景用不上编程能力；
  二进制体积与依赖面增加；用户看到的输出与原生 git 不一致。
  触发重审条件：需要构造提交/读取对象图等"库级"操作（如自动提交生成代码）。
- **完整透传 `goforge git <anything>`**：实现最省，但价值为零——
  只是 git 的别名，且让"哪些子命令有 goforge 特殊处理"永远含糊；拒绝。
- **os/exec 直调（绕过 process 包）**：丢失 ctx 取消、进程组、
  错误友好化与测试注入；拒绝。

## Trade-offs

- 换来：零依赖、与用户环境中的 git 行为/输出完全一致、git 升级零适配、
  实现总量约 25 行。
- 付出：要求目标机器装有 git（对 goforge 的目标用户是既定事实）；
  128 是 git 的约定而非标准，翻译点硬编码（接受：翻译你知道的，放行你不知道的）；
  将来若需要库级操作，需引入 go-git 并承担两套心智。
