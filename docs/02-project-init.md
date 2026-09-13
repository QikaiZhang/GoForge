# 02 — Project Init：`goforge new` 与文件系统

> 手写说明书。先读本文，完成 Handwriting Task，再看 `internal/project` 参考实现。

## 1. Background

01 阶段的 `goforge new` 只会打印计划。一个脚手架工具的立身之本是**真的把项目造出来**。
这一阶段让 `goforge new user-service` 在磁盘上生成一个能 `go run`、能 `go build` 的最小服务。

这一步的全部难度都在 Go 标准库的文件系统 API 和"失败时怎么办"上：
目录已存在怎么办？名字非法怎么办？写到一半磁盘出错了怎么办？

## 2. Problem

没有这个能力（以及没有正确处理失败），会发生：

- 用户拿到一个"打印计划"的玩具；
- `goforge new` 在已有目录上执行 → 静默覆盖用户手写的代码（最恶性的事故）；
- 项目名带空格/大写/路径分隔符 → 生成的 go.mod 直接语法错误，用户排查半天；
- 权限不足时只返回一个裸 `permission denied`，没有任何上下文（哪个路径？哪一步？）。

## 3. Requirements

```bash
goforge new user-service
```

生成（相对 `./user-service/`）：

```text
cmd/server/main.go        可运行的 HTTP 服务（含 /healthz、优雅退出、registerRoutes 接缝）
internal/handler/.gitkeep 层目录先占位（04 阶段才填内容）
internal/service/.gitkeep
internal/repository/.gitkeep
internal/model/model.go   示例领域类型 User
api/.gitkeep
configs/config.yaml       运行期配置示例
go.mod                    module user-service
.gitignore
README.md
```

失败语义：

- 目标目录已存在 → exit 1，提示"已存在，换名或删除"；
- 名字非法 → exit 2（用法错误）；
- 文件系统错误（权限等）→ exit 1，错误信息带出错的路径和操作。

名字规则（刻意比 Go module 规则严格）：`[a-z]` 开头，只含 `[a-z0-9-]`，不以 `-` 结尾。

## 4. Scope

不做：模板系统（内容先硬编码在 Go 字符串里，03 换成 text/template）、
`--dir` 指定输出路径、git init、依赖安装、覆盖已存在目录（永远拒绝，不做 --force）。

## 5. Design

```text
cli.runNew
   ├─ 参数个数校验        → ExitUsage
   ├─ project.ValidateName → 错 → ExitUsage
   └─ project.Create(dir, name)
        ├─ os.Stat(dir)：存在 → ErrDirExists；其他错 → 原样上抛
        ├─ for d := range dirs  → os.MkdirAll(join(dir, d), 0o755)
        └─ for f := range files → os.WriteFile(join(dir, f.path), content, 0o644)
```

关键结构决定：

- **Create 接收 `(dir, name)` 两个参数**：dir 是"往哪写"，name 是"项目叫什么"。
  分开才能在测试里往 `t.TempDir()` 写、而不是污染当前目录。
- **内容集中在 `files.go`**：一个 `[]fileSpec{path, content}` 表 + 若干 `const`。
  "项目长什么样"这个问题只有一个答案的地方。
- **占位符 `{{NAME}}` + `strings.ReplaceAll`**：V1 的模板就是字符串替换。
  它丑（转义、无逻辑、无法测试渲染规则），这正是 03 阶段要引入 text/template 的动机。
- **`ErrDirExists` 哨兵错误**：`Create` 返回 `%w` 包装的错误，cli 层用 `errors.Is`
  区分"已存在"和"真故障"，给出不同提示，但同为 ExitError。
- **main.go 里预埋 `registerRoutes(mux)` 空函数**：这是给 05 阶段 AST 接线留的接缝，
  也是示例服务"能跑但可扩展"的关键。现在只是一个空函数，看起来没什么用——留着。

## 6. Core Concepts

- **os.MkdirAll(path, perm)**：递归建目录，已存在**不是错误**（对比 os.Mkdir）。
  perm 受 umask 影响，0o755 是目录惯例。
