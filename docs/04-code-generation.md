# 04 — Code Generation：Generator 与 Template 的分工

> 手写说明书。先读本文，完成 Handwriting Task，再看 internal/generator 参考实现。

## 1. Background

脚手架只能给你一个空壳。真正的日常是：项目已经存在，现在要**追加**一个 `user` 特性——
handler、service、repository 三层。每次手写这三层，都是重复劳动：
文件路径有规律、类型名有规律、内容结构高度相似。

可规律的东西就该有工具。这一阶段引入 `goforge generate`。

## 2. Problem

没有 generator：

- 每加一个实体要复制粘贴三个文件，再全局替换名字——替换漏一处就是隐形 bug；
- 每个人复制的版本渐渐分叉，项目里出现三种风格的 service；
- 新成员要靠"看别的文件照着写"学习分层约定，约定口口相传必然漂移。

## 3. Requirements

```bash
cd user-service
goforge generate handler user      # → internal/handler/user.go
goforge generate service user      # → internal/service/user.go
goforge generate repository user   # → internal/repository/user.go
goforge generate handler user --force   # 已存在时覆盖
```

行为要求：

- 必须在 goforge 项目内运行（找不到 go.mod → 明确报错）；
- 实体名是 snake_case（`user`、`user_profile`），派生出 `User`/`UserProfile` 等标识符；
- 目标文件已存在且无 `--force` → 报错并提示 `--force`；
- 生成的代码必须能通过 `go build`（对脚手架实体 `user`；见 Scope 的已知限制）。

## 4. Scope

不做：自动接线 main.go（生成的 handler **不会**被服务使用——这是刻意的，05 阶段用 AST 解决）、
为非 user 实体自动补 model 类型（生成的代码会引用不存在的 `model.Order`，
此时项目编译不过；05 阶段一并解决）、模板 FuncMap、多文件生成（migration + entity 之类）、
删除/重命名已生成的代码。

**已知限制要大声说出来**：本阶段只有 `user` 能端到端编译通过，因为脚手架自带的
`model.User` 恰好是它需要的。这个限制是 05 的出场动机。

## 5. Design

核心问题：**"生成什么"和"怎么渲染"是两件事**。

```text
CLI (runGenerate)
  ├─ 校验参数合法性 → ExitUsage
  └─ generator.Generate(".", req, templatesFS)
       ├─ 校验 Kind / Name          ← 策略：什么样的请求是合法的
       ├─ modulePath(go.mod)        ← 上下文：项目是谁
       ├─ 计算目标路径 internal/<kind>/<name>.go
       ├─ 覆盖策略：存在 && !Force → ErrFileExists
       ├─ naming：user → Data{Name, Pascal, Module}
       ├─ template.Render(...)      ← 渲染：模板知识不在这里
       └─ MkdirAll + WriteFile      ← 副作用
```

要点：

- **Data 在 Go 里算好，模板只做替换**。`Pascal("user_profile") → "UserProfile"`
  是纯函数、表驱动可测；如果写进模板 FuncMap，测试就要绕模板引擎。
- **覆盖策略集中在 Generate**，CLI 只翻译错误。策略写进错误信息
  （`use --force to overwrite`），用户不需要查文档。
- **modulePath 读 go.mod**：生成的代码要 import `<module>/internal/model`，
  模块路径只能问项目要。字符串按行解析即可（ADR 里记了何时必须换 x/mod/modfile）。

## 6. Core Concepts

- **snake_case → PascalCase 转换**：按 `_` 切分、每段首字母大写。因为名字校验保证 ASCII，
  可以安全地按字节切片（`part[:1]`）而不是按 rune。
- **哨兵错误 + errors.Is 的第二次出场**：`ErrFileExists` 让 CLI 能区分
  "已存在（提示 --force）"和"其他失败"，但两者都映射 ExitError。
