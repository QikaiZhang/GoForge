# ADR 006 — 项目结构：internal 布局与依赖方向

状态：已采纳（10-refactor 阶段定稿；01-09 各阶段增量形成）

## Context

goforge 经过九个功能阶段后包含八个内部包。需要一份最终结构决策记录，
回答：包怎么分、依赖往哪个方向流、cmd/ 与根包各放什么、
哪些"标准做法"被刻意没采用。

## Decision

1. 布局：

```text
cmd/goforge/main.go      进程胶水（os.Args → cli.Run → os.Exit）
templates.go             根包 goforge：//go:embed templates（embed 不许 ..，被语言约束钉在根上）
templates/               内容资产（项目模板 + 生成器模板）
internal/
  cli/         命令注册表 + 参数解析 + exit code 翻译 + 信号壳
  project/     项目形态（目录表 + 文件表 + 名字规则）
  template/    text/template 的单一入口（Render(fsys, name, data)）
  generator/   生成策略（kind/naming/覆盖策略/接线编排）
  astedit/     既有源码编辑（parser 定位 + 文本插入 + format 验证）
  config/      goforge.yaml 管道（defaults → file → env）
  process/     子进程运行器（退出码/信号/流三契约）
  git/         git CLI 组合
docs/                  阶段文档 + ADR + 总览
tests 随包放置          集成测试与被测包同包（cli/generator/project 的 *_test.go）
```

2. 依赖方向：cli → {project, generator, config, process, git, goforge(根)}；
   generator → {template, astedit}；project → template；其余互不依赖、
   无人依赖 cli。禁止任何包 import internal/cli。
3. 测试与实现同包（白盒 + 表驱动），不建独立 test 包目录；
   需要跨包端到端验证时写在被测包内的 integration test（testing.Short 跳过）。
4. 抽象准则固化：interface 只在存在第二种实现时引入
   （cli 的 command 用 struct+func 字段）；错误哨兵定义在其语义所属的包
   （project.ErrDirExists、generator.ErrFileExists）。

## Alternatives

- **按层分包（handlers/ services/ models/）**：Web 服务惯例，与 CLI 的
  "能力域"切分正交；GoForge 的包边界跟着能力走（generator≠project≠process），
  跟着调用层数走会让每个包都依赖所有其他包。
- **单包 goforge（全部塞根包）**：01~03 时尚可；04 起编译隔离、测试聚焦、
  依赖可见性全部失效；拒绝。
- **tests/ 顶层目录放集成测试**：树好看，但与被测代码的距离会滋长白盒断言失效；
  集成测试紧贴被测包放置。
- **cmd/ 下再分 cmd/goforge/internal**：多 module 入口时才有意义；
  单入口阶段属于目录仪式；拒绝。

## Trade-offs

- 换来：依赖方向单一流（可画成有向无环图）、每个包一句话能说清职责、
  测试与实现同处可白盒断言、新能力有明确的"归所"可问。
- 付出：包数量（8 个）对三命令工具而言偏多——但对学习目标是核心产品；
  根包 goforge 因 embed 的语言约束带一个"资源包"（有注释解释，不污染）；
  白盒同包测试需要纪律（不测私有细节，测行为）。
