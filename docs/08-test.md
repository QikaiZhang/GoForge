# 08 — Test：`goforge test` 与退出码透传

> 手写说明书。先读本文，完成 Handwriting Task，再看 cli.go 里 runTest 参考实现。
> 本阶段的代码很少——**它的价值在于让你亲眼看到 07 阶段三个契约的复利**。

## 1. Background

`goforge dev` 让子进程跑起来了。现在把同样的能力给 `go test`：
`goforge test` 应该跑 `go test ./...`，测试通过就 0，测试失败就 1，
输出原样透传——像一个透明的包装层。

它同时是教程上最有针对性的一课：**为什么不能只判断 `err != nil`？**

## 2. Problem

一个只判断 error 的包装层长这样：

```go
if err := exec.Command("go", "test", "./...").Run(); err != nil {
	log.Fatalf("goforge test failed: %v", err)   // exit 1
}
```

三个实际后果：

1. **语义颠倒**：测试失败时（`go test` exit 1），用户看到的是
   "goforge test failed: exit status 1"——好像 goforge 坏了，其实是测试红了。
2. **CI 分辨率丢失**：外部脚本无法区分"测试失败"（该修代码）和
   "go 没安装"（该修环境），两者都变成笼统的 1。
3. **参数无法透传**：`goforge test -run TestX` 这种日常需求没有设计位置。

## 3. Requirements

```bash
cd user-service
goforge test                    # go test ./...
goforge test -- -run TestUser   # "-- 之后的参数原样给 go test"
goforge test -- -v -count=1
```

- 测试全过 → exit 0；有失败 → exit **1**（透传，不是 goforge 的错误码语义）；
- `go test` 的 stdout/stderr 原样流式透传（用户要看实时输出）；
- 不在项目内 → exit 1 + 明确提示；
- `--` 之前出现任何参数 → exit 2（用法错误）；
- Ctrl+C 能中断正在跑的测试。

## 4. Scope

不做：测试结果的美化/汇总（gotestsum 的领域）、缓存控制、覆盖率聚合、
watch 模式。goforge test 的定位是"透明包装 + 项目检测"，故意保持最薄。

## 5. Design

```text
runTest（信号壳：signal.NotifyContext）
   ↓
runTestContext(ctx, args)
   ├─ 参数切分：第一个 "--" 把 argv 分成 head（必须为空）和 passthrough
   ├─ go.mod 预检查
   └─ process.Run(ctx, "go", ["test","./..."] + passthrough, 流透传)
        ↓
   return code        ← 失败的测试通过退出码说话，goforge 不加戏
```

**核心对比**：`err != nil` 把两种完全不同的事混在一起——
"子进程跑完了、结果是非零"（结果）和"子进程根本没跑起来"（异常）。
07 阶段的 `Run` 把它们拆成 `(code, nil)` 和 `(0, err)` 两个通道，
本阶段命令因此薄到近乎没有逻辑——**薄，就是抽象买到的证据**。

## 6. Core Concepts

- **退出码是 CLI 的语言**：`go test` 的退出码约定——0 全过、1 有失败、
  2 构建失败（包编译不过）、其他为工具内部错误。透传意味着这些语义逐层可达：
  CI 里 `goforge test || notify` 的判断和 `go test || notify` 完全一致。
- **`--` 约定**：argv 的通用惯例（getopt、docker、npm 都用）——
  `--` 之后的一切不再属于当前命令，原样交给它调用的下一层。
  实现只是"按第一个 `--` 把切片一分为二"。
- **流式 vs 缓冲**：Stdout 直接给 io.Writer（exec 内部建管道 + io.Copy），
  测试输出实时可见。如果先 `cmd.Output()` 收集再打印，长测试就"沉默几分钟然后一次性倾倒"。
- **ExitError 的正确打开方式**（复习 07）：`errors.As` 取 ExitCode，
  信号死亡用 WaitStatus 翻译 128+n。

## 7. Design Decisions

1. **test 命令自己不接受任何 flag**，全部 flag 语义让给 `go test`（通过 `--`）。
   理由：包装层每多一个 flag，就多一条"这个 flag 该给谁"的心智负担；
   `--` 让边界永远清晰。代价是 `goforge test -v` 会得到用法错误——这是刻意的教学。
2. **go.mod 预检查而不是等 go 报错**：`go test ./...` 在非模块目录的报错
   （`go: cannot find main module`）对新人是黑话；goforge 知道语境，
   就应该说人话（同 dev 的决策）。
3. **复用 runDev 的"壳/核"分离**：`runTest` 装信号处理器，`runTestContext` 可注入 ctx。
   同一模式第二次出现——是模式而不是巧合的证据。
4. **不加项目外缓存、不加汇总**：与 04 阶段"generator 不碰 main.go"同理，
   工具的每一层只做该层的事；美化输出是另一个工具的事。

## 8. Alternatives