- **input 校验 vs 环境校验**：kind/name 非法是**用法错误**（exit 2）；
  不在项目内、文件冲突是**运行失败**（exit 1）。校验放两层：
  CLI 层为了 exit code，Generate 内部再校验一次（库函数不能信任调用方）。
- **位置参数 + flag 混合解析**：`--force` 可以出现在任意位置，解析时先抽走 flag、
  剩下的必须是恰好两个位置参数。
- **命名即 API**：`Pascal`/`Camel` 是导出函数——命名规则是 generator 的公开契约，
  测试直接锁定它，防止将来悄悄变化。

## 7. Design Decisions

1. **CLI 不直接生成文件**。`runGenerate` 只有"解析→翻译 exit code"两个职责。
   生成逻辑收进 generator 包，才能被测试和（将来的）05 阶段接线逻辑复用。
2. **实体名和项目名是两套规则**（`user-profile` 做项目名合法、做实体名非法），
   所以 `generator.ValidateName` 与 `project.ValidateName` 分开存在，不抽公共"校验器"——
   规则不同，抽象就是假的。
3. **不做"生成时顺手改 main.go"**：用字符串手段改既有代码太脆（见 05 的 Problem），
   本阶段宁缺毋滥，把缺口亮出来。
4. **每种 kind 一个模板文件**（handler/handler.go.tmpl），而不是一个大模板内 `{{if}}` 分叉：
   三层代码差异太大，分文件让每个模板都是"该层长什么样"的完整答案。
5. **kind 映射用 map[Kind]struct{dir,tmpl}**：加一种 kind（比如 `gateway`）只需加模板 + 一行注册。

## 8. Alternatives

- **在 cli 包里直接写生成逻辑**：测试要绕过 CLI；05 的接线逻辑无法复用命名和路径逻辑；拒绝。
- **用正则/字符串替换改 main.go 完成接线**：见 05 的 Problem 小节，这里忍住不做的理由：
  对"已存在的、用户可能手改过的文件"做文本手术，失败模式全是静默损坏。
- **go:generate + 注释驱动**：go:generate 的本质是"项目作者写脚本、go 帮你跑"，
  适合项目内的一次性任务，不适合"跨项目、带策略"的工具；两者互补而非替代。
- **golang.org/x/mod/modfile 解析 go.mod**：正确但为一个只读场景引入依赖；
  写 go.mod 时必须换它（写错 go.mod 会毁掉用户项目）。

## 9. Implementation Plan

1. `internal/generator/naming.go`：`ValidateName`（snake 规则）、`Pascal`、`Camel` + 表驱动测试。
2. `internal/generator/generator.go`：`Kind`/`kindSpecs`/`Request`/`Data`/`ErrFileExists`。
3. `modulePath(dir)`：按行找 `module ` 前缀，处理引号，测试覆盖带引号/缺失/无 module 行。
4. `Generate(dir, req, templates)`：按 Design 的顺序串起来；注意 stat 的三种分支和 03 一样。
5. cli：`runGenerate`——抽 `--force`、校验恰好两个位置参数、input/环境错误分流 exit code。
6. 集成测试：scaffold + 三个 kind + `go build`（`user` 实体必须编译通过）。

## Handwriting Task

不看参考实现，完成：

1. naming：`ValidateName` 拒绝 `user-profile`/`User`/`user__x`/`_user`/`user_`；
   `Pascal`/`Camel` 表驱动测试。
2. `modulePath`：从 go.mod 提取模块路径；不在项目内时错误信息要出现 "go.mod" 字样。
3. `Generate`：路径计算、ErrFileExists、`--force`、渲染、写盘。
4. cli `runGenerate`：unknown kind → 2，非法 name → 2，项目外 → 1，冲突 → 1 + 提示，成功 → 0。
5. 集成测试证明 scaffold + generate 三层后 `go build ./...` 仍通过。

**Expected Behavior**：

