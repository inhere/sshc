# sshc host import 批量主机导入实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-05 | Codex | 初版，补充已有 hosts 清单快速导入 sshc 的场景边界、命令设计和实施阶段 |

## 关联文档

- 后续能力设计：`docs/2026-07-04-sshc-next-features-design.md`
- 配置与凭证模型设计：`docs/2026-07-04-sshc-config-auth-design.md`
- cfg export/import 计划：`docs/plan/2026-07-05-sshc-config-export-import-plan.md`

## 背景

`sshc` 现在已经支持：

- `host add` 单个保存主机。
- `add --from-clipboard` 从剪贴板读取单个 host。
- `batch-run --hosts-file` 临时读取很多 IP/host 执行命令，但不保存。
- `cfg export/import` 计划用于跨机器迁移完整 sshc 配置。

实际使用中还有一个独立场景：用户手里已有不少 hosts 信息，可能来自 Excel、CSV、旧 CMDB、堡垒机导出、文本 IP 清单或临时运维记录，希望快速导入到 `sshc.config.json`，不想逐个执行：

```bash
sshc host add ...
```

这个场景应新增 `host import`，而不是扩展 `cfg import`。原因：

- `host import` 操作的是“散装 host 清单 -> sshc hosts”。
- `cfg import` 操作的是“另一个 sshc 导出包 -> 当前 sshc 完整配置”。
- 两者的输入格式、冲突处理、安全边界不同，混在一个命令里会让语义变复杂。

## 三种导入场景边界

### 场景 1: 临时执行，不保存 hosts

适合大量新机器初始化、临时巡检、一次性批量命令执行。

```bash
sshc batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh
sshc batch-run --hosts 10.0.0.8,10.0.0.9 --auth dev-root -- hostname
```

特点：

- 不修改 `sshc.config.json`。
- 不出现在 `host list`。
- 适合“只跑一次，不想沉淀到配置”的目标。
- 已由当前 `batch-run` 覆盖。

### 场景 2: 批量保存已有 hosts 清单

适合用户已有一批机器清单，希望导入为 sshc 可管理 host。

```bash
sshc host import -f ips.txt --auth dev-root --group testing
sshc host import -f hosts.csv --format csv --dry-run
sshc host import --from-clipboard --format csv --auth dev-root
```

特点：

- 写入 `sshc.config.json`。
- 后续可用 `host list/show/set/unset/rm` 管理。
- 后续可用 `run/login/scp/download/batch-run --group` 直接使用。
- 这是本计划要实现的能力。

### 场景 3: 从另一台 sshc 迁移完整配置

适合换机器、备份恢复、团队内迁移完整 sshc 配置。

```bash
sshc cfg export -o sshc-export.enc
sshc cfg import -f sshc-export.enc --key "sshc-v1:..."
```

特点：

- 迁移完整 config：`logs_path/defaults/auth_profiles/hosts`。
- 处理 `password_enc` 依赖本机 key 的问题。
- 导入时用目标机器本地 key 重新加密敏感字段。
- 不适合普通 CSV/IP 清单导入。

## 目标

- 新增 `sshc host import`。
- 支持从文件、stdin 和剪贴板导入 host。
- 支持纯 IP/hostname 列表。
- 支持 CSV with header。
- 支持命令行统一补默认字段，例如 `--auth`、`--group`、`--port`、`--jump`。
- 支持 dry-run 预览。
- 支持冲突策略：默认冲突拒绝、`--skip-existing`、`--overwrite`。
- 导入 password 字段时保存为 `password_enc`，不落明文。
- 导入过程不打印 password。
- 每个阶段独立验证、独立提交。

## 非目标

- 不实现完整 `cfg import` 的加密迁移能力；那是独立计划。
- 不导入 `~/.config/sshc/key`。
- 不连接远端验证 SSH 可用性；连接检查后续可由 `cfg doctor --connect` 设计。
- 不支持 Excel `.xlsx` 直接读取；用户应先导出 CSV。
- 不做 CMDB/API 拉取；初版只支持本地输入。
- 不实现任意 JSON path 或自定义字段映射语言。
- 不在导入时自动创建 auth profile；引用的 `--auth` 或行内 `auth` 必须已存在，或由 `cfg doctor` 报错。

## 命令面

### IP/hostname 列表导入

```bash
sshc host import -f ips.txt --auth dev-root --group testing
sshc host import -f ips.txt --auth dev-root --group testing --port 22
sshc host import -f - --auth dev-root --group testing
```

`ips.txt`：

```text
# comments are ignored
10.0.0.8
10.0.0.9
web.internal
```

规则：

- 每行一个 IP 或 hostname。
- 空行忽略。
- `#` 开头整行注释忽略。
- 初版不支持行内注释。
- `name` 默认等于 IP/hostname。
- 行内没有字段，因此认证和分组主要来自命令行默认值。

