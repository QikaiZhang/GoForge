# 05 — AST：修改已存在的 Go 源码

> 手写说明书。先读本文，完成 Handwriting Task，再看 internal/astedit 参考实现。
> **本阶段是最能体现"需求倒逼技术选型"的阶段**：请特别认真读 Problem 和 Design Decisions。

## 1. Background

04 阶段结束时有一个明摆着的缺口：`goforge generate handler user` 生成了 handler 文件，
但 `main.go` 根本不知道它的存在。生成的代码编译通过，却没有任何路由被注册——
一个"生成了但没接上"的功能对用户毫无价值。

同时，生成非 user 实体（比如 `order`）会引用不存在的 `model.Order`，项目直接编译失败。

这两个问题的共同点：**需要修改已存在的文件**（main.go、model.go），而不是生成新文件。

## 2. Problem

"修改已有 Go 源文件"的第一直觉是字符串替换：

```go
src = strings.Replace(src, "func registerRoutes(mux *http.ServeMux) {}",
    "func registerRoutes(mux *http.ServeMux) {\n  ...wiring...\n}", 1)
```

它会以四种方式失败：

1. **格式脆弱**：用户跑过 gofmt、golint 加了空行、或者只是换了个缩进——锚点字符串匹配不到。
2. **重复执行**：`generate user && generate user` 会把接线语句插入两遍（幂等性无从谈起）。
3. **语义盲**：无法回答"这个 import 是否已存在""这个类型是否已声明"这类**语义**问题。
4. **注释破坏**：盲目的文本手术可能把新代码插进注释中间。

## 3. Requirements

```bash
cd user-service
goforge generate handler user
# 之后 main.go 应该：
#   1. import 了 user-service/internal/{handler,service,repository}
#   2. registerRoutes 里多了 4 行接线（构造 repo/svc/handler 并 Register）
#   3. 重复执行后 main.go 不变（幂等）
goforge generate handler order
#   4. model.go 里自动出现 type Order struct{ID, Name string}
#   5. 项目仍然 go build 通过
```

另有行为约定：`generate service/repository`（单独）只补 model 类型，不动 main.go；
`generate handler` 会确保 service/repository 文件存在（没有就顺手生成）。

## 4. Scope

不做：删除或重命名已有代码、任意位置的 AST 手术（表达式级改写、函数重构）、
保留插入位置的精确注释、golang.org/x/tools 的完整使用（astutil、packages）。
goforge 只需要"往函数体尾部追加语句、往 import 块追加路径、往文件尾追加类型"三种追加式编辑。

## 5. Design

**先想清楚三种修改源码的手段，需求会自己选出答案**：

| 手段 | 能做什么 | 在本需求上的死穴 |
|---|---|---|
| 字符串替换 | 改文本 | 上面四条死穴全占 |
| 模板 | 生成新文件 | 无法读回/理解已有内容 |
| AST | 理解并定位语法结构 | 插入本身有坑（见下） |

最终设计是一个**三明治**，每层只干自己擅长的事：

```text
go/parser     ──理解──▶  回答语义问题：
                          - registerRoutes 在哪？（FuncDecl 查找）
                          - 接线是否已存在？（BodyContainsStmt：逐条打印语句做子串匹配）
                          - type Order 是否已声明？（HasType：遍历 GenDecl）
                          - import 是否已有？（HasImport）
      ↓ AST 计算出精确的插入点（字节偏移）
文本拼接      ──插入──▶  在偏移处插入新语句文本（保持缩进）
      ↓
go/format     ──收尾──▶  format.Source 规范化 + 语法验证，失败则一个字节都不写
```

### 为什么插入用文本而不是 printer？

这是本阶段用血泪换来的知识。第一版实现是纯 AST 路线：
snippet 解析成 `[]ast.Stmt` 后 append 进 `fn.Body.List`，再用 `format.Node(fset, f)` 打印。
结果是 printer 把**新节点的位置信息和原文件的注释位置混排**，输出长这样：

```go
h :=
    // existing wiring stays untouched
    newHandler()
h.Register(mux)
```

合法（selector 换行合法），但注释被吸进了表达式中间。原因：`go/printer` 依据
token.Pos 的相对大小决定换行和注释归属，合成节点的位置（来自另一个解析空间）
与真实文件的位置无法比邻。成熟的编辑工具要么精心伪造位置（astutil），
要么干脆放弃位置（dave/dst 重新设计 AST）。对 goforge 的需求，文本拼接 +
format.Source 兜底是复杂度和可靠性的最优交点。

### 幂等怎么做

`BodyContainsStmt` 把函数体每条语句用 `format.Node` 打印成规范文本再匹配子串
（如 `NewUserHandler(`）。在**语法树上回答问题**，注释和格式变化都骗不过它。

## 6. Core Concepts

- **AST 是什么**：源码的树形结构化表示。`package main` → `*ast.File`，
  每个声明是 `ast.Decl`（函数 `*ast.FuncDecl`、类型/变量/常量 `*ast.GenDecl`），
  函数体 `*ast.BlockStmt` 里的 `List` 是 `[]ast.Stmt`。