- **os.WriteFile(path, data, perm)**：一步完成 create+write+close。0o644 是普通文件惯例。
- **os.Stat + errors.Is(err, fs.ErrNotExist)**：判断存在性的正路。
  `err == nil` 存在；`errors.Is(err, fs.ErrNotExist)` 不存在；**其他错误必须原样上抛**——
  这是最容易漏的第三种情况（权限、IO 故障）。
- **filepath.Join**：跨平台路径拼接。永远不要 `dir + "/" + name`（Windows 会谢你）。
- **哨兵错误 + errors.Is + %w 包装**：包内定义 `var ErrDirExists = errors.New(...)`，
  返回时 `fmt.Errorf("...: %w", ErrDirExists)`，调用方 `errors.Is` 判断。
  这是 Go 1.13+ 的错误链标准玩法。
- **t.TempDir() / t.Chdir()**：测试文件系统操作的标配；`t.Chdir`（Go 1.24+）自动恢复 cwd。
- **`.gitkeep`**：git 不跟踪空目录，占位文件让层目录能进版本库。

## 7. Design Decisions

1. **目录已存在 = 拒绝而不是合并/覆盖**。脚手架工具没有"恢复现场"的能力，
   宁可让用户手动删除。cobra/yarn 等成熟工具同样默认拒绝。
2. **名字校验放在 project 包而不是 cli 包**：名字规则是"项目"的概念，
   cli 只负责"把校验失败翻译成 ExitUsage"。04 阶段的实体名校验是另一套规则，
   两者不能混在一个函数里。
3. **先校验全部、再动磁盘**：Create 一进来就把所有错误可能挡掉
   （名字、目录冲突），磁盘操作开始后基本不会中途失败。
   完美的事务性需要 tempdir+rename，本阶段不值得（见 Alternatives）。
4. **main.go 模板里用 Go 1.22 的 ServeMux 方法匹配**（`"GET /healthz"`）：
   生成的项目 go.mod 声明 go 1.22。学习者的工具链 ≥1.22 是 2026 年的合理假设。
5. **错误都带上下文再上抛**（`fmt.Errorf("write %s: %w", path, err)`）：
   用户看到的应该是 `write cmd/server/main.go: permission denied`，
   不是裸的 `permission denied`。

## 8. Alternatives

- **tempdir + rename 的事务式创建**：原子性完美，但要处理跨设备 rename、
  清理残留 tempdir，对本工具的失败概率而言是过度设计。记入 Failure Cases 备查。
- **afero 之类的文件系统抽象库**：为了"可测试"引入依赖。不需要——
  `t.TempDir()` 让真文件系统测试足够快，抽象层反而挡住了"真的能建目录"这个验证。
- **复制一个 templates/ 静态目录树（cp -r 风格）**：要处理二进制找不到资源文件的问题
  （03 阶段会正面撞上它，见 docs/03），本阶段先把文件系统语义做对。
- **用 flag 包解析 `new` 的参数**：参数只有 1 个位置参数，switch 更清楚。

## 9. Implementation Plan

1. `internal/project/project.go`：`ErrDirExists`、`ValidateName`（表驱动想清楚规则）。
2. `internal/project/files.go`：目录表 + 文件表 + 内容常量（`{{NAME}}` 占位）。
3. `Create(dir, name)`：stat 检查 → MkdirAll 循环 → WriteFile 循环，每步错误都包装上抛。
4. cli 层：`runNew` 接上 ValidateName/Create，翻译 exit code，打印 next steps。
5. 测试：ValidateName 表驱动、Create 结构断言、已存在、权限错误、
   **生成项目必须能 `go build`**（这条测试是后面所有阶段的安全网）。
6. cli 测试：用 `t.Chdir(t.TempDir())` 跑 `new`，断言 exit code 和磁盘结果。

## Handwriting Task

不看参考实现，完成：