- **解析 go test 输出做汇总**（`ok/FAIL/---` 行）：输出格式是人读的、不是 API，
  `go test -json` 才是机器接口；汇总属于另一个特性，本阶段 scope 外。
- **用 `go tool test2json` 做结构化转发**：正确方向（真实工具这么干），
  但会把本阶段的学习焦点从退出码转向输出协议；记入"深入方向"。
- **把 `--` 之前的 `-v` 之类静默转发而不是报用法错误**：便利但模糊了边界，
  用户永远学不会 `--` 的存在；刻意严格。

## 9. Implementation Plan

1. `runTest` / `runTestContext` 壳核分离（抄 dev 的模式，这次应该不假思索）。
2. `--` 切分：找到第一个 `--`，前后两段；`--` 前非空 → usage。
3. go.mod 预检查 → `process.Run` 组装 `["test","./..."]+passthrough` → 透传 code。
4. 测试：参数用法、项目外、**真实失败套件 → exit 1 且输出含 FAIL/boom**、
   真实通过套件 → exit 0、`-- -run TestX -v` 真的过滤了测试。
5. 手工链路：故意写个失败测试，观察输出和 `echo $?`。

## Handwriting Task

不看参考实现，完成：

1. 壳核分离的 `runTest`/`runTestContext`。
2. `--` 切分 + 用法错误（`--` 前有参数 → exit 2，stderr 给出 `--` 用法提示）。
3. 集成测试：失败套件断言 exit 1 + 输出含失败信息；通过套件 exit 0。
4. 透传断言：`-- -run TestOne -v` 时 TestTwo 不得出现在输出里。

**Expected Behavior**：

- 有失败测试的项目：`goforge test; echo $?` → `1`；
  屏幕上看到的是 go 原生的 `--- FAIL: TestAlwaysFails ... boom`，没有 goforge 前缀。
- 全过的项目：exit 0。
- `goforge test -- -run TestNope` → exit 0 且 "no tests to run"（证明过滤到达了 go）。
- 空目录：exit 1，stderr 含 `go.mod not found`。

## 10. Thinking Questions

1. `go test` 退出码 2（编译失败）和 1（测试失败）都会被透传。
   你觉得 goforge 应该对 2 做特殊处理（比如提示"先 build"）吗？利弊？
2. `--` 切分如果遇到第二个 `--` 怎么办？按"第一个生效"还是"全部转发"？
   参考你熟悉的工具（docker exec、kubectl exec）是怎么定义的。
3. 为什么透传退出码的命令，自己的"项目外错误"用 1 而不是 2？
   （提示：对照 usage 错误的定义，以及 go test 自己用 1 表达什么。）
4. 如果要给 `goforge test` 加 `--watch`（保存后重跑），架构上要动哪里？
   ctx 的角色会变成什么？

## 11. Testing

```bash
go test ./...
cd $(mktemp -d) && goforge new demo && cd demo
cat > main_test.go <<'EOF'
package main

import "testing"

func TestBoom(t *testing.T) { t.Fatal("kaboom") }
EOF
goforge test; echo $?          # 1，输出是 go 原生的
goforge test -- -run TestNope; echo $?   # 0, "no tests to run"
goforge test -v; echo $?       # 2（用法错误）
```

## 12. Failure Cases

- **把 ExitError 打印成自己的错误**：用户看到 `goforge test failed: exit status 1`
  会去 goforge 仓库提 issue——真实的反面教材模式。
- **`--` 切分用了 `strings.Split(a, "--")`**：会把值里含 `--` 的参数切碎；
  argv 切分必须按元素不是按字符。
- **缓冲收集输出再打印**：长测试沉默、Ctrl+C 时缓冲丢失。永远流式。
- **忘记 passthrough 的空切片**：`args[:i+1:]` 之类的越界/别名 bug——
  切片表达式共享底层数组，修改 passthrough 可能污染 head。

## 13. Reference Implementation

- `internal/cli/cli.go`：`runTest` / `runTestContext` 加起来 40 行——
  和 07 阶段对照，体会"复用一个良好抽象"时新命令的体积。
- `internal/cli/cli_test.go`：`TestTestExitCodePassthrough` 的断言措辞
  （"passthrough"）就是本阶段的契约名。

## 14. Git Diff

相对 07-dev 新增/修改：

```text
internal/cli/cli.go       test 命令分发；runTest/runTestContext；usage 增行；version 0.8.0
internal/cli/cli_test.go  用法/项目外/真实透传（失败与通过）/-- 过滤 五组测试
```

没有新包、没有新依赖——本阶段 diff 小是**设计正确**的信号：
process 包在 07 阶段多付的复杂度，在这里一次性收回。

## 15. Interview Questions

见 docs/interview.md 的 Process 部分。先自测：
`go test` 的退出码各代表什么？`--` 是什么约定？
为什么这个命令不需要判断 err != nil 以外的任何东西？
