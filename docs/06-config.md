# 06 — Config：`goforge.yaml` 与配置系统

> 手写说明书。先读本文，完成 Handwriting Task，再看 internal/config 参考实现。

## 1. Background

到目前为止 goforge 的每个行为都来自命令行参数。但真实工具很快就长出"配置"的需求：

- 每个项目都有自己的设置（跑在哪个端口、项目叫什么），每次敲参数是折磨；
- 团队需要把约定固化成文件、进版本库，而不是口口相传；
- CI 和本地需要不同的值——环境变量是 twelve-factor app 的标准答案。

这一阶段引入 `goforge.yaml`：

```yaml
project:
  name: user-service

server:
  port: 8080
```

## 2. Problem

没有配置系统会发生什么：

- `goforge dev`（下一阶段）只能硬编码 8080，用户项目的端口冲突无解；
- 配置散落在各命令的 flag 里，团队约定无法版本化；
- "工具也有配置"这件事被回避，最后长出七八个互不通用的 flag。

一个工具级的问题是：**CLI 为什么也需要配置系统？**
答案：CLI 是长生命周期工具的入口，配置让"每次输入"变成"一次声明"。
git 的 `.git/config`、npm 的 `package.json`、terraform 的 `*.tf` 都是同一件事。

## 3. Requirements

1. 新命令 `internal/config`，API：`Load(dir) (Config, error)`、`ApplyEnv(cfg, lookup)`。
2. 配置文件 `goforge.yaml` 放在**目标项目根目录**（不是 goforge 自己的目录）：

   ```yaml
   project:
     name: user-service
   server:
     port: 8080
   ```

3. **优先级契约**：内置默认值 < goforge.yaml < 环境变量（`GOFORGE_PORT`）< 命令行 flag。
4. 文件缺失不是错误（返回默认值）；文件存在但坏了（语法错、未知字段、端口越界）是错误。
5. 脚手架生成的项目自带一份合法的 goforge.yaml。

## 4. Scope

不做：全局配置（`~/.goforge.yaml`）、配置写入/修改命令、多文件合并、
JSON/TOML 等其他格式、配置项的热重载、`dev` 消费端口（下一阶段的活）。

**坦白说明本阶段的定位**：config 的最大消费者是下一阶段的 `goforge dev`。
本阶段它只被 `generate` 轻量使用（坏配置 → 警告）。这是刻意的：
先把配置的"加载/校验/优先级"地基打好并测透，dev 阶段才能保持单薄。

## 5. Design

三个决定：

1. **分层解析，每层一个函数**：
   `defaults() → Load()(读文件合并) → applyDefaults() → Validate() → ApplyEnv()`。
   优先级不是一堆 if，而是"后层覆盖前层"的管道，每一层都能单独测试。
2. **缺失 ≠ 损坏**：文件不存在返回默认值（配置是可选的）；
   文件存在但解析失败返回错误（用户以为生效的配置悄悄失效，比失败更危险）。
3. **KnownFields(true)**：yaml 解码器拒绝未知字段。没有它，
   `prot: 9090` 这样的 typo 会被静默忽略——服务器照旧跑 8080，用户对着配置文件怀疑人生。

结构映射：

```go
type Config struct {
	Project Project `yaml:"project"`
	Server  Server  `yaml:"server"`
}
```

嵌套 section 对应嵌套 struct，yaml tag 声明映射——这是所有配置库的通用形态。

## 6. Core Concepts

- **Go 没有内置 YAML**：`encoding/json` 有、`encoding/yaml` 不存在。YAML 是事实标准
  （k8s、CI 全在用），所以引入 `gopkg.in/yaml.v3`——本仓库**第一个第三方依赖**，
  理由必须写进 ADR（这就是"如果第三方依赖确实有价值，可以使用，但必须说明"）。
- **yaml tag 与 struct 映射**：`yaml:"port"`；小写无 tag 时 yaml.v3 默认按小写字段名匹配。
- **yaml.Decoder + KnownFields(true)**：严格模式；`yaml.Unmarshal` 没有这个开关，
  所以用 Decoder + `bytes.NewReader`。
- **函数注入代替全局**：`ApplyEnv(cfg, lookup)` 接收 `func(string) string`——
  生产传 `os.Getenv`，测试传 map。比在测试里改真实环境变量干净得多
  （`t.Setenv` 也有，两者对照学习）。
- **t.Setenv**：测试内安全设置/还原环境变量的标准库方法（自动在测试后恢复）。
- **零值语义**：`Server.Port == 0` 表示"未设置"——因为 0 不是合法端口，
  零值天然可用作哨兵。这是 Go 里"让零值有意义"的设计技巧。

## 7. Design Decisions

1. **配置文件放在被管理的项目里，而不是 goforge 的安装目录**：
   这些值描述的是"这个项目"，随项目走、随版本库走。全局配置留到真的出现全局需求。
2. **配置是可选的**：goforge 的命令在没有配置文件时必须照常工作。
   工具的配置应该像 `Makefile`——有则精调，无则默认——而不是像 `go.mod`——没有就跑不了。
3. **坏配置在 generate 时警告、在 dev 时致命**：警告和致命的区别来自
   "该命令多依赖配置"。这个梯度本身就是设计，写进了 cli 的注释。
4. **端口校验在 Load 时做**（fail fast），而不是等 dev 启动时才炸：
   配置错误是"现在"的错误，不是"将来某次运行"的错误。
