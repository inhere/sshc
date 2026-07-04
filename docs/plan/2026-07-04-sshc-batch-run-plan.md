# sshc batch-run/brun 批量执行实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-04 | Codex | 初版，基于后续能力设计拆分 batch-run/brun 的实现阶段、提交边界和验收项 |
| v0.2 | 2026-07-04 | Codex | 补充 `--hosts-file` 批量临时 IP/hostname 场景，通过共享 auth/defaults 执行初始化脚本，不要求先保存所有 host |

## 关联文档

- 后续能力设计：`docs/2026-07-04-sshc-next-features-design.md`
- 配置与凭证模型设计：`docs/2026-07-04-sshc-config-auth-design.md`
- 配置与凭证实施计划：`docs/plan/2026-07-04-sshc-config-auth-plan.md`

## 背景

当前 `sshc` 已经完成：

- `run` 单机远程命令执行。
- `run --script` 本地脚本上传执行。
- `run --cwd`、`--timeout`、`--kill-after`、`--env/-e`、`--efile`、`--sudo`、`--sudo-user`。
- `auth`、`host`、`cfg` 多级管理命令。
- effective host 解析：`host/auth_ref/defaults/内置默认值` 合并。
- 每台 host 的 JSONL run log。

下一步批量执行不应把 `run` 本身继续撑大。建议新增顶层执行命令：

```bash
sshc batch-run --hosts devhost,web-2 -- uptime
sshc batch-run --hosts-file hosts.txt --script ./deploy.sh
sshc batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh
sshc batch-run --group testing --parallel 5 --script ./deploy.sh
sshc brun --hosts devhost,web-2 -- uptime
```

`batch-run` 是执行型命令，初版保持在默认命令分组，不放到 `Management`。

## 目标

- 新增 `batch-run` 命令，别名 `brun`。
- 支持三类 host 来源：
  - `--hosts dev1,dev2`
  - `--hosts-file hosts.txt`
  - `--group testing`
- 支持 `--hosts-file ips.txt` 读取未保存的临时 IP/hostname，并通过共享 `--auth` 或命令行认证覆盖执行初始化脚本。
- 复用 `run` 的主要执行能力：
  - command after `--`
  - `--script`
  - `--cwd`
  - `--timeout`
  - `--kill-after`
  - `--env/-e`
  - `--env-file/--efile`
  - `--sudo`
  - `--sudo-user`
  - `--remote-script-dir`
  - `--keep-remote-script`
- 支持 `--parallel` 控制并发。
- 支持 `--fail-fast`。
- 输出按 host 分块，并在最后输出 summary。
- 每台 host 继续写入自己的 run log。
- 任意 host 失败时，整体退出码非 0。
- 临时 IP/hostname 只参与本次执行，不写入 `sshc.config.json`。
- 分阶段实现、验证和提交，避免一次大提交。

## 非目标

- 不实现完整 Ansible/playbook/inventory/role/template 能力。
- 不实现 batch 级持久化任务记录或中心化执行历史。
- 不实现 batch summary log 文件，初版只依赖每 host run log 和命令输出。
- 不把 `--hosts-file` 中的临时 IP/hostname 自动保存为 host 配置。
- 不实现 `--json` 输出，后续单独设计结构化输出。
- 不实现跨命令批量 scp/download/login。
- 不实现交互式 host 选择；host 来源必须由参数给定。

## 命令面

### 基础用法

```bash
sshc batch-run --hosts devhost,web-2 -- uptime
sshc brun --hosts devhost,web-2 -- uptime
```

### 文件来源

```bash
sshc batch-run --hosts-file hosts.txt -- hostname
sshc batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh
sshc batch-run --hosts-file ips.txt --auth dev-root -u root --port 22 --script ./init.sh
```

`hosts.txt` 格式：

```text
# comments are ignored
devhost
web-2
192.168.1.10
```

规则：