- `goforge generate handler user` 输出 `created internal/handler/user.go`；
  文件里有 `type UserHandler`、`NewUserHandler`、路由 `GET /user/{id}`。
- `goforge generate service user_profile` 产出 `user_profile.go`，
  里面类型是 `UserProfileService`。
- 对同一 (kind, name) 重复执行：exit 1，stderr 含 `--force`；
  加 `--force` 后 exit 0 且文件内容被覆盖。
- 在空目录执行：exit 1，stderr 含 `go.mod`。
- `goforge generate`（无参数）与 `goforge generate a b c`：exit 2。

## 10. Thinking Questions

1. 为什么 `Pascal` 放在 generator 包而不是 template 包的 FuncMap？
   两种放法各牺牲什么？
2. `--force` 的语义是"覆盖文件"。如果 handler 文件被用户改过，覆盖会丢失手改内容。
   你能设计一个更安全的策略吗？代价是什么？（提示：备份？diff？三方合并？）
3. modulePath 用字符串解析。什么时候这个决定会变得不可辩护？
4. 生成的 handler 定义了 `UserService` 接口（消费者侧接口）。这和"service 包定义接口、
   handler 引用"相比，耦合方向有什么不同？
5. 如果用户在 monorepo 子目录（`services/user-service/`）里执行 generate，
   当前实现会怎样？"找最近的 go.mod"该怎么实现？

## 11. Testing

```bash
go test ./...
cd $(mktemp -d) && goforge new demo && cd demo
goforge generate handler user && goforge generate service user && goforge generate repository user
go build ./...                 # user 实体必须编译通过
goforge generate handler user; echo $?        # 1
goforge generate handler user --force; echo $? # 0
goforge generate foo user; echo $?             # 2
```

## 12. Failure Cases

- **覆盖不加提示**：`--force` 不存在时用户只能手动删文件重试，工具等于不可重试。
- **Pascal 转换对非 ASCII 崩溃**：`part[:1]` 按字节切，中文名会切出非法 UTF-8——
  前提是 ValidateName 已拒绝非 ASCII。校验和转换必须成对出现。
- **模板渲染成功但代码编译不过**：03 的 parse 契约测试拦不住"数据对了但模板内容错"。
  编译集成测试是最后一道闸。
- **在 goforge 自己的仓库里执行 generate**：go.mod 存在 → 会把 internal/handler/user.go
  写进 goforge 源码树！当前实现无法阻止（这是文档记录的边界，不是 bug）。
- **exit code 把冲突当 usage**：脚本无法区分"参数写错了"和"项目状态不对"。

## 13. Reference Implementation

- `internal/generator/generator.go`：Generate 的分支顺序就是 Design 的顺序；
  `kindSpecs` 是唯一的 kind→模板/目录映射。
- `internal/generator/naming.go`：两个转换函数加一个校验，没有任何 interface。
- `internal/cli/cli.go` 的 `runGenerate`：先抽 flag、再校验位置参数个数、
  再校验内容合法性，最后交给 Generate。注意 CLI 对 Generate 的错误**只翻译不吞**。

## 14. Git Diff

相对 03-template 新增/修改：

```text
internal/generator/generator.go       新增：Kind/Request/Generate/modulePath
internal/generator/naming.go          新增：ValidateName/Pascal/Camel
internal/generator/generator_test.go  新增：命名表、生成断言、覆盖策略、编译集成
internal/cli/cli.go                   generate 命令分发 + runGenerate；usage 增行；version 0.4.0
internal/cli/cli_test.go              generate 的成功/错误/force/项目外用例
docs/04-code-generation.md            本文档
```

注意 diff 里**没有** templates/ 的改动——模板在 03 已就位，
这正是"模板先行"红利：本阶段 diff 几乎全是策略代码。

## 15. Interview Questions

见 docs/interview.md 的 Code Generation 部分。先自测：
为什么要代码生成？怎么避免覆盖用户代码？消费者侧接口是什么？重复生成怎么设计？
