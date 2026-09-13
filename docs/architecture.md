# Architecture — GoForge 最终形态

> 本文从最终形态出发，解释每一层为什么存在、层与层之间的依赖方向、
> 以及这个形状是怎么从十个阶段的演进里"长"出来的（而不是画出来的）。
> 每个阶段的设计细节见对应的 docs/NN-*.md。

## 1. 分层总览

```text
┌────────────────────────────────────────────────────────────┐
│ 进程世界                                                    │
│   cmd/goforge/main.go:                                     │
│   os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))      │
└──────────────┬─────────────────────────────────────────────┘
               │ argv ↓ / exit code ↑
┌──────────────▼─────────────────────────────────────────────┐
│ CLI 层  internal/cli                                       │
│   registry 分发 → runNew / runGenerate / runDev /          │
│                   runTest / runGit / runVersion            │
│   职责：参数解析、exit code 翻译（input→2，runtime→1）、     │
│         信号壳（signal.NotifyContext）、用户提示语           │
└──────┬──────────┬──────────┬──────────┬────────────────────┘
       │          │          │          │
┌──────▼───┐ ┌────▼─────┐ ┌──▼───────┐ ┌▼─────────────┐
│ project  │ │generator │ │ process  │ │ config       │
│ 脚手架   │ │代码生成  │ │ 子进程   │ │ goforge.yaml │
└──────┬───┘ └────┬─────┘ └──┬───────┘ └──────────────┘
       │          │          │              （无依赖，被别人依赖）
       │          │          │
┌──────▼──────────▼───┐ ┌────▼─────────────┐
│ template  渲染      │ │ astedit 语法编辑 │
└─────────────────────┘ └──────────────────┘
       │                      │
┌──────▼──────────────────────▼──────────────────────────────┐
│ os 世界：os.MkdirAll / os.WriteFile / go/parser /          │
│          go/format / embed.FS / yaml.v3                    │
└────────────────────────────────────────────────────────────┘
```

依赖方向永远是**向下**的；任何包都不 import `internal/cli`。
这就是"CLI 是最外层的翻译器"的结构保证。

## 2. 每一层的存在理由

### main.go（进程胶水）
只做三件事：取 os.Args、绑定真实 std 流、os.Exit。
它薄到没有逻辑，所以**永远不需要为它写测试**——逻辑全在可注入的 cli 包里。
为什么 main.go 不能一直长？因为它不可测（进程全局状态无法替换）、
不可复用（其他入口无法调用）、不可读（噪声淹没信号）。

### internal/cli（翻译器）
把"人类敲的 argv"翻译成"对内部函数的调用"，把"内部返回的错误"翻译成
"退出码 + stderr 消息"。它**不实现任何业务**：runGenerate 不知道文件怎么写，
runDev 不知道进程怎么起。判断一个 CLI 层是否干净的方法：
把里面所有函数体换成 panic，编译仍通过（只是行为挂）——说明它只有翻译职责。

### internal/project（项目形态的唯一权威）
"一个 goforge 项目长什么样"只有一个答案的地方：目录表 + 文件表。
它依赖 template（怎么渲染内容）但不知道 generator（往已有项目加东西）。

### internal/template（模板引擎的单一入口）
全仓库唯一 import text/template 的包。20 行代码的价值在于：
模板知识不泄漏、fs.FS 注入让测试用 MapFS、错误信息统一带模板名。

### internal/generator（生成策略）
决定"生成什么文件、放哪里、冲突怎么办、要不要接线"。
它组合 template（渲染）和 astedit（改已有文件），是**策略层**的典型：
不写文件系统原语，只做决策和编排。

### internal/astedit（对已有代码的手术刀）
混合编辑策略：parser 理解与定位、文本插入保注释、format 收尾兼验证。
它是唯一知道 go/ast 的包。

### internal/config（配置管道）
defaults → file → env 的分层合并管道。每一层是独立函数、独立测试。
它不 import 任何业务包（被依赖，不依赖）。

### internal/process（子进程运行器）
三个契约：退出码是结果不是错误；ctx 取消走组 SIGINT + SIGKILL 升级；
流是注入的。dev/test/git 三个命令是它的消费者。

### internal/git（组合的示范）
对 git CLI 的最薄封装。它证明：当外部工具的输出不需要机器解析时，
"组合"比"链接库"便宜一个数量级。

## 3. 依赖注入的三个入口

本项目没有用任何 DI 框架，靠三个惯用法完成全部注入：

1. **io.Writer 注入**（cli.Run）：stdout/stderr 可替换 → 全链路可测；
2. **fs.FS 注入**（project.Create）：embed.FS / MapFS / DirFS 可互换；
3. **函数注入**（config.ApplyEnv 的 lookup；process 的 KillDelay 数值参数）。

原则：**在构造函数参数里放接口，在包内 new 具体类型**。
测试永远不需要 mock 框架——`bytes.Buffer`、`fstest.MapFS`、闭包就是全部。

## 4. 横切关注点的落点

| 关注点 | 落点 | 为什么 |
|---|---|---|
| exit code 约定（0/1/2） | cli 包常量 | 只有 CLI 层关心"给操作系统什么" |
| 错误包装（%w + 上下文） | 各包返回处 | 谁制造错误谁补上下文 |
| 错误分类（Is/As 分流） | cli 层 | 分类影响 exit code，归翻译层 |
| 信号处理 | runXxx 壳 + process 包 | 壳负责装处理器，核接收 ctx |
| 版本号 | cli.version var + ldflags | 构建期注入 |
| 模板数据结构 | 消费方（project.Data / generator.Data） | 数据属于需要它的层 |

## 5. 形状是从哪里来的

最终分层不是画出来的，是每阶段被真实问题逼出来的：

- `Run(args, stdout, stderr) int` ←—— 01：想表驱动测试就得出这个签名；
- fs.FS 注入 ←—— 03：embed 不能写 `..`，语言约束逼出注入；
- generator 独立 ←—— 04：CLI 命令无法复用生成逻辑，策略必须有自己的家；
- astedit 混合策略 ←—— 05：go/printer 的位置语义把纯 AST 路线逼成混合路线；
- config 先于 dev ←—— 06：先打地基，07 的 dev 才能薄；
- process 抽包 ←—— 07：test/git 马上要用，第二次需要就是抽包的时机；
- registry ←—— 10：三处重复的同份知识，积累够了才值得动。

**这条时间线反过来读就是手写顺序**：先让最简单的版本跑通，
让每一次结构变动都对应一个说得出口的动机。

## 6. 如果继续长，会在哪里破

诚实的架构文档要预言自己的死亡点：

- 命令 >15 个、需要 completion → registry 换 cobra；
- 生成代码需要跨层类型检查 → astedit 升级到 go/packages（类型信息）；
- 配置出现全局层/多文件合并 → config 换 viper 或自研合并器；
- 需要"删除已生成代码"→ astedit 的追加式假设破裂，需要 dst 级工具；
- Windows 一等公民 → process 的 Setpgid 换 Job Objects，信号语义重写。

每一个破点都写在对应 ADR 的"触发重审条件"里。