- 空行忽略。
- `#` 开头的整行注释忽略。
- 初版不支持行内注释，避免 host name 中包含 `#` 时产生歧义。
- 已保存的 host name/IP 优先走现有配置解析。
- 未保存但符合 IP/hostname 形态的 target，在存在共享认证信息时按临时 host 处理。
- 临时 host 使用 `--auth`、`-u/--user`、`--key`、`-p/--password`、`--port` 和 defaults 合并出 effective host。
- 临时 host 不写入配置文件，也不出现在 `host list`。

### 临时 IP 初始化

这个场景用于大量新机器初始化：不希望先把所有 IP 都 `host add` 到配置，只想复用同一套认证资料执行脚本。

```bash
sshc auth add dev-root -u root -p
sshc batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh --parallel 10
```

`ips.txt`：

```text
10.20.1.11
10.20.1.12
10.20.1.13
```

规则：

- `--auth dev-root` 引用 `auth_profiles[].name`，用于这些临时 IP。
- 如果同时设置 `-u/--user`、`--key`、`-p/--password`、`--port`，按现有 effective host 优先级覆盖 `--auth` 和 defaults。
- `-p/--password` 建议设计为 bool prompt，隐藏读取一次共享密码，不接受 `-p secret`，避免密码进入 shell history。
- 未保存 target 的显示名默认使用原始 IP/hostname；run log 文件名应复用现有安全文件名逻辑。
- host key 校验仍按 defaults/命令行覆盖生效；新机器未写入 `known_hosts` 时会失败，用户需要先接受/写入 known_hosts，或显式配置 `host_key_check=insecure`。

### 分组来源

```bash
sshc batch-run --group testing -- uptime
```

规则：

- 只从 `sshc.config.json` 和 `~/.ssh/config` 合并后的 host 列表中筛选。
- 分组匹配使用 `core.HostGroupName(host)`，空 group 视为 `default`。
- `--group default` 可匹配未显式设置 group 的 host。

### 脚本执行

```bash
sshc batch-run --group testing --script ./deploy.sh
sshc batch-run --hosts devhost,web-2 --script ./deploy.sh --cwd /opt/app
sshc batch-run --hosts-file hosts.txt --script ./deploy.sh --remote-script-dir /opt/app/tmp
```

### 并发与失败

```bash
sshc batch-run --group testing --parallel 5 -- uptime
sshc batch-run --group testing --parallel 5 --fail-fast -- uptime
```

建议默认：

```text
parallel = 3
fail-fast = false
```

说明：

- `--parallel` 小于 1 时返回错误。
- `--parallel` 大于 host 数量时按 host 数量执行。
- `--fail-fast=false` 时跑完所有 host，再汇总失败。
- `--fail-fast=true` 时发现失败后不再启动新的 host；已经运行中的 host 等待结束。

## 输出设计

默认输出按 host 分块：

```text
==> devhost (root@192.168.1.10:22)
... remote output ...

==> web-2 (root@192.168.1.11:22)
... remote output ...

Summary: total=2 success=2 failed=0 skipped=0 elapsed=3.2s
```

失败示例：

```text
==> web-2 (root@192.168.1.11:22)
sshc: error: exit status 1

Summary: total=2 success=1 failed=1 skipped=0 elapsed=3.2s
Failed hosts: web-2
```

并发输出策略：

- 初版不做实时交错输出，避免并发时输出混乱。
- 每个 host 的 stdout/stderr 合并输出先收集到内存。
- 每个 host 结束后按完成顺序打印一个完整 block。
- summary 按最终 host 总数汇总。

风险：

- 远端输出特别大时会占用内存。初版接受该限制；后续 `--stream` 或 `--json` 单独设计。

## 日志设计

每台 host 继续调用现有 run log：

```go
core.AppendRunLog(host, core.RunLogRecord{...})
```

记录字段建议：

- `target`: 用户输入的 batch source 项或 group 展开后的 host name。
- `command`: 实际执行命令。
- `status`: `success/error`。
- `script`、`remote_script`、`cwd`、`duration_ms` 沿用现有字段。

