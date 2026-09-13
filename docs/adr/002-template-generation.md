# ADR 002 — 模板化代码生成：text/template + go:embed + fs.FS 注入

状态：已采纳（03-template 阶段）

## Context

02 阶段用 Go 字符串常量 + strings.ReplaceAll 生成项目内容，已被证明不可维护
（转义、无条件逻辑、无法独立校验）。GoForge 需要决定：
生成内容的载体、存储方式（随二进制分发 vs 随源码分发）、以及模板引擎与生成逻辑的边界。

## Decision

1. 生成内容一律使用标准库 `text/template`（不是 html/template）。
2. 模板文件放仓库根 `templates/`，编译期 `//go:embed` 进二进制，
   保证 `go install` 出来的 goforge 自包含。
3. 模板引擎知识收敛在 `internal/template`（全项目唯一 import text/template 的包），
   API：`Render(fsys fs.FS, name string, data any) ([]byte, error)`。
4. 消费方（project、将来的 generator）只依赖 `io/fs.FS`；
   embed.FS 通过 `fs.Sub` 剥掉根目录前缀后传入。
5. 数据用预构造的小结构体（如 `project.Data{Name}`）传入，
   不在模板里做命名转换等逻辑；转换在 Go 代码里算好、可单测。

## Alternatives

- **字符串拼接/替换**：无法处理嵌套反引号（struct tag）、无条件逻辑；拒绝。
- **html/template**：HTML 转义会破坏 Go 源码；拒绝（它只该用于 Web 输出）。
- **第三方模板引擎（pongo2 等）**：无额外价值、增加供应链；拒绝。
- **模板放系统目录/用户目录，运行时读盘**：二进制不自包含，版本会漂移
  （二进制是 v3、磁盘上的模板可能还是 v2）；拒绝，改用 embed。
- **模板里写命名转换逻辑（自定义 FuncMap）**：逻辑藏进模板难以单测；
  改为 generator 预计算字段。FuncMap 保留为将来的逃生舱。

## Trade-offs

- 换来：内容与逻辑分离、模板可独立 parse 校验、测试可用 MapFS、二进制单文件分发。
- 付出：embed 的三个约束（相对路径、禁 `..`、跳过点文件）+ 路径前缀不对称
  （需要 fs.Sub）构成了一组必须记住的坑；模板字段名在编译期不检查，
  依赖"渲染测试 + 契约测试（全部模板必须 parse）"兜底。
