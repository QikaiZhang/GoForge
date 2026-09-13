# 03 — Template：从字符串拼接到 text/template

> 手写说明书。先读本文，完成 Handwriting Task，再看 templates/ 与 internal/template 参考实现。
> 本阶段有两个 commit，建议学完文档后分别 diff 体会"发现问题→修复问题"的过程：
> `git diff 02-project-init 03-template`（引入模板）和 03-template 分支内部第二个 commit（go:embed 修复）。

## 1. Background

02 阶段的项目内容是 Go 字符串常量 + `strings.ReplaceAll("{{NAME}}", name)`。
它已经出现了维护信号：README 里要嵌 markdown 代码块就得拼反引号、
想生成带条件的内容（比如"带 Docker 的项目"）就无能为力、转义规则全靠人肉。

程序化地生成**文本**，是代码生成器的核心问题。Go 的答案是 `text/template`。

## 2. Problem

字符串拼接/替换方案会持续恶化：

- **转义地狱**：Go 模板里写 Go 代码，代码里又有反引号字符串（struct tag）→ 拼接方案直接爆炸；
- **没有逻辑**：占位符替换无法表达"如果用户要 X 就生成 Y"；
- **双语言混乱**：改一处生成的代码要同时懂"宿主 Go 代码"和"占位符协议"；
- **无法独立校验**：模板写错了要到运行时才发现，而且报错指不出模板里的哪一行。

## 3. Requirements

1. 引入 `templates/` 目录，把 02 的全部内联字符串迁出去：

```text
templates/
├── project/     goforge new 用
│   ├── main.go.tmpl
│   ├── go.mod.tmpl
│   ├── model.go.tmpl
│   ├── config.yaml.tmpl
│   ├── gitignore.tmpl      ← 注意：不是 .gitignore.tmpl（原因见 Design #3）
│   └── README.md.tmpl
├── handler/handler.go.tmpl      ← 04 阶段消费，本阶段先落位
├── service/service.go.tmpl
└── repository/repository.go.tmpl
```

2. 新增 `internal/template` 包，签名 `Render(fsys fs.FS, name string, data any) ([]byte, error)`，
   成为全项目唯一 import `text/template` 的地方。
3. `project.Create` 改为 `Create(dir, name string, templates fs.FS) error`——模板从哪来由调用方决定。
4. **二进制必须自包含**：`goforge` 在任何目录下运行都不能依赖源码 checkout（这是本阶段第二个 commit 的主线）。
5. 行为零回归：02 阶段的全部测试不改断言继续通过。

## 4. Scope

不做：模板继承/布局（block + define）、自定义 FuncMap（04 的命名转换用"预先算好放进 data"实现，见 docs/04）、
html/template（生成的是 Go 源码和配置，不是网页；两套转义规则完全不同）、模板热加载。

## 5. Design

三个决定串起整个阶段：

1. **`fs.FS` 是注入边界**。`Render` 与 `Create` 都只认 `io/fs.FS` 接口：
   生产环境传 `embed.FS`，测试传 `fstest.MapFS`，调试可传 `os.DirFS`。
   这让"模板内容对不对"和"渲染流程对不对"可以分开测试。
2. **项目模板的数据只有一个字段**：`project.Data{Name string}`。
   main.go/go.mod/README 只需要项目名。生成器模板（04）需要更多字段（Pascal、Module），
   那是 generator 包的 Data，不该混进来。
3. **`gitignore.tmpl` 而非 `.gitignore.tmpl`**：`//go:embed templates` 的普通 pattern
   **不匹配 `.` 或 `_` 开头的文件**。要么 `all:templates` 前缀，要么不给模板文件起点名。
   我们选后者，并在渲染时写入真实的 `.gitignore` 路径——模板名和输出名本来就该解耦。
4. **embed 的路径前缀坑**：`//go:embed templates` 塞进 FS 的路径是 `templates/project/...`
   （带根目录名），而 `os.DirFS("templates")` 里的路径是 `project/...`（不带）。
   用 `fs.Sub(goforge.Templates, "templates")` 把子树剥出来，
   保持"模板名相对于 templates/ 目录"这个约定对两种 FS 一致。

## 6. Core Concepts

- **text/template 三步**：Parse（模板文本 → 语法树）、Execute（语法树 + data → 输出）、
  `ParseFS`（直接从 fs.FS 读一批模板）。模板语法：`{{.Field}}`、`{{if}}`、`{{range}}`、
  `{{/* comment */}}`；缺字段默认输出 `<no value>`（危险！见 Testing）。
