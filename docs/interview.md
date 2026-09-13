# Interview — GoForge 覆盖的面试题库

> 每题按固定结构：**一句话 → 原理 → 本项目 → 深入追问**。
> 一句话是"面试时先说出口的答案"；追问是下一轮深挖的入口。
> 答不上追问时，回对应 branch 的 docs/NN-*.md 重读。

---

## CLI

### CLI 是什么？
**一句话**：一个接收 argv、操作三个标准流、用退出码报告结果的普通进程。
**原理**：操作系统不区分"命令"和"函数调用"——execve 只传参数数组和环境变量，
进程结束时内核只保留一个 8 位退出码。CLI 框架做的事只是包装这层协议。
**本项目**：`cli.Run(args, stdout, stderr) int` 就是这个本质的直接翻译；
`main.go` 只剩 `os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))`。
**追问**：为什么 args 不含程序名？——`os.Args[0]` 是调用路径，
对命令语义无用；含它会让所有下标偏移一位且测试困难。

### argv 是什么？
**一句话**：进程启动时操作系统交付的字符串数组，argv[0] 是程序自身路径。
**原理**：argv 的切分（空格、引号、glob）由 **shell** 完成，程序收到的已是切好的。
`--` 之后原样传递是 argv 层的约定（getopt 传统）。
**本项目**：07/08 阶段的 `--port=9090` 与 `--port 9090` 两种写法都手工解析；
`goforge test -- -run X` 按 argv 元素切分，绝不做字符串级 split。
**追问**：`goforge test -- -run "Test X"` 里带空格的参数会怎样？——
按元素透传时安全；一旦有人用 `strings.Split(s, " ")` 重组就碎了。

### stdout / stderr 的区别？
**一句话**：stdout 是产品输出（可被管道消费），stderr 是诊断输出（给人看的）。
**原理**：两者默认都连终端，所以平时看不出差别；一旦重定向/管道，混用的工具
就会污染下游数据流。约定 > 强制——go 的 log 默认走 stderr 就是这个约定。
**本项目**：usage 在主动求助时走 stdout、在未知命令时随错误走 stderr；
所有 `runXxx` 的错误一律 stderr。
**追问**：`goforge version | wc -l` 和 `goforge badcmd | wc -l` 各输出什么？——
前者计数正常输出；后者 stdout 为空（错误全在 stderr）。

### exit code 有哪些约定？
**一句话**：0 成功、非零失败；1 通常是运行失败、2 通常是用法错误。
**原理**：内核只保存 8 位（0-255）；POSIX 惯例 128+n 表示被信号 n 杀死
（Ctrl+C = 130）。退出码是脚本/CI 唯一能读的"返回值"，即跨进程 API。
**本项目**：`ExitOK/ExitError/ExitUsage` 三个常量；
`goforge test` 的失败透传自 `go test` 的 1；process 包把信号死亡翻译成 128+n。
**追问**：`os.Exit(-1)` 实际退出码是？——255（截断为 u8）。

### command / subcommand / flag 的边界？
**一句话**：command 决定"做什么"，flag 修饰"怎么做"，位置参数是"对象"。
**原理**：解析顺序敏感：`git commit -m x` 中 `-m` 属于子命令而非 git 本身——
**每个子命令有自己的 flag 作用域**。标准库 `flag` 的全局 FlagSet 模型
恰好在这里失灵，所以要 per-command FlagSet 或手工解析。
**本项目**：01~09 手工解析（switch + 逐参扫描），10 的 registry 把命令提升为数据。
**追问**：为什么 `goforge test` 拒绝 `-v` 但接受 `-- -v`？——
强制 `--` 让"goforge 的 flag"和"go test 的 flag"边界永远清晰，不猜。

---

## Filesystem

### MkdirAll 和 Mkdir 的区别？
**一句话**：MkdirAll 递归创建且"已存在不算错"；Mkdir 只建一层且存在即报错。
**原理**：MkdirAll 的幂等语义（idempotent）是脚手架/临时目录场景的刚需；
它内部先尝试 Stat 再决定，属"检查-行动"竞争窗口的实用主义容忍。
**本项目**：project.Create 的 scaffoldDirs、generator 的层目录全部 MkdirAll(0o755)。
**追问**：perm 0o755 会原样生效吗？——会被 umask 掩码（常见 022 → 实际 755）；
WriteFile 对已存在文件不修改 perm。