5. **错误信息带文件名**：`parse goforge.yaml: ...`——用户可能同时打开三个终端，
   别让他猜是哪个文件坏了。

## 8. Alternatives

- **encoding/json + goforge.json**：零依赖。放弃原因：人类手写的配置用 YAML 可读性更好、
  不支持注释的 JSON 对配置文件是硬伤（JSON5 又不是标准库）。
- **BurntSushi/toml**：TOML 同样优秀（Cargo 在用）。放弃原因：Go 生态的配置惯例偏 YAML；
  两者无本质差异，选一个并写明理由即可。
- **viper（spf13）**：合并多来源、热重载、远程配置。放弃原因：本项目只需要
  "单文件 + env + flag"三层，viper 的抽象半径远大于需求，且会掩盖优先级管道这个学习对象。
- ** flag 全部兜底（不做配置文件）**：不可版本化、不可共享；被需求否决。

## 9. Implementation Plan

1. `go get gopkg.in/yaml.v3`——第一个第三方依赖，体验 go.mod/go.sum 的变化。
2. `internal/config/config.go`：类型 + `defaults()` + `Load`（三态：缺失/读取失败/解析）。
3. `applyDefaults` + `Validate` + `ApplyEnv`。
4. 测试九连：缺失/完整/部分/坏 YAML/未知字段/端口越界/env 覆盖/env 垃圾/t.Setenv 真环境。
5. `templates/project/goforge.yaml.tmpl` + fileSpec 注册，脚手架带上配置文件。
6. cli `runGenerate`：成功后 `config.Load(".")`，出错则向 stderr 打 warning（不改变 exit code）。

## Handwriting Task

不看参考实现，完成：

1. `Load(dir)` 的三态处理：缺失→默认值；读取失败→包装错误；存在→解码合并。
2. KnownFields 严格模式：写一个 `prot:` typo 的测试，断言报错且错误信息含字段名。
3. `ApplyEnv`：函数注入版 + `t.Setenv` 真环境版各一个测试。
4. 优先级管道单测：文件里 port=9090、env GOFORGE_PORT=7777 → Load+ApplyEnv 后是 7777。
5. 脚手架生成 goforge.yaml，其内容能被自己的 `Load` 读回（round-trip 测试）。

**Expected Behavior**：

- `goforge new demo` 后，`demo/goforge.yaml` 的 `project.name` 是 `demo`。
- 在 demo 项目里手改 `server.port: 70000`，然后
  `goforge generate service user` → 仍然成功（exit 0），但 stderr 有
  "server.port 70000 out of range" 警告。
- 手加一行 `prot: 9090`，再跑 generate → stderr 警告
  `field prot not found`（不静默吞掉）。
- 删除 goforge.yaml，所有命令照常工作。
- `go test ./...` 全绿。

## 10. Thinking Questions

1. 为什么"文件缺失返回默认值"和"文件损坏返回错误"必须区别对待？
   如果都返回默认值，用户会以什么方式发现配置没生效？（代价大吗？）
2. 优先级里 flag 高于 env。反向设计（env 最高）会在什么场景下更合理？
3. `ApplyEnv` 注入 lookup 函数和测试里用 `t.Setenv`，两种方案各自的适用边界？
4. yaml.v3 的 KnownFields 拒绝未知字段——这对"配置文件向后兼容"（新版本加字段、旧二进制读）有什么影响？
5. 如果 `project.name` 与 go.mod 的 module 名不一致，goforge 该警告、报错还是不管？依据是什么？

## 11. Testing

```bash
go test ./...
cd $(mktemp -d) && goforge new demo && cd demo
cat goforge.yaml
goforge generate handler user          # 干净通过
echo "prot: 9090" >> goforge.yaml
goforge generate service order         # stderr 出现 unknown field 警告，exit 0
GOFORGE_PORT=7777 goforge version      # env 层在 dev 阶段才有可见效果，先记住契约
```

## 12. Failure Cases

- **静默忽略坏配置**：最危险的失败模式，KnownFields + Validate 就是为了堵它。
- **用 yaml.Unmarshal 而不是 Decoder**：拿不到 KnownFields 开关，typo 溜过去。
- **优先级管道顺序写反**（env 先加载、文件覆盖 env）：单元测试要固定"env 赢"这一条。
- **配置文件路径用绝对路径**：goforge.yaml 的定位是"项目根"，跟着 `dir` 参数走。
- **校验只查存在不查范围**：`port: -1` 通过解码、炸在运行时。

## 13. Reference Implementation

- `internal/config/config.go`：优先级管道的注释、Load 三态、零值即"未设置"的用法。
- `internal/config/config_test.go`：九个测试就是九条行为契约。
- `internal/cli/cli.go` 的 `runGenerate`：警告的措辞解释了"为什么警告而不是报错"。

## 14. Git Diff

相对 05-ast 新增/修改：

```text
go.mod / go.sum               第一个第三方依赖 gopkg.in/yaml.v3
internal/config/              新增：Load/Validate/ApplyEnv + 九个行为测试
templates/project/goforge.yaml.tmpl  新增
internal/project/project.go   scaffoldFiles 注册 goforge.yaml
internal/cli/cli.go           generate 后的配置警告；version 0.6.0
```

## 15. Interview Questions

见 docs/interview.md 的 Architecture 部分（配置层级）。先自测：
CLI 为什么需要配置？缺失和损坏为什么不同对待？flag/env/file 的优先级怎么实现和测试？