- **`io/fs` 统一文件系统抽象**（Go 1.16+）：`embed.FS`、`os.DirFS`、`fstest.MapFS`、`fs.Sub`
  都实现/操作同一个接口。这是标准库里"小接口大能量"的典范。
- **go:embed**：编译期把文件打进二进制。三个硬约束：
  pattern 相对**源码文件所在目录**；不能写 `..`；普通 pattern 跳过 `.`/`_` 开头文件。
- **fstest.MapFS**：`map[string]*fstest.MapFile` 就是一个内存文件系统，单测不再碰磁盘。
- **fs.Sub(dir)**：返回"以 dir 为根"的子文件系统，用于剥掉 embed 的前缀。
- **parse 失败 vs execute 失败**：前者是模板语法错（开发期就该被测试拦住），
  后者是数据不匹配（运行期错误），错误信息要能区分（我们包装时分别写了 parse/execute）。

## 7. Design Decisions

1. **模板作为独立包（internal/template）而不是 project 的私有函数**：
   04 阶段 generator 也要渲染，提前把"text/template 的知识"收敛到一个包。
   包里只有 20 行，看起来"太小"——但这 20 行是全项目唯一的模板 API 面。
2. **Create 接收 fs.FS 而不是自己 embed**：embed 指令不能写 `..`，
   internal/project 根本 embed 不了仓库根的 templates/。这个语言约束倒逼出了正确的注入设计。
3. **缺字段要报错而不是 `<no value>`**：渲染时数据结构少给了字段，
   静默输出 `<no value>` 会生成损坏的 Go 文件。对代码生成器，
   "宁可不产出，不可产出错的"。测试 TestRenderDataMismatch 固定这个行为。
4. **generator 模板本阶段就落位**：目录结构和"模板先行"的思路在 03 定型，
   04 只写 generator 逻辑。也借机验证"所有模板必须能 parse"这条契约测试。

## 8. Alternatives

