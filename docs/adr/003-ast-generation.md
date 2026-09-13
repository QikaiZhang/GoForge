# ADR 003 — 修改既有 Go 源码：AST 定位 + 文本插入 + format 收尾

状态：已采纳（05-ast 阶段）

## Context

`goforge generate handler` 必须修改已存在的文件：把接线语句追加进 cmd/server/main.go
的 registerRoutes、把缺失的实体类型追加进 internal/model/model.go。
对"已存在的、可能被用户改过格式的文件"做修改，需要在字符串替换、纯 AST 突变、
第三方编辑库之间做选择。

## Decision

1. 编辑策略为三段式：
   - **go/parser** 解析并回答全部语义问题（函数定位、类型/ import 是否存在、
     基于语句级打印的幂等检测）；
   - **文本拼接**在 AST 计算出的字节偏移处插入内容（import 块、函数体闭括号前、文件尾）；
   - **go/format 的 format.Source** 对结果做规范化 + 语法验证，验证失败则不写任何字节。
2. 只支持追加式编辑；写盘的输出必须先通过 format.Source。
3. 生成代码的所有顶层标识符与接线变量带实体前缀，避免多实体撞名；
   跨实体的共享 helper（writeJSON）由脚手架统一提供。
4. `generate handler` 是原子语义：确保 service/repository 文件与 model 类型存在，
   然后接线；接线失败以 error 报告但明确说明文件已创建。

## Alternatives

- **字符串替换/正则**：格式脆弱、不幂等、语义盲；04 阶段已知的死路。
- **纯 AST 突变（parse → 改树 → format.Node）**：实测 go/printer 将合成节点位置与
  原文件注释位置混排，注释被移进表达式中间；伪造合法位置的工作量与收益不成比例。
- **golang.org/x/tools/go/ast/astutil**：仅覆盖 import 编辑，语句追加仍需自研，
  且为单点功能引入 x/tools 依赖；当需要任意 AST 编辑时作为首选积木重审。
- **github.com/dave/dst**：可往返编辑的正解，概念负担重；作为 go/ast 局限性的
  "标准答案"记录在文档中，本项目先体验问题再认识答案。

## Trade-offs

- 换来：注释零损伤、幂等可靠、产物天然 gofmt、失败即不写盘、零新增依赖。
- 付出：编辑能力限定为追加式；`BodyContainsStmt` 的子串匹配在极端情况下
  可能误判（例如用户手写了恰好含相同构造调用文本的语句）——被接受，
  因为误判的后果只是"少插一次"，不会损坏文件。