初版不新增 batch summary log。

## 代码落点

建议新增：

```text
internal/command/batch_run.go
internal/core/batch_run.go
```

可能需要调整：

```text
internal/bootstrap/init.go
internal/command/run.go
internal/command/commands_test.go
internal/core/run.go 或现有 run 相关文件
README.md
README.zh-CN.md
docs/TODO.md
```

### command 层职责

- 定义 `batch-run/brun` 命令。
- 解析 batch flags。
- 校验 host source 互斥。
- 调用 core 层解析 hosts。
- 复用 `buildRunOptions`。
- 调度并发执行。
- 输出分块结果和 summary。

### core 层职责

建议新增结构：

```go
type BatchHostSource struct {
    Hosts     []string
    HostsFile string
    Group     string
    Overrides HostOverrides
    AuthRef   string
    AllowRaw  bool
}

type BatchRunResult struct {
    Host       Host
    Target     string
    Output     string
    Error      string
    Status     string
    DurationMS int64
}

type BatchRunSummary struct {
    Total   int
    Success int
    Failed  int
    Skipped int
    Elapsed time.Duration
}
```

建议新增函数：

```go
ResolveBatchHosts(source BatchHostSource) ([]Host, error)
ReadHostsFile(path string) ([]string, error)
```

说明：

- `Overrides` 复用现有 `HostOverrides`，不要新增 batch 专属认证模型。
- `AuthRef` 用于构造临时 host 的 `auth_ref`；保存的 host 仍使用自身配置。
- `AllowRaw` 表示允许把未解析到保存配置的 target 当作临时 IP/hostname。初版建议由命令层根据 `--auth` 或认证覆盖参数自动置 true，不额外暴露 `--host-mode`。
- `ResolveBatchHosts` 内部建议先 `LoadConfigWithSSHConfig()` 一次，再基于同一份 config 解析所有 target，避免每个 target 重复读配置。

执行调度可放 command 层或 core 层。初版建议 command 层调度，因为它直接依赖 `runRemote` 测试 hook，更容易复用现有 command 测试。

## Host 解析规则

### `--hosts`

```bash
--hosts devhost,web-2,192.168.1.10
```

规则：

- 按 `,` 分割。
- trim 空白。
- 空项报错，例如 `devhost,,web-2`。
- 每个项优先使用现有配置解析，支持已有的精确和唯一部分匹配。
- 多个项解析到同一 host 时去重，保持首次出现顺序。
- 任一项匹配多项，整体参数解析失败，不执行任何 host。
- 任一项未找到：
  - 如果未开启 raw target，则整体参数解析失败。
  - 如果开启 raw target 且 target 是合法 IP/hostname，则构造临时 host。
  - 如果开启 raw target 但 target 包含不支持的格式，例如 `host:22`，返回错误，端口统一使用 `--port`。

### `--hosts-file`

规则：

- 读取文件失败则报错。
- 文件中的每一行按一个 target 处理。
- target 解析规则同 `--hosts`。
- 文件为空或全是注释时报错。
- `--hosts-file ips.txt --auth dev-root` 是临时 IP 批量初始化的主要入口。

### 临时 raw target

临时 raw target 指未保存到 `sshc.config.json` 或 `~/.ssh/config` 的 IP/hostname。

触发条件：

- `--hosts` 或 `--hosts-file` 中某个 target 无法解析为保存 host。
- 命令层检测到共享认证来源，设置 `AllowRaw=true`：
  - `--auth NAME`
  - `-u/--user USER`
  - `--key PATH`
  - `-p/--password`
  - `--port PORT` 只作为补充，不能单独证明认证完整；但可以和 defaults/auth profile 一起使用。

构造规则：

```go
raw := Host{
    Name:    target,
    IP:      target,
    AuthRef: source.AuthRef,
    User:    source.Overrides.User,
    Password: source.Overrides.Password,
    KeyPath: source.Overrides.KeyPath,
    Port:    source.Overrides.Port,
}
effective, _, err := config.EffectiveHost(raw, source.Overrides)
host := effective.ToHost()
```

