# 10 — Refactor：回头看，重划边界

> 本阶段不引入任何用户可见的新能力。它存在的目的，是让你经历一次
> **"功能长完了，回头还债"** 的完整过程——这是工程最日常、也最少被教的一面。

## 1. Background

九个阶段的功能演进结束后，`internal/cli` 的状态：

- `Run` 里的 dispatch switch 已经膨胀到七路分支（help/version/裸调用 + 六个命令）；
- usage 文本在两处维护：`const usage` 和各 `runXxx` 里的碎片；
- `--version` 和 `goforge version` 是两段几乎相同的代码。

没有一处是"错的"——它们都是当时最简单的写法。但同一份知识（有哪些命令）
现在存在于三个地方，加一个命令要改三处，漏一处就是 help 与现实不符。

## 2. Problem

重复的税单是这样寄到的：

- 新命令开发者必须同时记得：写 runXxx、加 switch 分支、改 usage、
  在命令自己的错误里打 usage 碎片——四处同步，全靠自觉；
- usage 文本一旦和注册表不一致，用户看到的 help 就是谎言；
- 评审者的注意力被消耗在"你改 usage 了吗"这种机器能回答的问题上。

## 3. Requirements

1. 命令注册收敛到一个 `commands` 表（registry）：name + 一行描述 + run 函数；
2. help 文本**从 registry 生成**——新增命令不再可能"忘了写进 help"；
3. dispatch 变成一次表查找；
4. **行为零变化**：01~09 的全部测试不改一行断言继续通过（这是重构的定义，也是验收）；
5. 顺带偿还小债：`generate` 的 usage 碎片去重（3 处 → 1 处）。

## 4. Scope

不做：给 registry 加 interface（理由见 Design Decisions #2）、子命令的
参数 spec 化、帮助文本国际化、plugin 式动态注册。任何"顺手"的架构美化都不做——
**重构阶段的纪律就是只还确认过的债**。

## 5. Design

```go
type command struct {
	name  string
	short string
	run   func(args []string, stdout, stderr io.Writer) int
}

var commands = []command{
	{name: "new", short: "create a new project skeleton", run: runNew},
	{name: "generate", short: "...", run: runGenerate},
	// ... dev / test / git / version
}
```

- `Run` 的形状：特例先行（裸调用 → help；help flag；--version），然后表查找，
  默认 = 未知命令；
- `usageText()` 从表渲染，`printUsage(w)` 是唯一的打印点；
- 表的**切片顺序就是 help 顺序**——确定性优先于 map 的便利。

## 6. Core Concepts

- **"同一份知识的唯一归所"（Single Source of Truth）**：本阶段偿还的债
  本质上全是 DRY（Don't Repeat Yourself）。判断重复是否该消除的标准不是出现次数，
  而是**变化原因是否相同**——命令列表的变化永远同时影响 dispatch 和 help，
  所以必须合并。
- **用数据代替代码**：switch 分发命令 = 把"命令列表"编码在控制流里；
  registry = 把它提升为数据。数据可以被遍历（生成 help）、被测试（唯一性检查）、
  被排序——代码不行。
- **行为保持的重构**：重构的定义性约束。它的安全网是**已有的测试**
  （01 阶段就写好的表驱动测试此刻兑现全部价值）。
- **不变量测试**：`TestHelpListsEveryRegisteredCommand` 断言"每个注册命令
  必须出现在 help 里"——把"不许忘记"从纪律变成机器检查。

## 7. Design Decisions

1. **struct + func 字段，而不是 Command interface**。接口的价值在于"多种实现"；
   这里只有一种实现形态（普通函数），interface 是没有买家的保险。
   什么时候该升级成 interface？当出现第二种"命令"形态（比如：从配置文件生成的
   动态命令、带元数据的组命令）时。现在不付这笔复杂度。
2. **特例留在 switch，命令走 registry**。裸调用/help/--version 不是"命令"——
   它们没有参数、不进 usage 命令列表的语义（help 本身不列 help）。
   把特例伪装成数据是过度泛化；让数据保持纯粹。
3. **切片而不是 map**：map 遍历顺序随机，help 输出必须稳定。
   排序切片的顺序即文档顺序。
4. **版本号升到 1.0.0**：纯仪式，但仪式有信息量——重构完成 = 契约稳定 =
   可以被放心地复刻和依赖。

## 8. Alternatives

- **保留 switch，只把 usage 改成从 switch 注释生成**（注释驱动文档）：
  治标不治本，注释照样可能忘记更新；拒绝。
- **立即上 cobra**：registry 长到需要分组、全局 flag 树、shell completion 时
  这是正确答案；现在六个命令，registry 40 行更诚实。