### CSV with header 导入

```bash
sshc host import -f hosts.csv --format csv --dry-run
sshc host import -f hosts.csv --format csv --overwrite
sshc host import --from-clipboard --format csv --auth dev-root
```

`hosts.csv`：

```csv
name,ip,auth,group,remark,port,jump
devhost,10.0.0.8,dev-root,testing,app server,22,bastion
dbhost,10.0.0.9,dev-root,testing,db server,22,bastion
```

允许内联认证：

```csv
name,ip,user,password,key_path,group,remark,port
devhost,10.0.0.8,root,secret,,testing,app server,22
keyhost,10.0.0.9,deploy,,~/.ssh/id_ed25519,testing,key login,22
```

字段白名单：

| 字段 | 说明 |
| --- | --- |
| `name` | host 名称；为空时使用 `ip` |
| `ip` | SSH 目标 IP 或 hostname |
| `auth` / `auth_ref` | 引用 auth profile |
| `user` | SSH 用户 |
| `password` | 明文输入字段，只在内存态使用，保存时加密 |
| `key` / `key_path` | SSH private key 路径 |
| `group` | host 分组 |
| `remark` | 备注 |
| `port` | SSH 端口 |
| `jump` | 默认 jump host |
| `connect_timeout` | host 级连接超时 |
| `run_timeout` | host 级 run 超时 |
| `remote_script_dir` | host 级远端脚本目录 |
| `host_key_check` | `known_hosts` 或 `insecure` |
| `known_hosts_path` | known hosts 文件路径 |

CSV 解析必须使用 Go 标准库 `encoding/csv`，不要手写 `strings.Split(",")`。

### 剪贴板导入

```bash
sshc host import --from-clipboard --format csv --auth dev-root
```

规则：

- 剪贴板内容按 `--format` 解析。
- 初版要求显式 `--format csv` 或 `--format list`。
- 不复用 `add --from-clipboard` 的单 host 解析规则，避免单 host 和批量导入语义混淆。

## 参数设计

```bash
sshc host import \
  -f hosts.csv \
  --format csv \
  --auth dev-root \
  --group testing \
  --port 22 \
  --jump bastion \
  --dry-run \
  --skip-existing
```

建议参数：

| 参数 | 说明 |
| --- | --- |
| `-f/--file` | 输入文件；`-` 表示 stdin |
| `--from-clipboard` | 从剪贴板读取输入 |
| `--format` | `list` 或 `csv`；未设置时按文件扩展名推断，无法推断时报错 |
| `--auth` | 默认 auth profile |
| `-u/--user` | 默认 SSH 用户 |
| `--key` | 默认 SSH key path |
| `--group` | 默认 group |
| `--remark` | 默认 remark，通常只适合 list 格式 |
| `--port` | 默认端口 |
| `--jump` | 默认 jump host |
| `--host-key-check` | 默认 host key 策略 |
| `--known-hosts-path` | 默认 known hosts 路径 |
| `--dry-run` | 只预览，不保存 |
| `--skip-existing` | 冲突时跳过已有 host |
| `--overwrite` | 冲突时覆盖已有 host |
| `--yes` | 跳过批量导入确认 |

互斥规则：

- `--file` 和 `--from-clipboard` 互斥。
- `--skip-existing` 和 `--overwrite` 互斥。
- 未设置 `--file` 且未设置 `--from-clipboard` 时默认从 stdin 读取，或直接报错。建议初版直接报错，避免命令卡住等待 stdin。

## 字段优先级

导入时按以下顺序合并 host 字段：

```text
CSV/行内字段 > host import 命令参数 > config defaults > 内置默认值
```

说明：

- CSV 行内 `auth` 覆盖命令行 `--auth`。
- CSV 行内 `group` 覆盖命令行 `--group`。
- list 格式没有行内字段，主要使用命令行默认值。
- 保存前仍使用现有 effective host 校验，确保最终有 user 和认证方式。

## 冲突策略

默认策略是冲突拒绝，不写入配置。

冲突定义：

- 同名 host 已存在。
- 同 IP host 已存在。
- 导入文件内部出现重复 name。
- 导入文件内部出现重复 IP。

命令：

```bash
sshc host import -f hosts.csv --dry-run
sshc host import -f hosts.csv --skip-existing
sshc host import -f hosts.csv --overwrite
```

行为：

| 策略 | 行为 |
| --- | --- |
| 默认 | 任一冲突则整体失败，不保存 |
| `--skip-existing` | 跳过已存在 host，保存未冲突 host |
| `--overwrite` | 更新已存在 host，追加未冲突 host |
| `--dry-run` | 只输出计划，不保存 |