校验规则：

- final effective host 必须有 `user`。
- final effective host 必须有 `password`、`password_enc` 或 `key_path`。
- `host:port` 初版不支持，避免 IPv6/Windows 路径/端口语义混淆；端口用 `--port`。
- 保存 host 和 raw host 去重时，使用最终连接地址 `ip:port` 优先，其次使用 host log name。
- raw host 不调用任何保存函数，不修改 `sshc.config.json`。

### `--group`

规则：

- 读取 `LoadStoreWithSSHConfig()`。
- 按 `HostGroupName(host)` 过滤。
- 结果按配置顺序执行。
- 结果为空时报错。

### 互斥规则

三类来源必须且只能设置一个：

```text
--hosts
--hosts-file
--group
```

## Run 参数复用

`batch-run` 应尽量复用 `run` 参数，避免用户学习两套执行模型。

建议内部复用：

```go
buildRunOptions(flags runFlagOptions)
applyHostRunDefaults(&runOptions, host)
```

如果现有 `runFlagOptions` 是 command 内部结构，P1 可以先迁移为可复用 helper：

```text
internal/command/run_options.go
```

但不要为复用做大重构。只抽取必要小函数。

## 阶段计划

### P1.1: Host 来源解析

目标：

- 新增 batch host source 解析。
- 暂不执行远端命令。

范围：

```text
internal/core/batch_run.go
internal/core/core_test.go
```

实现：

1. 新增 `BatchHostSource`。
2. 新增 `ReadHostsFile(path)`。
3. 新增 `ResolveBatchHosts(source)`。
4. 支持 `--hosts`、`--hosts-file`、`--group` 互斥校验。
5. 支持去重并保持顺序。
6. 支持 `--hosts-file`/`--hosts` 中未保存 IP/hostname 通过共享认证构造临时 host。
7. raw target 不写入配置文件。

测试：

```text
TestReadHostsFileIgnoresBlankAndCommentLines
TestResolveBatchHostsFromCommaList
TestResolveBatchHostsRejectsMultipleSources
TestResolveBatchHostsFromGroup
TestResolveBatchHostsDeduplicates
TestResolveBatchHostsRejectsMissingHost
TestResolveBatchHostsFileRawIPsWithAuthRef
TestResolveBatchHostsRawIPDoesNotPersist
TestResolveBatchHostsRejectsRawIPWithoutAuth
TestResolveBatchHostsUsesSavedHostBeforeRaw
TestResolveBatchHostsRawIPUsesPortOverride
TestResolveBatchHostsRejectsHostPortRawTarget
```

验证：

```powershell
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal/core
```

提交：

```text
feat(batch): resolve batch host sources
```

### P1.2: batch-run 命令骨架和串行执行

目标：

- 新增 `batch-run/brun`。
- 支持串行执行多个 host。
- 复用 `run` 的 command/script/options。

范围：

```text
internal/command/batch_run.go
internal/bootstrap/init.go
internal/command/commands_test.go
```

实现：

1. 新增 `NewBatchRunCmd()`。
2. 注册 alias `brun`。
3. 注册到 bootstrap app，保持默认分组。
4. 新增共享目标认证参数：
   - `--auth NAME`
   - `-u/--user USER`
   - `--key PATH`
   - `-p/--password`，隐藏读取一次共享密码。
   - `--port PORT`
5. 根据 `--auth` 或认证覆盖参数自动允许 raw target；不新增 `--host-mode`。
6. 复用 `buildRunOptions` 和 `runRemote`。
7. 串行执行每个 host。
8. 每个 host 继续写 `AppendRunLog`。
9. 输出 host block 和 summary。
10. 任一 host 失败返回 error，使整体退出码非 0。

测试：