### WriteFile 的语义？
**一句话**：open(O_CREATE|O_TRUNC|O_WRONLY)+write+close 一步到位。
**原理**：TRUNC 意味着它天然是"覆盖"语义——不存在"追加"，也没有原子性。
需要原子替换用 temp 文件 + rename。
**本项目**：脚手架与生成器写新文件用 0o644；astedit 的 Save 因为"改已有文件"
先 format 后写，格式失败一个字节都不落盘（失败即不变）。
**追问**：写到一半断电，文件里是什么？——部分内容（TRUNC 后顺序写）。
关键文件要 write-to-temp + rename。

### filepath.Join 为什么不能用字符串拼接？
**一句话**：Join 处理分隔符平台差异、多余斜杠和相对段，拼接只是碰巧在 Linux 对。
**原理**：Windows 分隔符 `\`；`filepath.Clean` 规则（去掉 `./`、折叠 `..`）；
URL 路径要用 `path.Join`（永远 `/`），两者不可混。
**本项目**：所有落盘路径一律 Join；模板内部路径（embed FS）保持 `/`
因为 fs 接口永远用斜杠。
**追问**：`filepath.Join("a", "..", "b")` 的结果？——`b`（Clean 会解析 `..`），
这也解释了为什么用户输入要先过 ValidateName。

### 文件已存在怎么处理？
**一句话**：先 Stat 判断，再按语义选择拒绝（脚手架）、提示覆盖（生成器）、
静默覆盖（缓存）；Stat 的错误要区分"不存在"和"其他错误"。
**原理**：`errors.Is(err, fs.ErrNotExist)` 是唯一可靠的"不存在"判定；
TOCTOU（检查与使用之间状态可变）在单机 CLI 里容忍，多机/并发场景需要 O_EXCL。
**本项目**：三种选择都出现过——new 拒绝目录（ErrDirExists）、
generate 拒绝文件但给 --force、astedit 幂等跳过。
**追问**：Stat 和实际写入之间目录被别人建了怎么办？——MkdirAll 幂等兜底；
WriteFile 覆盖。真要防竞争用 `os.OpenFile(path, O_CREATE|O_EXCL, ...)`。

---

## Template

### 为什么用模板而不是字符串拼接？
**一句话**：拼接把"生成的内容"和"生成的逻辑"焊死在一起，模板把内容提升为数据文件。
**原理**：拼接的三宗罪——转义（内容里有反引号/引号即爆炸）、无条件逻辑、
无法独立校验。模板引擎提供 parse（语法可单独验证）与 execute（数据注入）的分离。
**本项目**：02 用 ReplaceAll 留下痛苦，03 用 `templates/` 目录 + ParseFS 根治；
`git diff 02-project-init 03-template` 是完整证据。
**追问**：html/template 和 text/template 差在哪？——前者做上下文相关 HTML 转义，
会破坏 Go 源码里的 `<`、`&`；生成代码必须用后者。

### 模板数据怎么传？
**一句话**：传预构造的结构体字段，模板里只做 `{{.Field}}` 引用，逻辑留在 Go 里。
**原理**：模板引擎的类型检查发生在 execute 期；把 Pascal 转换这类逻辑
写进 FuncMap 会让它逃出单元测试的射程。"数据准备"是纯 Go 函数，可表驱动测试。
**本项目**：`project.Data{Name}`、`generator.Data{Name, Pascal, Module}`；
generator 在渲染前调 `Pascal(req.Name)` 算好一切。
**追问**：数据缺字段时输出什么？——`<no value>`（静默！），所以渲染测试必须
断言输出内容而非"没报错"，并考虑 `Option("missingkey=error")`。

### go:embed 的约束？
**一句话**：编译期把文件打进二进制；pattern 相对源文件目录、不许 `..`、
普通 pattern 跳过 `.`/`_` 开头文件。
**原理**：embed 是编译器特性不是运行时查找——路径必须是编译期常量表达式，
所以永远不可能引用包目录之外的文件。
**本项目**：templates/ 因此必须放仓库根、由根包 goforge 嵌入；
`.gitignore.tmpl` 改名 `gitignore.tmpl`；`fs.Sub` 剥掉路径里的 `templates/` 前缀。
**追问**：embed.FS 里的路径为什么带 `templates/` 前缀？——
FS 根是源文件所在目录，pattern 原样保留目录结构；fs.Sub 补齐与 os.DirFS 的不对称。

---

## Code Generation

### 为什么需要代码生成？
**一句话**：凡是"有规律的重复"，就该由机器复述而不是人复制粘贴。
**原理**：生成的价值 = 约定的执行成本。生成器把分层约定（handler/service/repo
各自长什么样）固化成模板 + 测试，约定从"口口相传"变成"工具产出"。
**本项目**：`goforge generate handler user` 固化了三层样板与消费者侧接口风格；
生成项目的"必须能编译"是集成测试的硬契约。
**追问**：生成 vs 运行时反射/代码注入？——Go 没有运行时代码生成；
生成发生在构建前，类型安全、可调试（生成物是普通代码）。

### 如何避免覆盖用户代码？
**一句话**：默认拒绝覆盖 + 显式 --force + 幂等（重复执行无副作用）。
**原理**：生成器对用户工作区只有"写"的权限，权限越大越要显式授权。
幂等通过"写入前检测目标是否已表达同一意图"实现。
**本项目**：ErrFileExists 哨兵 + `--force`；
astedit 的接线用 BodyContainsStmt 检测后跳过——连续跑两次 `generate handler user`
磁盘不变。
**追问**：--force 覆盖会丢用户手改的代码，怎么补救？——备份文件、
diff 预览、或三方合并；各自引入状态管理成本，goforge 选择最简的"提示后覆盖"。

### 重复生成/多实体怎么设计？
**一句话**：同一个包会被多个实体共享——所有顶层标识符必须带实体前缀，
跨实体共享的 helper 只能存在一份（由脚手架提供）。
**原理**：Go 的包级命名空间强制这一点；单实体测试永远测不出撞名，
必须用"两个实体一起编译"的集成测试。
**本项目**：真实踩坑——`writeJSON`/`Repository`/`ErrNotFound` 在第二个实体
生成时全部撞名；修复后模板里是 `{{.Pascal}}Repository`、`Err{{.Pascal}}NotFound`，
接线变量是 `userRepo/userSvc/userHandler`。
**追问**：消费者侧接口（handler 定义 UserService）vs 生产者侧（service 定义）？
——前者按需最小化、让 service 无需为 handler 而导出接口，是 Go 惯例
（"accept interfaces"在消费端定义）。

---

## AST

### AST 是什么？
**一句话**：源码的树形表示——编译器的前端产物，也是一切可靠代码手术的解剖图。
**原理**：文本是线性的、格式是任意的；树是语义的、与排版无关。
"这个函数在不在""这个类型声明了吗"是树上的问题，不是字符串上的问题。
**本项目**：astedit 用 go/parser 得到 *ast.File，用 FuncDecl/HasType/HasImport
回答存在性问题，用 BodyContainsStmt 做幂等判断。
**追问**：AST 和 token 流的关系？——parser 消费 token（scanner 产出）建树；
go/token 包同时承载 token 种类和位置（FileSet/Pos）体系。

### parser / token / format 各做什么？
**一句话**：parser 是源码→AST；token 定义词元与位置坐标系；format 是 gofmt 的库形态。
**原理**：FileSet 给每个被解析文件分配位置区间，Pos 是区间内的整数偏移——
一棵跨文件树里的每个节点都能回答"我在哪"。format.Source 等价于 gofmt 命令：
规范化排版、排序 import 块，且**语法错误会作为 error 返回**（天然验证器）。
**本项目**：插入点的字节偏移 = `fset.Position(fn.Body.Rbrace).Offset`；
astedit 每次写盘都过 format.Source。
**追问**：`parser.ParseComments` 不开会怎样？——注释不进 AST，
BodyContainsStmt 仍工作，但注释在编辑时可能被破坏。

### AST 和字符串替换的区别？
**一句话**：字符串替换匹配文本，AST 匹配结构——注释、空格、换行都对结构查询不可见。
**原理**：文本手术的失败模式（锚点漂移、重复插入、插进注释）全部源于
"文本不是语义"。但结构手术有自己的代价：printer 靠位置信息排版，
合成节点没有合法位置。
**本项目**：05 的关键实验——纯 AST 追加后注释被 printer 塞进表达式中间；
最终策略是三明治：AST 定位 + 文本插入 + format.Source 验证。
**追问**：什么时候纯文本替换反而是对的？——目标文本受你控制时
（比如替换自己上一版生成的产物）。

### 为什么插入用文本拼接而不是 printer？
**一句话**：go/printer 按 token.Pos 的相对大小决定换行与注释归属，
合成节点的位置和真实注释的位置混在一起时输出会错乱。
**原理**：位置即语义——printer 的换行、缩进、注释挂靠全由 Pos 差值驱动；
这是"节点带位置"设计的天赋也是诅咒。dst 库（position-free AST）为绕开它而生。
**本项目**：astedit 包注释完整记录该决策；插入点偏移来自 AST（Rbrace 的 Position），
插入后 format.Source 兜底规范化。
**追问**：如果必须纯 AST（比如要"修改表达式"而非追加），工具链答案是什么？
——x/tools/astutil（import）、dst（任意编辑）、或精心伪造位置。

---

## Process

### exec.Command 的默认行为哪里不够？
**一句话**：三处——ExitError 混淆"子进程失败"和"工具失败"；
CommandContext 默认 SIGKILL 跳过优雅关闭；信号默认只到直接子进程。
**原理**：标准库提供原语不提供策略；策略（信号升级链、进程组、退出码语义）
是每个 CLI 工具自己的事。
**本项目**：process 包用 cmd.Cancel + WaitDelay + Setpgid 三件套修正，
`(code, error)` 双通道返回修正退出码语义。
**追问**：WaitDelay 设 0 会怎样？——无限等待（默认无升级），
忽略 SIGINT 的子进程会挂死父进程。

### 子进程的 stdin/stdout/stderr 怎么处理？
**一句话**：nil 继承或连 devnull；io.Writer 则 exec 建管道并开 goroutine 转发；
*os.File 则直接继承 fd。
**原理**：管道方案多一次内存拷贝且两个流并发——同一个非线程安全的 writer
被 stdout/stderr 同时写就是数据竞争；file 继承方案零拷贝但绕过了注入测试。
**本项目**：生产路径注入 os.Stdout/os.Stderr；测试里用带锁 writer
捕获输出（cli_test 的 syncWriter）。
**追问**：为什么 dev/test 要流式透传而不是收齐再打印？——实时性（长测试沉默
不可接受）与 Ctrl+C 时缓冲丢失。

### exit code 和 err != nil 的关系？
**一句话**：非零退出码是子进程的**结果**，err 是父进程**自身**失败的信号——
两者必须走不同的通道。
**原理**：`cmd.Run()` 把非零退出包装成 *ExitError，诱导调用方当成错误处理；
`errors.As(err, &ee)` 取回 ExitCode，信号死亡还要看 WaitStatus（ExitCode()==-1）。
**本项目**：process.Run 返回 `(code, nil)`；`goforge test` 因此薄到 40 行——
测试失败 = code 1 = 原样上报。
**追问**：`go test` 的 1 和 2 各是什么？——1 测试失败、2 构建失败；
透传让 CI 脚本能区分"修代码"和"修环境"。

### context 和信号怎么联动？
**一句话**：signal.NotifyContext 把 SIGINT/SIGTERM 翻译成 ctx 取消；
ctx 取消再通过 cmd.Cancel 翻译成发给子进程的信号——信号进、信号出，ctx 是中转站。
**原理**：这解决了"goroutine 收尾"与"外部中断"的统一问题：
任何监听 ctx 的代码（包括正在 Wait 的 exec）都会被同一个取消唤醒。
**本项目**：runDev/runTest/runGit 都是"信号壳 + ctx 内核"两层——
壳装 NotifyContext，核接收 ctx 参数（测试可直接取消，不必真的杀进程）。
**追问**：defer stop() 忘了会怎样？——信号通道泄漏，ctx 永远不会因信号取消，
且干扰后续注册的处理者。

### 优雅关闭的完整链条？
**一句话**：组 SIGINT → 子进程的处理者（NotifyContext）→ 有界 Shutdown →
超时则 SIGKILL 兜底。
**原理**：优雅的关键是"每一跳都有人接住信号且有超时"；SIGKILL 是唯一不可捕获的，
因此只能做最后手段而非默认。
**本项目**：process.Run 的 Setpgid + kill(-pid) + WaitDelay(10s)；
集成测试证明 server 打出 "shutting down"、端口释放、无孤儿。
**追问**：为什么必须进程组？——`go run` 和它编译出的 server 是两个进程，
不进同组则信号到不了真正监听端口的那个。

---

## Config

### CLI 为什么需要配置系统？
**一句话**：让"每次输入"变成"一次声明"，让团队约定可以进版本库。
**原理**：配置 = 延迟绑定的参数。优先级管道（defaults < file < env < flags）
是所有配置系统的共同骨架，env 层是 twelve-factor 的要求（环境变化不改文件）。
**本项目**：config.Load/applyDefaults/Validate/ApplyEnv 四段管道，
每段独立测试；dev 是"坏配置致命"、generate 是"坏配置警告"——消费深度分级。
**追问**：缺失配置返回默认值 vs 报错，怎么选？——看工具对配置的依赖深度：
可选配置（goforge.yaml）缺失即默认；必要配置（数据库 DSN）缺失必须 fail fast。

### YAML 的坑？
**一句话**：标准库没有 YAML（必须第三方）；字段名 typo 默认静默忽略；类型暗示规则多。
**原理**：yaml.v3 的 KnownFields(true) 打开未知字段拒绝；
Decoder API（而非 Unmarshal）才暴露这个开关。
**本项目**：`prot: 9090` 的 typo 测试断言报错含字段名；
goforge.yaml 的合法性由"自己的 Load 能读回自己"的 round-trip 保障。
**追问**：为什么不用 JSON/TOML？——JSON 无注释（人写配置的硬伤）、
TOML 同级可选项但 Go 生态惯例偏 YAML；理由写在 ADR 而不是凭感觉。

---

## Architecture

### 为什么 main.go 不能一直增长？
**一句话**：main 不可测（全局状态）、不可复用（单入口）、不可读（噪声）。
**原理**：main 所在的包无法被 import（package main 的函数外部不可见），
里面的任何逻辑都自动进入"测试盲区"。
**本项目**：main.go 永远是一行；所有增长发生在 internal/。
**追问**：internal/ 目录的意义？——编译器强制的封装：`goforge/internal/...`
只允许本 module import；别人拿你的包当库时拿不到内部实现，接口面由你决定。

### 为什么 command 和 generator 要分开？
**一句话**：CLI 是翻译层（argv→调用、error→exit code），业务是可复用的库函数；
混在一起则两头都做不成。
**原理**：分层测试理论——翻译层测"参数→exit code"（表驱动），
业务层测"输入→输出/副作用"（单元+集成）。混合层的测试只能端到端，慢且脆。
**本项目**：runGenerate 40 行且无业务；generator.Generate 被 05 的接线逻辑复用——
复用是分层正确性的最强证据。
**追问**：什么信号说明分错了层？——出现"CLI 层需要知道业务细节"
（比如 runNew 里算目录树）或"业务层需要知道 exit code"。

### 什么时候需要抽象，什么时候不需要？
**一句话**：第二次需要时才抽象；抽象的每一分通用性都要能用具体消费者指认。
**原理**：错误的抽象比重复更贵（Sandi Metz）——重复改 N 处，
错抽象要理解 M 层再改 N 处。interface 只在存在第二种实现时才值得。
**本项目**：正例——process 包在 dev 前抽出（test/git 紧随其后）；
反例（刻意不做）——project.ValidateName 与 generator.ValidateName 规则不同
就不合并、command 用 struct+func 不用 interface。
**追问**：`Run(args, stdout, stderr) int` 这个抽象在 01 就定了，
算"过早抽象"吗？——不算：它有立刻的消费者（表驱动测试）；
测试就是抽象的第一个用户。

### 重构和重写的边界？
**一句话**：不改行为的结构变更是重构（测试零修改）；改行为的是演进，必须独立提交。
**原理**：混合提交让"回滚结构"和"回滚行为"都做不到。
**本项目**：10 的 registry 重构要求 01~09 全部测试零断言修改；
新增的只有不变量测试（help 完整性、名字唯一性）。
**追问**：为什么"帮测试改断言让它过"危险？——测试是规格；
改规格来适配实现等于取消验证。