输出示例：

```text
Parsed: total=120 valid=120 invalid=0
Plan: add=118 update=0 skip=0 conflict=2
conflict: name=devhost ip=10.0.0.8 already exists
use --overwrite to update or --skip-existing to ignore existing hosts
```

成功示例：

```text
Imported hosts: added=118 updated=0 skipped=2
```

## 安全规则

- CSV 中允许 `password` 字段，这是从现有清单迁移的现实需求。
- `password` 只进入内存态，保存时走现有 `SaveConfig` 加密为 `password_enc`。
- 输出、dry-run 和错误信息都不显示 password 值。
- `--dry-run` 只显示该行存在 password，例如 `auth=password` 或 `auth=key+password`。
- 不新增 `--allow-plain-passwords`，避免降低批量导入实用性；安全边界靠“不打印、不落明文”保证。
- 导入过程中不写 run log。

## 代码落点

建议新增：

```text
internal/core/host_import.go
internal/core/host_import_test.go
```

建议修改：

```text
internal/command/host.go
internal/command/host_test.go
README.md
README.zh-CN.md
docs/TODO.md
docs/2026-07-04-sshc-next-features-design.md
```

### core 层建议 API

```go
type HostImportFormat string

const (
    HostImportList HostImportFormat = "list"
    HostImportCSV  HostImportFormat = "csv"
)

type HostImportDefaults struct {
    AuthRef        string
    User           string
    KeyPath        string
    Group          string
    Remark         string
    Port           int
    Jump           string
    HostKeyCheck   string
    KnownHostsPath string
}

type HostImportOptions struct {
    Format    HostImportFormat
    Defaults  HostImportDefaults
    Overwrite bool
    SkipExisting bool
}

type HostImportPlan struct {
    Hosts     []Host
    Added     int
    Updated   int
    Skipped   int
    Conflicts []HostImportConflict
    Invalid   []HostImportError
}
```

建议函数：

```go
func ParseHostImport(reader io.Reader, format HostImportFormat, defaults HostImportDefaults) ([]Host, []HostImportError)
func ParseHostImportList(reader io.Reader, defaults HostImportDefaults) ([]Host, []HostImportError)
func ParseHostImportCSV(reader io.Reader, defaults HostImportDefaults) ([]Host, []HostImportError)
func PlanHostImport(config Config, hosts []Host, opts HostImportOptions) (HostImportPlan, error)
func ApplyHostImport(config *Config, plan HostImportPlan) error
```

说明：

- `ParseHostImport` 只负责解析输入和基础字段校验。
- `PlanHostImport` 负责冲突判断和 add/update/skip 计划。
- `ApplyHostImport` 只在 plan 无阻断错误时修改 config。
- 保存仍由命令层调用 `core.SaveConfig(config)`。

## 阶段计划

### P1: host import 解析器

目标：

- 支持 list 和 CSV with header 解析。
- 支持命令行默认字段覆盖。
- 暂不接命令。

范围：

```text
internal/core/host_import.go
internal/core/host_import_test.go
```

实现：

1. 新增 `HostImportFormat`。
2. 新增 list 解析。
3. 新增 CSV with header 解析。
4. 支持字段别名：`auth/auth_ref`、`key/key_path`。
5. 校验必需字段：最终 `ip` 非空。
6. 校验 port 范围。
7. 校验 `host_key_check`。
8. password 字段只进入内存态。

测试：

```text
TestParseHostImportList
TestParseHostImportListIgnoresBlankAndCommentLines
TestParseHostImportCSV
TestParseHostImportCSVSupportsAliases
TestParseHostImportCSVKeepsCommaInRemark
TestParseHostImportAppliesDefaults
TestParseHostImportRowOverridesDefaults
TestParseHostImportRejectsUnknownField
TestParseHostImportRejectsInvalidPort
TestParseHostImportRejectsInvalidHostKeyCheck
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
feat(host): parse host import files
```

### P2: host import 计划与冲突策略

目标：

- 支持 dry-run 所需 plan。
- 支持默认冲突拒绝、skip-existing、overwrite。
- 暂不接命令。

范围：

```text
internal/core/host_import.go
internal/core/host_import_test.go
```

实现：

1. 新增 `PlanHostImport`。
2. 检查导入文件内部重复 name/IP。
3. 检查与当前 config 的重复 name/IP。
4. 默认冲突拒绝。
5. `SkipExisting` 时跳过冲突项。
6. `Overwrite` 时生成 update 计划。
7. 保存前通过 `config.EffectiveHost` 校验认证完整性。
8. `ApplyHostImport` 修改 config。

测试：

