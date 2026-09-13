# GoForge

Go Backend Scaffold / Developer CLI —— 一个为「手写复刻」而设计的 Go 工程学习项目。

```bash
goforge new user-service
goforge generate handler user
goforge dev
goforge test
goforge git status
```

## 这是什么

`goforge` 能创建、生成和管理 Go 后端项目。但这个仓库的真正目的不是发布一个脚手架工具，
而是把 **CLI、文件系统、模板、代码生成、AST、配置、进程管理、Git 集成、工程重构**
这九类后端工程能力，拆成 10 个可以逐个手写的 Git 阶段。

## 怎么用这个仓库（学习路径）

```bash
git checkout 01-cli          # 1. 切到某个阶段
# 2. 读 docs/01-cli.md（刻意不看源码实现）
# 3. 自己重新手写该阶段能力
go test ./...                # 4. 跑测试验证
git diff main...01-cli       # 5. 和参考实现对比，理解差异
```

详细流程见 [docs/handwriting-guide.md](docs/handwriting-guide.md)，
每个阶段的文档索引见 [docs/00-project-overview.md](docs/00-project-overview.md)。

## 分支地图

| Branch | 能力 |
|---|---|
| `01-cli` | argv / 命令分发 / exit code（纯标准库） |
| `02-project-init` | `goforge new` 项目脚手架 |
| `03-template` | text/template + go:embed |
| `04-code-generation` | `goforge generate handler\|service\|repository` |
| `05-ast` | go/ast 修改既有源码（自动接线 main.go） |
| `06-config` | goforge.yaml 配置系统 |
| `07-dev` | `goforge dev` 子进程 / 信号 / graceful |
| `08-test` | `goforge test` exit code 透传 |
| `09-git` | `goforge git status`（组合 git CLI） |
| `10-refactor` | command registry，重划边界 |

## 构建

```bash
go build -o bin/goforge ./cmd/goforge
```

## 声明

本仓库的 Git 历史是为学习设计的 **staged history**（模拟的工程演进顺序），
不是真实公司项目历史；不包含真实用户、业务数据或生产事故。