- **go/parser**：源码 → AST。`ParseFile(fset, path, src, parser.ParseComments)`；
  ParseComments 决定注释是否进树（我们的幂等检测和"注释保留"都依赖它）。
- **go/token**：两个角色。`token.FileSet` 是"位置登记处"——每个被解析的文件登记一个
  `*token.File`，把全局递增的 `token.Pos` 翻译成 (file, line, column, offset)；
  `token.Pos` 本身是个 int，`NoPos` 表示"无位置"。
- **go/format**：`format.Source(src)` 等价于 gofmt 命令——规范化排版**并隐式验证语法**
  （语法错的输入直接返回 error），还顺带排序 import 块。
- **位置即语义**：AST 节点携带 Pos/End；printer、注释归属都由位置驱动。
  这是"合成节点"一切麻烦的根源，也是 `fset.Position(node.Pos()).Offset`
  能给出精确字节偏移的原因（我们靠它定位插入点）。
- **snippet 解析**：语句没法直接 parse，要包一层 `package p / func __x() { ... }`
  再取 Body.List；类型声明包一层 `package p` 再取 Decls。

## 7. Design Decisions

1. **混合编辑（AST 定位 + 文本插入 + format 收尾），而不是纯 AST 突变**：
   见 Design 的实验证据。原则：**AST 负责理解和定位，printer 只用于只读打印**。
2. **只做追加式编辑**：插入点要么在函数体闭括号前、import 块内、要么在文件尾。
   追加使"失败即不变"成为可能——任意一步失败，磁盘上的文件保持原样。
3. **format.Source 是验证闸门**：拼接产物先 format，失败就报错不写盘。
   语法上不可能写出损坏文件（比"写完再验证再回滚"简单得多）。
4. **顶层标识符带实体前缀**：集成测试抓到的真实 bug——第二个实体 `order` 的
   `writeJSON`/`Repository`/`ErrNotFound` 与 `user` 的撞名，编译失败。
   解法有三层：模板里所有导出标识符加 `{{.Pascal}}` 前缀、接线变量用
   `userRepo`/`userSvc`、共享 helper（writeJSON）由脚手架统一提供而不是每个 handler 自带。
5. **`generate handler` 确保 service/repository 存在**：main.go 的接线 import 了三个层包，
   缺任何一层编译都会挂。与其让用户记住顺序，不如让 handler 生成成为"添加完整特性"的原子操作。
6. **wiring 失败 ≠ 整体失败**：文件已生成但接线失败时（比如用户删了 registerRoutes），
   返回的 error 信息里明确说"文件已创建，但接线失败"，CLI 以 exit 1 报告。
   部分成功必须被大声说出来，而不是静默吞掉。

## 8. Alternatives

- **纯 AST 突变 + format.Node**：注释错位（真实踩坑记录见 Design）；伪造位置可行但复杂度失控。
- **golang.org/x/tools/go/ast/astutil**：`astutil.AddImport` 处理了 import 的各种边角。
  没选：(a) 它只解决 import，语句追加仍要自己做；(b) 引入 x/tools 的依赖面较大。
  如果项目需要任意 AST 编辑，这是第一块该捡的积木。
- **github.com/dave/dst**：为"可往返编辑"重新设计的 AST（decorations 承载注释/位置）。
  优雅但概念负担重，学习项目应该先体会 go/ast 为什么"不够"再看答案。
- **golang.org/x/tools/go/packages 加载类型信息**：需要类型级判断时才用；
  我们的判断（类型/ import 是否存在）语法级足够。
- **字符串替换 + 正则**：02 阶段的 ReplaceAll 思路，Problem 小节已判死刑。

## 9. Implementation Plan

1. `internal/astedit`：
   a. `Load`（ParseFile + ParseComments）、查询函数 `FuncDecl`/`HasType`/`HasImport`/`BodyContainsStmt`。
   b. snippet 解析 helper：语句包在假函数里、声明包在假包里。
   c. `appendStmtsBeforeBodyEnd`：用 `fset.Position(fn.Body.Rbrace).Offset` 定位，
      处理两种情况——闭括号独占一行（常规）/ 和代码同行（`func f() {}`）。
   d. `addImports`：三种形态——有括号 import 块（最常见）、单行 import、没有 import。
   e. `EnsureFuncWiring`/`EnsureType` 高层 API：读文件 → 查询 → 拼接 → format → 写盘。
      注意拼接顺序：**从文件尾往文件头**（先语句后 import），否则前面的插入会让
      后面基于旧 src 计算的偏移全部失效（真实踩坑 #2）。
2. `internal/generator/wiring.go`：`ensureModelType` + handler 的"确保三层 + 接线"。
3. 模板修正：实体前缀 + respond.go 归脚手架。
4. 集成测试：两个实体（user、order）都接线后 `go build` 必须通过。
5. 手工验证：`generate handler user` 跑两遍，`git diff`（或 cat）确认 main.go 无重复。