- **同时重构 internal/project、generator 等包**：它们在各自阶段已经收敛
  （每个阶段的小重构都当场做了），没有累积的债。**没有债就不要制造重构**——
  这句反过来也成立：不要用"以后重构"当理由容忍眼前的坏味道。
- **把 runXxx 全部拆成独立文件**：cli.go 430 行尚在"一个屏幕能理解全貌"的范围内；
  文件拆分是品味题，不是原则题。

## 9. Implementation Plan

1. 定义 `command` 类型和 `commands` 表（从现有 switch 分支机械搬运）。
2. `usageText()`/`printUsage()`：从表渲染；对照旧 usage 逐字符核对格式。
3. 改写 `Run`：特例 switch + 表查找 + 未知命令分支（错误信息保持原样）。
4. `runVersion` 从 `--version` 和 `version` 两个调用点共享。
5. `printGenerateUsage` 去重三处碎片。
6. 跑全部测试——**一行断言都不许改**。若有测试需要改，先问：是行为真的变了，
   还是测试在断言实现细节？前者 = 重构失败，回滚。
7. 补两条不变量测试（help 完整性、名字唯一性）。

## Handwriting Task

不看参考实现，完成：

1. registry 结构 + 从旧 switch 机械搬运六个命令。
2. help 从表生成，输出格式与旧 usage 完全一致（可以 diff 验证）。
3. `Run` 重写后，01~09 阶段的所有测试**不改一行**通过。
4. 两条不变量测试。

**Expected Behavior**：

- `goforge help` 输出与重构前逐字符一致；
- `goforge --version` 与 `goforge version` 行为一致；
- `goforge frobnicate` 的 exit code 和错误信息与重构前一致；
- `go test ./...` 全绿，且 `git diff 09-git 10-refactor -- internal/cli/cli_test.go`
  只新增测试，不修改旧断言。

## 10. Thinking Questions

1. "行为零变化"的重构和"行为有变化的重构"（比如换错误信息），
   在 commit 策略上应该有什么区别？
2. registry 里的 run 函数签名是统一的 `(args, stdout, stderr) int`。
   这个统一性是谁保证的？如果某个命令需要 ctx（dev/test 需要），抽象会不会破？
   （提示：看 runDev 的壳核分离是怎么在统一签名内解决这个问题的。）
3. 什么样的重复**不该**消除？（提示：看 project.ValidateName 和
   generator.ValidateName——它们长得像，为什么保持分开？）
4. 如果第 11 个命令需要"互斥 flag 校验"这种通用机制，你会把它放哪？
   什么时候 registry 应该升级成 struct 带更多元数据？

## 11. Testing

```bash
go test ./...                          # 全绿，旧断言零修改
git diff 09-git 10-refactor --stat     # 重构的影响面一目了然
go build -o /tmp/gf ./cmd/goforge
diff <(/tmp/gf help) <(git show 09-git:internal/cli/cli.go | grep -A20 'const usage') >/dev/null 2>&1 || true
# 手工对照 help 输出
/tmp/gf frobnicate; echo $?            # 2，与重构前一致
```

## 12. Failure Cases

- **重构顺手"改良"了行为**（换措辞、调顺序）：重构 commit 里混入行为变化，
  两者都无法独立回滚。纪律：行为变化必须单独 commit。
- **help 格式对不齐**：`%-10s` 的宽度魔数和命令名长度——
  "generate"（8 字符）是当前最长的名字，第 11 个命令超过 10 字符时格式会挤——
  已知限制，注释里点名。
- **把特例也塞进 registry**（help 作为一个 command）：然后 usage 列表里
  出现 "help — show this help" 的自我引用问题。特例和数据各归其位。
- **测试改了断言才通过**：那不是重构，是重写。重写需要新的讨论，
  而不是藏在 "refactor" 的 commit message 里。

## 13. Reference Implementation

- `internal/cli/cli.go` 顶部：`command` 类型的注释完整记录了
  "为什么不用 interface"的推理——这类"为什么不"的推理最容易失传，值得写在代码里。
- `usageText()`：格式化宽度魔数旁的注释。
- `internal/cli/cli_test.go`：两条不变量测试是 registry 的"存在证明"。

## 14. Git Diff

相对 09-git 新增/修改：

```text
internal/cli/cli.go       commands 表 + usageText/printUsage/runVersion；
                          Run 改为特例 switch + 表查找；printGenerateUsage 去重；
                          version 1.0.0
internal/cli/cli_test.go  新增两条不变量测试（help 完整性、名字唯一）
docs/10-refactor.md       本文档
docs/adr/006-project-structure.md  最终结构决策
docs/architecture.md      全景架构文档
docs/handwriting-guide.md 复刻指南
docs/interview.md         面试题库
```

## 15. Interview Questions

见 docs/interview.md 的 Architecture 部分。先自测：
什么时候该重构？重构和重写的边界？"用数据代替代码"的适用条件？
