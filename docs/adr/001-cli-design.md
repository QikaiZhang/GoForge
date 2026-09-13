# ADR 001 — CLI 设计：标准库手写分发 + Run(args, stdout, stderr) int

状态：已采纳（01-cli 阶段）

## Context

GoForge 的入口是一个二进制 `goforge`。CLI 层的设计决定后续所有阶段的可测试性：
如果分发逻辑和进程全局状态（os.Args/os.Stdout）耦合，任何一层都无法单测。
同时，Go 生态存在成熟的第三方 CLI 框架（cobra、urfave/cli），需要决定是否引入。

## Decision

1. 全部 CLI 逻辑收敛到一个纯函数：
   `Run(args []string, stdout, stderr io.Writer) int`，
   args 不含程序名，返回值即进程 exit code。
2. `cmd/goforge/main.go` 只做进程胶水：`os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))`。
3. 命令分发用 `switch args[0]`，不引入框架。
4. Exit code 约定：0 成功、1 运行失败、2 用法错误，以命名常量固化。
5. 错误信息一律走 stderr；usage 在"主动求 help"时走 stdout。

## Alternatives

- **cobra**：生态标准、自动生成 completion/帮助。拒绝理由：命令数 <10 时装配成本大于收益；
  且会掩盖"argv→exit code"这个本项目的核心学习对象。触发重审条件：命令 >10 个或需要补全。
- **flag.FlagSet**：标准库、每命令独立 flag 集。拒绝理由：本阶段 flag 只有全局两个，
  FlagSet 的装配模板会淹没分发主干；04 阶段若 flag 增多可局部引入。
- **Run 返回 error，main 翻译 exit code**：无法区分"用错了"（2）和"失败了"（1），
  翻译规则会散落在 main；拒绝。

## Trade-offs

- 换来：零依赖、全链路可测（bytes.Buffer 即可捕获输出）、CLI 本质对读者完全透明。
- 付出：命令很多时 switch + 手写 usage 会重复（已知且接受，10-refactor 用 command registry 偿还）；
  复杂 flag 语法（合并短 flag、`=` 赋值）需要自己实现（本项目用不到）。