## Handwriting Task

不看参考实现，完成：

1. 用 go/parser 写一个"函数查找器"：给定源码和函数名，返回该函数体里有几条语句。
2. `EnsureType(path, typeName, declSrc)`：不存在则追加、存在则不动，二次调用是 no-op。
3. `EnsureFuncWiring(path, funcName, imports, stmtSrc, key)`：幂等接线，
   要过"注释保留"测试（插入后原有注释一个不少）。
4. 单测覆盖 import 的三种形态：括号块、单行 `import "fmt"`、无 import。
5. 集成：scaffold → `generate handler user` 两遍 → `generate handler order`
   → `go build ./...` 通过。

**Expected Behavior**：

- 接线后的 main.go 中 `registerRoutes` 长这样（实体前缀变量、无重复）：

```go
func registerRoutes(mux *http.ServeMux) {
	userRepo := repository.NewUserRepository()
	userSvc := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(svc)
	userHandler.Register(mux)
}
```

- 整个过程前后 `gofmt -l .` 对项目输出为空（产物天然是 gofmt 格式）。
- 手工在 registerRoutes 里加一行注释再跑 `generate handler user --force`：
  注释还在，接线不重复。
- 删掉 registerRoutes 函数再跑 generate：exit 1，错误信息含 "registerRoutes not found"，
  且 handler 文件已生成（部分成功被明确报告）。

## 10. Thinking Questions

1. `token.Pos`、`token.Position`、`token.FileSet` 三者什么关系？
   为什么 Pos 是 int 也能表示"第几个文件第几行"？
2. `BodyContainsStmt` 打印每条语句来匹配子串。为什么不直接在整个函数体文本里找？
   （提示：哪个回答会误报？）
3. 为什么"追加式"编辑能简化失败处理？如果要支持"删除一条接线"（比如 remove 命令），设计会复杂在哪里？
4. format.Source 为什么能当验证器？它和 format.Node(fset, node) 的适用场景差在哪？
5. 如果用户的 main.go 里 registerRoutes 是方法而不是函数、或者在 internal/ 路由器文件里，
   当前设计会怎么失败？你会怎么泛化"接线点"这个概念？

## 11. Testing

```bash
go test ./...
cd $(mktemp -d) && goforge new demo && cd demo
goforge generate handler user
cat cmd/server/main.go                 # 看接线
goforge generate handler user --force  # 再来一遍
grep -c "NewUserHandler(" cmd/server/main.go   # 1，不重复
goforge generate handler order
go build ./...                          # 双实体编译通过
go run ./cmd/server &                   # 路由真的注册了
curl -X POST localhost:8080/user -d '{"id":"1","name":"a"}'
```

## 12. Failure Cases

- **注释被 printer 重排**：纯 AST 突变的真实结局，本文档有原图。
- **偏移失效**：先插 import（文件头部）再插语句（用旧偏移）→ 语句插错位置，
  format.Source 报 syntax error。插入必须"从后往前"。
- **撞名**：多实体场景下 `writeJSON`/`Repository`/`repo` 变量重复声明。
  单实体的测试永远测不出它——集成测试必须用两个实体。
- **忘了 ParseComments**：注释不进 AST，BodyContainsStmt 依然工作，
  但你对"文件里有什么"的理解已经不完整，拼接时更容易踩注释。
- **format 之前就写盘**：拼接 bug 直接损坏用户文件。"不写没格式化过的输出"是铁律。

## 13. Reference Implementation

- `internal/astedit/astedit.go`：包注释完整记录了混合策略的理由；
  `EnsureFuncWiring` 里"从尾往头拼接"的顺序注释是本阶段的精髓。
- `internal/astedit/appendStmtsBeforeBodyEnd`：两种闭括号形态的处理。
- `internal/generator/wiring.go`：接线语句的模板、实体前缀变量的由来。
- `internal/generator/generator_test.go` 的 `TestGenerateHandlerWiresFeature`：
  幂等断言怎么写（count == 1）。

## 14. Git Diff

相对 04-code-generation 新增/修改：

```text
internal/astedit/            新增：混合编辑包（查询/拼接/format 闸门）
internal/generator/wiring.go 新增：ensureModelType + wireFeature
internal/generator/generator.go
                             Generate 拆出 renderLayer；接线失败时返回 path + error
templates/handler/...        writeJSON 移除；接口已是实体前缀
templates/service/...        Repository → {{.Pascal}}Repository
templates/repository/...     ErrNotFound → Err{{.Pascal}}NotFound
templates/project/respond.go.tmpl  新增：脚手架自带共享 helper
internal/project/project.go  scaffoldFiles 加入 respond.go，handler/.gitkeep 退场
internal/generator/generator_test.go  接线断言 + 双实体编译集成
```

## 15. Interview Questions

见 docs/interview.md 的 AST 部分。先自测：
AST 和字符串替换的本质区别？parser/token/format 各自的职责？
为什么插入用文本拼接？怎么保证编辑的幂等性？