1. `ValidateName`：空名/大写/下划线/路径分隔符/`..`/首字符数字/尾随 `-` 全部拒绝。
2. `Create`：目录冲突时返回可被 `errors.Is` 识别的错误；stat 的第三种错误（非 NotExist）上抛。
3. `files.go`：用 `{{NAME}}` 占位符 + ReplaceAll 渲染 go.mod 和 main.go。
4. cli 层接线：非法名 exit 2，已存在 exit 1，成功 exit 0。
5. 写"生成的项目必须能编译"的集成测试（`go build ./...`，Dir 指向生成目录）。

**Expected Behavior**：

- `goforge new demo` 后 `find demo -type f` 与 Requirements 的树一致；
  `demo/go.mod` 第一行是 `module demo`。
- `cat demo/cmd/server/main.go` 里有 `registerRoutes(mux)` 的调用和定义，
  且 `go run ./cmd/server` 能启动（另开终端 `curl localhost:8080/healthz` 得到 `ok`）。
- 二次执行 `goforge new demo`：exit 1，stderr 含 "already exists"。
- `goforge new ../evil`：exit 2，不产生任何文件。
- `go test ./...` 全绿，包括编译集成测试。

## 10. Thinking Questions

1. `os.Stat` 的错误有三种结局：存在/不存在/其他错误。为什么"其他错误"必须上抛而不是当成"不存在"？
2. `os.WriteFile` 的 perm 参数什么时候真正生效？如果文件已存在呢？
3. 为什么 ErrDirExists 要定义在 project 包而不是 cli 包？
4. `strings.ReplaceAll` 的占位符方案，在什么输入下会悄悄产出错误的 Go 代码？
   （提示：如果名字出现在**生成代码的字符串字面量**里呢？03 阶段回头看这个答案。）
5. 如果 Create 写到第 5 个文件时磁盘满了，磁盘上会留下什么？用户重试 `goforge new` 会看到什么？
   这个体验你能接受到什么规模？

## 11. Testing

```bash
go test ./...
go run ./cmd/goforge new demo && find demo -type f | sort
cat demo/go.mod
(cd demo && go build ./... && go run ./cmd/server &)   # 冒烟启动
go run ./cmd/goforge new demo; echo $?                  # 1
go run ./cmd/goforge new Bad_Name; echo $?              # 2
```

## 12. Failure Cases

- **忘了 stat 的第三种错误**：把"stat 失败"一律当"不存在"，于是权限坏掉的目录被当作可写。
- **MkdirAll 用了 0o777**：生成的目录权限过大，安全审查必挂。
- **占位符替换遗漏某个文件**：go.mod 正常但 README 里留着 `{{NAME}}`，
  用户对脚手架的信任瞬间归零——所以"每个文件都出现项目名"要有测试兜底。
- **路径用字符串拼接**：`dir + "/go.mod"` 在 Windows 生成 `\` 混排路径。
- **测试直接在包目录里跑 `new`**：仓库里多出一个 `user-service/`，还污染 git status。

## 13. Reference Implementation

- `internal/project/project.go`：校验、stat 三分支、两个循环。
- `internal/project/files.go`：内容常量表。注意 main.go 模板里
  `registerRoutes(mux)` 的**参数是有名字的**（不是 `_`）——05 阶段 AST 要引用它。
- `substitute()` 是全项目唯一的占位符替换点，03 阶段它会被整体删除。

## 14. Git Diff

相对 01-cli 新增/修改：

```text
internal/project/project.go   新增：ValidateName / Create / ErrDirExists
internal/project/files.go     新增：脚手架全部文件内容（V1 字符串拼接）
internal/project/project_test.go  新增：校验表、结构断言、已存在、权限、编译集成
internal/cli/cli.go           runNew 从"打印计划"变为真创建；version → 0.2.0
internal/cli/cli_test.go      t.Chdir 测真实落盘；已存在/非法名用例
docs/02-project-init.md       本文档
```

## 15. Interview Questions

见 docs/interview.md 的 Filesystem 部分。先自测：
MkdirAll 和 Mkdir 的区别？WriteFile 的 perm 何时生效？怎么正确判断"文件不存在"？
文件已存在时你选择报错还是覆盖的依据是什么？
