# 00 — Project Overview：GoForge 全景图

> **重要声明**：本仓库的 Git 历史是为了学习而**专门设计的 staged history**，
> 不是某个真实公司项目的开发历史。它模拟的是"一个工具从零长出来"的合理工程演进顺序，
> 不包含任何真实用户、业务数据、性能数据、生产事故或公司背景。
> 每个阶段（branch）都是一个可以独立运行、独立测试的快照。

---

## GoForge 是什么

`goforge` 是一个 Go 后端脚手架 CLI。它做三类事情：

1. **创建项目**：`goforge new user-service` → 生成一个可运行、可测试的 Go 服务骨架。
2. **生成代码**：`goforge generate handler user` → 向已有项目追加 handler/service/repository 层代码。
3. **驱动工程动作**：`goforge dev` / `goforge test` / `goforge git status` → 包装子进程。

它**刻意不做**的事情：RPC、Redis、Kafka、Kubernetes、微服务治理、云部署、插件生态。
这些和"CLI/文件系统/模板/AST/进程管理"这些核心 Go 工程能力无关，留给其他项目。

---

## 模块地图

```text
goforge (最终形态)
   │
   ├── CLI (internal/cli) ............ 接收 argv，分发命令，返回 exit code
   │
   ├── Project (internal/project) .... 项目初始化：目录树、go.mod、main.go
   │
   ├── Template (internal/template) .. 模板加载、渲染（text/template + go:embed）
   │
   ├── Generator (internal/generator)决定"生成什么文件、生成到哪里"，调用 Template
   │
   ├── AST Edit (internal/astedit) ... 修改"已存在"的 Go 源文件（go/ast）
   │
   ├── Config (internal/config) ...... goforge.yaml 的加载、默认值、校验、env 覆盖
   │
   ├── Process (internal/process) .... 子进程管理：exec、stdio 转发、信号、exit code
   │
   └── Git (internal/git) ............ 通过调用 git CLI 组合出 git 能力
```

### 为什么每个模块必须存在

| 模块 | 它解决的问题 | 如果没有它 |
|---|---|---|
| CLI | 进程入口只有一个，命令有很多 | main.go 里全是 if/else，无法测试 |
| Project | 创建项目的文件系统操作集中在一处 | 创建逻辑散落在 CLI 命令里，无法复用和测试 |
| Template | 生成的内容（代码文本）需要和生成的逻辑分离 | 字符串拼接写代码，转义地狱，无法维护 |
| Generator | "生成什么、到哪、覆盖策略"是策略问题，与"怎么渲染"无关 | 渲染和策略耦合，换模板就要改业务逻辑 |
| AST Edit | 修改已有文件（把新 handler 接线进 main.go） | 字符串替换对已存在代码极其脆弱 |
| Config | 工具行为需要默认值和按项目定制 | 每条命令都要用户敲全参数 |
| Process | dev/test/git 本质是"起子进程并管理它" | exec.Command 的信号、exit code 处理四处复制粘贴 |
| Git | 版本控制是脚手架的自然延伸 | 用户要自己切出去敲 git |

### 数据流（最终形态）

```text
用户敲 argv
   ↓
os.Args → cli.Run(args, stdout, stderr) → 返回 exit code → os.Exit
   ↓                                    ↘ stderr（错误）/ stdout（正常输出）
Command（new / generate / dev / test / git）
   ↓
Application Logic
   ├── project.Create()      → template.Render()  → os.MkdirAll / os.WriteFile
   ├── generator.Generate()  → template.Render()  → astedit.WireHandler() → 文件
   ├── config.Load()         → yaml.v3 decode     → 结构体 + 默认值 + 校验
   ├── process.Run(ctx,...)  → exec.Command       → 子进程 stdin/stdout/stderr/exit code
   └── git.Status()          → process.Run(ctx, "git", "status")
   ↓
Filesystem / OS Process（真实世界的边界）
```

---

## 阶段地图（10 个 branch，每个只引入一个主能力）

| Branch | 引入的能力 | 关键 Go 知识 |
|---|---|---|
| `main` | 项目全貌文档 | — |
| `01-cli` | argv 解析、命令分发、help/version、exit code | os.Args、io.Writer、os.Exit |
| `02-project-init` | `goforge new` 真正创建项目 | os.MkdirAll、os.WriteFile、filepath.Join |
| `03-template` | 字符串拼接 → text/template + go:embed | template 语法、embed.FS、fs.FS |
| `04-code-generation` | `goforge generate ...` 生成层代码 | 命名转换、覆盖策略、错误设计 |
| `05-ast` | AST 修改已有源码（接线 main.go） | go/ast、go/parser、go/token、go/format |
| `06-config` | goforge.yaml 配置系统 | struct 映射、默认值、校验、env 覆盖 |
| `07-dev` | `goforge dev` 跑子进程 | os/exec、context、signal、graceful |
| `08-test` | `goforge test` 包装 go test | exit code ≠ err != nil |
| `09-git` | `goforge git status` | CLI 组合、子进程复用 |
| `10-refactor` | 回头重划边界（command registry） | 职责划分、何时需要抽象 |

**刻意保留的演进痕迹**（不要当成"做错了"）：

- 02 用字符串拼接生成文件内容，03 才换成 template —— 这就是"拼接→模板"的演进。
- 04 的 handler 生成完不会被 main.go 使用（没有接线），05 用 AST 补上 —— 这就是"模板→AST"的演进。
- 01~09 的命令分发是 switch + 硬编码 help 文本，10 才重构成 registry —— 这就是"先跑通→再划边界"。

---

## 学习方式（详见 docs/handwriting-guide.md）

```text
git checkout 01-cli
  → 读 docs/01-cli.md（不要看源码实现）
  → 自己手写该阶段能力
  → go test ./... 验证
  → git diff 对比 AI 实现，理解差异
  → 进入下一个 branch
```

每个阶段的文档都包含 Background / Problem / Requirements / Design / Handwriting Task 等
15 个固定小节，专为"先看文档再手写"设计。