- **继续字符串拼接 + ReplaceAll**：撑不过 struct tag 的反引号，且无法条件化。02 的注释里就预告了它的死期。
- **golang.org/x/text/template**（不存在的东东）/**第三方模板引擎**（pongo2、hero）：
  text/template 是标准库、编译期类型检查（模板里访问的字段在 Execute 时才检查，但字段名拼错会在测试中暴露）、
  生态默认。第三方引擎没有为代码生成提供额外价值。
- **把 templates/ 装到 /usr/share/goforge 之类的系统路径**：Linux 发行版的做法，
  但用户是 `go install` 装的二进制，没有包管理器帮你放资源文件。embed 是 Go 的正解。
- **html/template**：自动 HTML 转义会破坏 Go 源码（`<`、`&`、引号都会被转义）。生成代码必须用 text/template。

## 9. Implementation Plan

1. 建 `templates/` 目录树，把 02 的 files.go 内容逐个迁成 `.tmpl`，
   `{{NAME}}` 改成 `{{.Name}}`。这一步可以机械执行。
2. 写 `internal/template/template.go`：Render 三步（ParseFS → Execute → 返回 bytes），
   两类错误分别包装。写它的单测（MapFS 三连：正常/缺失/语法错 + 数据不匹配）。
3. 改 `internal/project`：fileSpec 的 content 字段换成 tmpl 名；Create 加 fs.FS 参数；
   删 files.go 和 substitute。项目测试改传 `os.DirFS("../../templates")`（测试 cwd = 包目录）。
4. cli：把 FS 传给 Create。此时先用 `os.DirFS("templates")`（刻意保留 V1 缺陷），
   跑通全部测试后提交（commit 1）。
5. 写根包 `templates.go`：`//go:embed templates` + `fs.Sub` 剥前缀；
   cli 换用 embed FS；删除 cli 测试里的模板 fixture；
   加"所有内嵌模板必须 parse"契约测试；在源码目录外实际跑一次二进制验证（commit 2）。

## Handwriting Task

不看参考实现，完成：

1. 迁移模板：`templates/project/` 下 6 个 .tmpl，README 模板里保留 markdown 代码块（体会转义差异）。
2. `internal/template.Render` + 4 个单测（正常/缺失/语法错/数据缺字段）。
3. `Create(dir, name, templates fs.FS)` 重构，删掉 ReplaceAll。
4. 契约测试：遍历内嵌 FS，断言每个 .tmpl 都能 parse。
5. 让二进制在**没有源码的目录**里成功 `goforge new demo`。

**Expected Behavior**：

- `go test ./...` 全绿（02 的测试一行断言都不用改）。
- 生成项目和 02 产出的文件**逐字节一致**（这是"纯重构"的验收标准：
  `goforge new demo` 后 `diff -r` 两个不同阶段生成的项目目录，应当无差异）。
- 二进制拷贝到 /tmp 下任意目录，`goforge new demo` 依然成功。
- 把某个 .tmpl 内容故意改坏（比如删一个 `}}`），`go test ./...` 应当在 parse 契约测试上变红。

## 10. Thinking Questions

1. `Render` 的第一个参数为什么是 `fs.FS` 接口而不是 `embed.FS` 具体类型？
   如果写死 embed.FS，测试会变成什么样？
2. embed.FS 里 `templates/project/go.mod.tmpl` 的路径前缀从哪来？fs.Sub 解决了什么不对称？
3. 模板里 `{{.Name}}` 在 data 没有 Name 字段时会发生什么？对代码生成器来说这是不是可以接受的默认？
4. 为什么 `gitignore.tmpl` 不能叫 `.gitignore.tmpl`？两条路（all: 前缀 vs 改名）各牺牲了什么？
5. 如果一个模板渲染成功但内容是残缺的 Go 代码（比如少了半个括号），哪个环节会兜底？（提示：04 的编译集成测试）

## 11. Testing

```bash
go test ./...
go build -o /tmp/gf ./cmd/goforge
cd /tmp && rm -rf tpl-check && mkdir tpl-check && cd tpl-check
/tmp/gf new demo && cat demo/go.mod      # 二进制自包含验证
go test -run TestAllEmbeddedTemplatesParse ./...   # 契约测试
```

## 12. Failure Cases

- **embed 路径前缀**：`ParseFS(goforge.Templates, "project/go.mod.tmpl")` →
  `pattern matches no files`。真实撞上过的坑，用 fs.Sub 解决。
- **模板名带点**：`.gitignore.tmpl` 不进 embed.FS，运行时"模板不存在"。
- **`<no value>` 污染**：字段名拼错（`{{.name}}` vs `{{.Name}}`）不报错、静默输出小写占位符，
  生成的 go.mod 变成 `module <no value>`。测试必须断言渲染结果而不是"没报错"。
- **用 html/template 生成 Go 代码**：所有 `<`、`&` 被转义成 `&lt;`、`&amp;`，产物不可编译。
- **测试里用真磁盘路径定位 templates/**：一旦测试从别的目录触发（IDE、CI 工作区复制）就找不到，
  用 MapFS 或"测试 cwd = 包目录"的 `../../templates` 约定。

## 13. Reference Implementation

- `internal/template/template.go`：20 行，全项目唯一 import text/template 的地方。
- `templates.go`（仓库根）：`//go:embed templates`。为什么在根上？embed 不许 `..`，
  而 templates/ 在仓库根——语言约束决定了包的位置，这类"约束驱动的结构"值得记住。
- `internal/project/project.go`：fileSpec 里空 tmpl 名表示"空文件"（.gitkeep 不需要模板）。
- `internal/cli/cli.go`：`templatesFS` 包级变量 + fs.Sub，panic 只可能由编程错误触发。

## 14. Git Diff

相对 02-project-init 的两个 commit：

```text
commit 1 — refactor(template): replace string substitution with text/template
  templates/**             新增（内容迁自 files.go，{{NAME}} → {{.Name}}）
  internal/template/       新增：Render + 4 个单测
  internal/project/        Create 加 fs.FS 参数；files.go 删除
  internal/cli/            Create 调用传入 os.DirFS("templates")（V1 缺陷，刻意保留）

commit 2 — fix(template): embed templates into the binary
  templates.go             新增：//go:embed templates
  templates_test.go        新增：所有内嵌模板必须 parse
  internal/cli/            os.DirFS("templates") → fs.Sub(goforge.Templates, "templates")
                           cli 测试中的模板 fixture 全部删除（自包含后不再需要）
```

对比 commit 1 和 commit 2 里 cli 测试的变化：**生产代码的抽象（fs.FS 注入）
让测试随着部署方式改变几乎零修改**——这就是抽象买到的保险。

## 15. Interview Questions

见 docs/interview.md 的 Template 部分。先自测：
template 和字符串拼接的本质区别？ParseFS 的参数顺序？embed 有哪些约束？
`<no value>` 什么时候出现、怎么禁掉？