```text
TestBatchRunUsesHosts
TestBatchRunAlias
TestBatchRunPassesRunOptions
TestBatchRunWritesLogsPerHost
TestBatchRunReturnsErrorWhenAnyHostFails
TestBatchRunHostsFileRawIPsUsesSharedAuth
TestBatchRunPasswordPromptReadsOnce
```

验证：

```powershell
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe batch-run --help | Out-String
git diff --check -- internal/command internal/bootstrap
```

提交：

```text
feat(batch): add batch-run command
```

### P1.3: 并发执行和 fail-fast

目标：

- 支持 `--parallel`。
- 支持 `--fail-fast`。

范围：

```text
internal/command/batch_run.go
internal/command/commands_test.go
```

实现：

1. 新增 `--parallel`，默认 3。
2. 新增 `--fail-fast`。
3. 使用 worker pool 或 semaphore 控制并发。
4. 结果通过 channel 回收。
5. 输出按完成顺序打印 block。
6. `fail-fast` 时停止启动新任务，但等待已启动任务结束。
7. summary 增加 `skipped`。

测试：

```text
TestBatchRunRejectsInvalidParallel
TestBatchRunRunsWithParallelLimit
TestBatchRunFailFastSkipsPendingHosts
TestBatchRunSummaryCountsSkippedHosts
```

验证：

```powershell
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal/command
```

提交：

```text
feat(batch): add parallel and fail-fast options
```

### P1.4: 文档和 help 收口

目标：

- README、中文 README、TODO 和 LongHelp 更新。

范围：

```text
README.md
README.zh-CN.md
docs/TODO.md
internal/command/batch_run.go
```

实现：

1. README 增加 batch-run 示例。
   - 保存 host 批量执行。
   - `--hosts-file ips.txt --auth dev-root --script ./init.sh` 临时 IP 初始化。
2. 中文 README 同步。
3. TODO 标记批量执行完成。
4. LongHelp 保留常用示例，不重复 option 描述。

验证：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe batch-run --help | Out-String
git diff --check -- README.md README.zh-CN.md docs internal/command
```

提交：

```text
docs: document batch-run usage
```

## 完整验收

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe --help | Out-String
.\tmp\sshc.exe batch-run --help | Out-String
.\tmp\sshc.exe brun --help | Out-String
git diff --check -- .
git status --short --branch
```

手工 smoke 建议：

```powershell
.\tmp\sshc.exe batch-run --hosts devhost -- echo ok
.\tmp\sshc.exe batch-run --group testing --parallel 2 -- hostname
.\tmp\sshc.exe batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh --parallel 5
```

如果本机没有可连接远端，至少通过 mock 测试覆盖执行路径。

## 风险与处理

| 风险 | 处理 |
| --- | --- |
| 并发输出交错导致难读 | 初版按 host 收集输出后分块打印 |
| 远端输出过大导致内存占用 | 初版接受限制，后续单独设计 `--stream` |
| host partial match 多个候选 | 整体失败，不执行任何 host |
| `--hosts-file` 中 raw IP 没有认证信息 | 明确报错，提示使用 `--auth`、`--key` 或 `-p/--password` |
| raw IP 新机器未在 known_hosts 中 | 默认失败并提示 host key 校验；需要用户先写入 known_hosts 或显式配置 `insecure` |
| raw target 被误认为保存 host | 保存 host 优先解析；LongHelp 说明如需强制 raw 模式后续单独设计 |
| raw target `host:port` 语义不清 | 初版拒绝 `host:port`，端口统一使用 `--port` |
| `--fail-fast` 中已启动任务仍在跑 | 等待已启动任务结束，只跳过未启动任务 |
| batch-run 参数与 run 参数漂移 | 复用 `buildRunOptions`，新增 run 参数时同步测试 batch-run |

## 后续扩展

- `batch-run --json`
- batch summary log
- `--stream` 实时输出
- `--targets-from` 支持 stdin
- 批量 scp/download
- 批量执行结果重试

这些不进入本轮 P1。