```text
TestPlanHostImportAddsNewHosts
TestPlanHostImportRejectsExistingName
TestPlanHostImportRejectsExistingIP
TestPlanHostImportRejectsDuplicateInputName
TestPlanHostImportSkipExisting
TestPlanHostImportOverwrite
TestPlanHostImportValidatesEffectiveHost
TestApplyHostImportDoesNotPartiallyApplyInvalidPlan
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
feat(host): plan imported host changes
```

### P3: host import 命令

目标：

- 新增 `sshc host import`。
- 支持 file/stdin/clipboard 输入。
- 支持 dry-run、skip-existing、overwrite、yes。

范围：

```text
internal/command/host.go
internal/command/host_test.go
```

命令：

```bash
sshc host import -f ips.txt --auth dev-root --group testing
sshc host import -f hosts.csv --format csv --dry-run
sshc host import --from-clipboard --format csv --auth dev-root
```

实现：

1. `host` 注册 `import` 子命令。
2. `-f/--file` 支持文件路径。
3. `-f -` 支持 stdin。
4. `--from-clipboard` 读取剪贴板。
5. `--format list|csv`，未设置时按扩展名推断。
6. 构造 `HostImportDefaults`。
7. 调用 core parser 和 planner。
8. dry-run 只打印统计和冲突，不保存。
9. 非 dry-run 且影响数量较多时默认确认，`--yes` 跳过确认。
10. 调用 `core.SaveConfig` 保存，确保 password 加密。

测试：

```text
TestHostImportListCommand
TestHostImportCSVCommand
TestHostImportDryRunDoesNotSave
TestHostImportRejectsConflicts
TestHostImportSkipExisting
TestHostImportOverwrite
TestHostImportFromClipboard
TestHostImportRequiresYesInNonInteractiveMode
TestHostImportEncryptsPassword
```

验证：

```powershell
go test ./internal/command
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host import --help | Out-String
git diff --check -- internal/command internal/core
```

提交：

```text
feat(host): import hosts from files
```

### P4: 文档和 TODO 收口

目标：

- README、中文 README、TODO、后续能力设计更新。

范围：

```text
README.md
README.zh-CN.md
docs/TODO.md
docs/2026-07-04-sshc-next-features-design.md
docs/plan/2026-07-05-sshc-host-import-plan.md
```

实现：

1. README 增加 `host import` list 和 CSV 示例。
2. 中文 README 同步。
3. TODO 增加并标记 host import 完成。
4. 后续能力设计标记 host import 完成。
5. 本计划更新各阶段状态。

验证：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host import --help | Out-String
git diff --check -- README.md README.zh-CN.md docs
```

提交：

```text
docs: document host import usage
```

## 完整验收

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host import --help | Out-String
git diff --check -- .
git status --short --branch
```

建议手工 smoke：

```powershell
$env:SSHC_CONFIG = "$PWD\tmp\host-import.json"
.\tmp\sshc.exe auth add dev-root -u root -p
@"
10.0.0.8
10.0.0.9
"@ | Set-Content tmp\ips.txt
.\tmp\sshc.exe host import -f tmp\ips.txt --auth dev-root --group testing --yes
.\tmp\sshc.exe host list --group testing
.\tmp\sshc.exe cfg show --raw
```

CSV smoke：

```powershell
@"
name,ip,auth,group,remark,port
devhost,10.0.0.8,dev-root,testing,app server,22
"@ | Set-Content tmp\hosts.csv
.\tmp\sshc.exe host import -f tmp\hosts.csv --format csv --dry-run
.\tmp\sshc.exe host import -f tmp\hosts.csv --format csv --overwrite --yes
```

## 风险与处理

| 风险 | 处理 |
| --- | --- |
| CSV password 被打印 | 输出、dry-run、错误信息都只显示认证类型，不显示 password 值 |
| CSV 解析因逗号备注出错 | 使用 `encoding/csv`，不手写 split |
| 默认导入覆盖已有 host | 默认冲突拒绝，必须显式 `--overwrite` |
| 大批量导入误操作 | 默认需要确认，脚本场景显式 `--yes` |
| 部分导入导致配置半成功 | 先 parse + plan + validate，全部通过后一次保存 |
| auth_ref 不存在 | effective host 校验失败或 doctor 报错；导入阶段应明确提示 |
| IP list 缺少认证信息 | 要求通过 `--auth`、`--user/--key` 或 defaults 补齐最终认证 |
| host:port 解析歧义 | 初版不支持 `host:port`，端口统一用 `port` 字段或 `--port` |

## 后续扩展

- JSON/YAML host 清单导入。
- key=value 文本导入。
- 字段映射参数，例如 `--map hostname=ip,desc=remark`。
- `--name-template`，例如 `--name-template node-{index}`。
- 导入后自动执行 `cfg doctor`。
- 导入后立即按 group 执行 `batch-run`。

这些不进入本轮实现。
