# sshc P1 易用性增强实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-08 | Codex | 初版，规划 check、host tags、group defaults、ssh config import、batch summary 和 rerun failed |
| v0.2 | 2026-07-08 | Codex | 调整 `host set` 和 `group set` 为 `key=value...` 多字段设置语法，放入 P1.1/P1.2 实施 |

## 关联文档

- 易用性增强设计：`docs/2026-07-08-sshc-usability-enhancements-design.md`
- 后续能力设计：`docs/2026-07-04-sshc-next-features-design.md`
- 配置与凭证模型设计：`docs/2026-07-04-sshc-config-auth-design.md`
- host import 计划：`docs/plan/2026-07-05-sshc-host-import-plan.md`
- batch-run 计划：`docs/plan/2026-07-04-sshc-batch-run-plan.md`

## 当前基础

当前代码已具备：

- `host/auth/cfg` 管理命令。
- `run/login/batch-run/upload/download/log/serve` 主链路命令。
- `batch-run --hosts/--hosts-file/--group`、`--parallel`、`--fail-fast`。
- `host import` 的 `ips/plain/csv`、dry-run、skip-existing、overwrite。
- `run log` 的 `task_id`、大输出外置、`log --id`、`--lines`。
- `known_hosts` 默认校验和 `host trust`。
- jump host、command_proxy 和 Web terminal。

本计划只做 P1 易用性增强，不重复实现已完成基础能力。

## 范围

本轮实现 5 类能力：

1. `check`：主机连接健康检查。
2. host tags：host 信息新增 `tags`，支持添加、导入、列表过滤和匹配。
3. group defaults：分组级默认配置，减少大量主机重复字段。
4. `.ssh/config import`：在现有 `host import` 上新增 `--from-ssh-config`。
5. batch-run 汇总和失败重跑：新增 batch summary log 与 `--rerun-failed`。

## 非目标

- 不做完整 sync。`.ssh/config` 初版只 import，不跟踪删除、覆盖和双向同步。
- 不做完整 TUI 主程序。
- 不做 Ansible 风格 playbook、role、inventory。
- 不做多级 jump 或 OpenSSH `ProxyCommand` 转换。
- 不做 batch 自动 retry 策略，只做显式 `--rerun-failed`。
- 不改变 `sshc run` 的单主机语义。

## 设计确认

### host tags

host 新增字段：

```json
{
  "name": "devhost",
  "ip": "10.0.0.8",
  "tags": ["app", "testing", "gpu"]
}
```

命令面：

```bash
sshc host add --ip 10.0.0.8 --name devhost --tags app,testing
sshc host set devhost tags=app,testing,gpu remark="app server"
sshc host unset devhost tags remark
sshc host list --tag testing
sshc host list --tag app,gpu
sshc list --tag testing
```

规则：

- `tags` 是字符串数组，保存时去空、trim、去重、稳定排序。
- tag 推荐允许字母、数字、`-`、`_`、`.`、`:`；初版不做强格式限制，只拒绝空 tag。
- `--tags` 用于写入，接收逗号分隔。
- `--tag` 用于过滤，接收逗号分隔。
- `--tag app,gpu` 初版按 AND 处理，即 host 必须同时包含两个 tag。
- `--match` 应匹配 tags，方便 `sshc run "gpu app" -- ...` 的唯一模糊匹配。
- list/table 输出增加 `tags` 列；tags 多时用 `,` 连接。

`host set` 需要在 tags 阶段一起重构为多字段 `key=value` 语法：

```bash
sshc host set devhost user=root port=22 group=testing tags=app,gpu
sshc host set devhost auth=dev-root jump=bastion
sshc host set lxc-app backend=command_proxy via=pve-host run_template="pct exec 101 -- sh -lc {{cmd}}"
```

`host unset` 改为一次删除多个字段：

```bash
sshc host unset devhost tags remark jump
sshc host unset lxc-app backend via run_template login_command
```

规则：

- `host set <host> <key=value>...` 一次可设置多个字段。
- 任意字段解析或校验失败时整体不保存，避免半成功。
- `auth` 是 `auth_ref` 的别名，`key` 是 `key_path` 的别名。
- `tags` 值使用逗号分隔并归一化。
- 因工具尚未正式发布，允许破坏旧的 `host set --field value` 语法，不保留兼容层。

### group defaults

配置新增：

```json
{
  "groups": {
    "testing": {
      "auth_ref": "dev-root",
      "jump": "bastion",
      "port": 22,
      "connect_timeout": "10s",
      "run_timeout": "60s",
      "remote_script_dir": "/tmp",
      "host_key_check": "known_hosts",
      "known_hosts_path": "~/.ssh/known_hosts"
    }
  }
}
```

合并优先级调整为：

```text
命令行显式参数 > host 内联字段 > group defaults > auth_ref > defaults > 内置默认值
```

解释：

- host 自己设置的字段优先。
- host 没有 `auth_ref` 时可以继承 group 的 `auth_ref`。
- group 里的 `auth_ref` 会继续解析 auth profile。
- group defaults 只影响 effective host，不直接改写 host 配置。

命令面：

```bash
sshc group list
sshc group show testing
sshc group set testing auth=dev-root jump=bastion port=22
sshc group set testing connect_timeout=10s run_timeout=60s remote_script_dir=/tmp
sshc group unset testing jump port
sshc group rm testing --yes
```

说明：

- `group` 是管理命令，设置 `Category: "Management"`。
- `group set <group> <key=value>...` 一次可设置多个字段。
- `group unset <group> <field>...` 一次可删除多个字段。
- 任意字段解析或校验失败时整体不保存。
- `auth` 是 `auth_ref` 的别名，`key` 是 `key_path` 的别名。
- `group list` 展示 group defaults，不等同于 `host list --group`。
- `host list --group testing` 仍展示主机。

### check

命令面：

```bash
sshc check devhost
sshc check --group testing
sshc check --tag app
sshc check --hosts devhost,dbhost
sshc check --all
sshc check --parallel 10 --group testing
sshc check --json --tag app
```

检查项：

- target 是否能解析为 effective host。
- `auth_ref` 是否存在。
- group defaults 是否可解析。
- key 文件是否存在。
- known_hosts 文件路径是否可解析。
- known_hosts 是否存在目标记录。
- TCP 连接是否可达。
- SSH handshake 是否成功。
- auth 是否成功。
- jump host 链路是否成功。
- command_proxy 的 `via` 是否可达，`run_template/login_command` 是否配置完整。

默认表格输出：

```text
Name       Group    Tags        Addr        TCP  SSH  Auth  HostKey  Latency  Error
devhost    testing  app,gpu     10.*.*.8    ok   ok   ok    ok       42ms     -
dbhost     testing  db          10.*.*.9    no   -    -     ok       -        timeout
```

边界：

- `check` 不执行用户命令。
- command_proxy 初版只检查 `via` host 和配置完整性，不进入逻辑目标执行探测。
- 大量主机检查支持 `--parallel`，默认 5。
- 任一 host 检查失败，整体退出码非 0。

### `.ssh/config` import

使用现有 `host import` 新增 source 选项：

```bash
sshc host import --from-ssh-config
sshc host import --from-ssh-config -f ~/.ssh/config --dry-run
sshc host import --from-ssh-config --group imported --tags imported,ssh-config --skip-existing --yes
sshc host import --from-ssh-config --overwrite --yes
```

参数：

| 参数 | 说明 |
| --- | --- |
| `--from-ssh-config` | 输入来源为 OpenSSH config |
| `-f/--file` | 指定 ssh config 文件；未指定时默认 `~/.ssh/config` |
| `--group` | 给导入 host 设置默认 group |
| `--tags` | 给导入 host 设置默认 tags |
| `--auth` | 强制导入 host 使用指定 auth profile |
| `--import-identity-file` | 即使设置了 `--auth`，仍导入 `IdentityFile` 到 `key_path` |
| `--dry-run` | 只预览，不保存 |
| `--skip-existing` | 跳过已存在 host |
| `--overwrite` | 覆盖更新已存在 host |
| `--yes` | 跳过确认 |

字段映射：

| SSH config | sshc host | 说明 |
| --- | --- | --- |
| `Host` | `name` | 跳过包含 `*`、`?` 的 pattern |
| `HostName` | `ip` | 可以是 IP 或 DNS |
| `User` | `user` | 设置 `--auth` 时默认不导入 |
| `Port` | `port` | 合法范围 `1..65535` |
| `IdentityFile` | `key_path` | 多个时初版取第一个并 warning |
| `ProxyJump` | `jump` | 仅支持单个 jump host |
| `ProxyCommand` | warning | 初版不转换 |
| `LocalForward`/`RemoteForward` | warning | 后续 tunnel 功能处理 |

边界：

- 初版只做 import，不做 sync。
- 跳过 `Host *`、`Host *.example.com` 等 pattern。
- `Include` 和 `Match` 初版不保证完整支持；parser 支持则读取，不能支持则 warning。
- `ProxyJump a,b` 多级跳板初版不支持，warning。

### batch-run summary 和 rerun failed

新增 batch summary log：

```text
{logs_path}/batch/{yyyyMMdd}.jsonl
```

每次 batch-run 记录：

```json
{
  "batch_id": "20260708-120102-a1b2",
  "started_at": "2026-07-08T12:01:02.123",
  "ended_at": "2026-07-08T12:01:08.456",
  "source": {
    "kind": "group",
    "value": "testing"
  },
  "command": "uptime",
  "script": "",
  "task_name": "",
  "hosts": ["devhost", "web-2"],
  "success_count": 1,
  "failed_count": 1,
  "skipped_count": 0,
  "results": [
    {
      "host": "devhost",
      "status": "success",
      "task_id": "20260708-120102-x1",
      "duration_ms": 1200
    },
    {
      "host": "web-2",
      "status": "error",
      "task_id": "20260708-120103-x2",
      "duration_ms": 800,
      "error": "exit status 1"
    }
  ]
}
```

命令面：

```bash
sshc batch-run --group testing --summary table -- uptime
sshc batch-run --rerun-failed 20260708-120102-a1b2
sshc batch-run --rerun-failed 20260708-120102-a1b2 --parallel 5
```

规则：

- 普通 batch-run 完成后输出 `Batch ID: ...`。
- `--summary table` 初版可作为默认行为；`--summary json` 后续再做。
- `--rerun-failed` 读取 batch summary，找到失败 host，复用原始 command/script/run options。
- `--rerun-failed` 可以覆盖 `--parallel`、`--fail-fast`。
- `--rerun-failed` 不允许同时设置 `--hosts/--hosts-file/--group` 或 inline command。
- rerun 会生成新的 batch_id，并在 summary 中记录 `rerun_of`。

## 阶段计划

### P1.1: host tags 数据模型与解析（已完成）

目标：

- `host set/unset` 重构为 `key=value...` / `field...` 多字段语法。
- host 支持 `tags []string`。
- add/set/import/list/match 能理解 tags。

范围：

```text
internal/core/store.go
internal/core/config_mask.go
internal/core/config_resolve.go
internal/core/host_import.go
internal/core/*_test.go
internal/command/add.go
internal/command/host.go
internal/command/list.go
internal/command/*_test.go
internal/server/api_hosts.go
internal/server/api_test.go
README.md
README.zh-CN.md
```

实现：

1. `core.Host` 新增 `Tags []string json:"tags,omitempty"`。
2. 新增 `NormalizeTags(value string) []string` 和 `HostHasTags(host, tags)`。
3. `host add/add` 支持 `--tags app,testing`。
4. `host set` 从 flag-style 改为 `host set <host> <key=value>...`。
5. `host set` 支持字段：
   - `name`
   - `ip`
   - `auth` / `auth_ref`
   - `user`
   - `key` / `key_path`
   - `group`
   - `remark`
   - `tags`
   - `port`
   - `jump`
   - `backend`
   - `via`
   - `run_template`
   - `login_command`
   - `connect_timeout`
   - `run_timeout`
   - `remote_script_dir`
   - `host_key_check`
   - `known_hosts_path`
6. `host set` 任意字段解析或校验失败时整体不保存。
7. `host unset` 改为 `host unset <host> <field>...`。
8. `host unset` 支持一次删除多个字段，例如 `tags remark jump`。
9. `host list/list` 支持 `--tag app,gpu`。
10. `filterHosts` 同时支持 group、tag、match。
11. `match` 范围包含 tags。
12. `host import` 支持 `tags` 字段和 `--tags` 默认值。
13. Web API host JSON 接收和返回 tags。
14. README 中补 tags 示例和新的 `host set` 示例。

测试：

```text
TestNormalizeTags
TestHostAddWithTags
TestHostSetKeyValueFields
TestHostSetRejectsInvalidFieldWithoutSaving
TestHostSetTags
TestHostUnsetTags
TestHostUnsetMultipleFields
TestHostListFiltersByTag
TestHostListTagFilterUsesAND
TestHostMatchIncludesTags
TestHostImportParsesTags
TestHostImportAppliesDefaultTags
TestHostAPICreateUpdateTags
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./internal/server
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host add --help | Out-String
.\tmp\sshc.exe host set --help | Out-String
.\tmp\sshc.exe host list --help | Out-String
git diff --check -- internal README.md README.zh-CN.md
```

实施结果：

- `core.Host` 已新增 `tags` 字段，配置加载/保存会统一 trim、去空、去重、排序。
- `add/host add/host import/Web API` 已支持 tags 写入和归一化。
- `host set` 已切换为 `key=value...` 多字段语法，`host unset` 已切换为 `field...` 多字段语法。
- `host list/list` 已支持 `--tag` AND 过滤，table 输出增加 `Tags` 列，模糊匹配包含 tags。
- 已更新 README/README.zh-CN 的 tags 和新 set/unset 示例。
- 验证通过：`go test ./...`、`go build -o tmp\sshc-p1.exe ./cmd/sshc`、`git diff --check -- internal README.md README.zh-CN.md`。

提交：

```text
feat(host): add host tags
```

### P1.2: group defaults 配置模型

目标：

- 配置支持 group defaults。
- effective host 解析支持 group 级默认值。
- 增加 group 管理命令。

范围：

```text
internal/core/store.go
internal/core/config_resolve.go
internal/core/config_doctor.go
internal/core/config_mask.go
internal/core/config_test.go
internal/core/resolve_test.go
internal/command/group.go
internal/command/group_test.go
internal/bootstrap/init.go
README.md
README.zh-CN.md
```

实现：

1. `core.Config` 新增 `Groups map[string]GroupDefaults json:"groups,omitempty"`。
2. 新增 `GroupDefaults` 结构，字段覆盖：
   - `auth_ref`
   - `user`
   - `key_path`
   - `port`
   - `jump`
   - `connect_timeout`
   - `run_timeout`
   - `remote_script_dir`
   - `host_key_check`
   - `known_hosts_path`
3. `EffectiveHost` 在 host 内联字段之后、auth profile 之前应用 group defaults。
4. `cfg doctor` 检查 group defaults：
   - group name 非空。
   - group `auth_ref` 存在。
   - port 合法。
   - host_key_check 合法。
   - jump host 存在。
5. 新增 `group` 命令：
   - `group list`
   - `group show NAME`
   - `group set NAME key=value...`
   - `group unset NAME field...`
   - `group rm NAME --yes`
6. `group set` 支持字段：
   - `auth` / `auth_ref`
   - `user`
   - `key` / `key_path`
   - `port`
   - `jump`
   - `connect_timeout`
   - `run_timeout`
   - `remote_script_dir`
   - `host_key_check`
   - `known_hosts_path`
7. `group set` 任意字段解析或校验失败时整体不保存。
8. `group unset` 支持一次删除多个字段。
9. README 补 group defaults 示例。

测试：

```text
TestResolveEffectiveHostUsesGroupDefaults
TestResolveEffectiveHostHostOverridesGroupDefaults
TestResolveEffectiveHostGroupAuthRef
TestResolveEffectiveHostHostAuthRefOverridesGroupAuthRef
TestCfgDoctorReportsInvalidGroupAuthRef
TestGroupSetAndShow
TestGroupSetMultipleFields
TestGroupSetRejectsInvalidFieldWithoutSaving
TestGroupUnset
TestGroupUnsetMultipleFields
TestGroupRemove
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe group --help | Out-String
git diff --check -- internal README.md README.zh-CN.md
```

提交：

```text
feat(config): add group defaults
```

### P1.3: check 主机健康检查

目标：

- 新增 `sshc check`。
- 支持按 target、hosts、group、tag、all 检查主机。
- 输出表格和 JSON。

范围：

```text
internal/core/check.go
internal/core/check_test.go
internal/command/check.go
internal/command/check_test.go
internal/bootstrap/init.go
README.md
README.zh-CN.md
```

实现：

1. 新增 `core.CheckHost(host, opts)`。
2. 新增 `CheckResult`：
   - name/group/tags/addr
   - tcp_status
   - ssh_status
   - auth_status
   - host_key_status
   - latency_ms
   - error
3. 支持 TCP dial 超时。
4. 支持 SSH handshake/auth 检查。
5. 支持 jump host。
6. 支持 command_proxy via 检查。
7. 新增 `check` 命令参数：
   - target 参数可选。
   - `--hosts`
   - `--group`
   - `--tag`
   - `--all`
   - `--parallel`
   - `--json`
   - `--timeout`
8. source 互斥规则：
   - target、`--hosts`、`--group`、`--tag`、`--all` 只能选择一种；`--group` 和 `--tag` 可考虑组合，但初版建议互斥，保持清晰。
9. 输出 table 使用 cliui show/table。
10. 任一 host 失败，返回非 0。

测试：

```text
TestCheckCommandRequiresSource
TestCheckCommandFromTarget
TestCheckCommandFromGroup
TestCheckCommandFromTag
TestCheckCommandFromAll
TestCheckCommandRejectsMultipleSources
TestCheckCommandJSON
TestCheckHostReportsMissingKey
TestCheckHostReportsKnownHostsMissing
TestCheckHostTCPFailure
TestCheckHostCommandProxyViaFailure
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe check --help | Out-String
git diff --check -- internal README.md README.zh-CN.md
```

提交：

```text
feat(check): add host health checks
```

### P1.4: host import --from-ssh-config

目标：

- 在现有 `host import` 上新增 `--from-ssh-config` source。
- 复用 import plan、dry-run、skip-existing、overwrite、yes。

范围：

```text
internal/core/ssh_config.go 或 internal/core/host_import_ssh_config.go
internal/core/ssh_config_test.go
internal/command/host.go
internal/command/host_test.go
README.md
README.zh-CN.md
```

实现：

1. 新增 `ParseSSHConfigHosts(reader, defaults, opts)`。
2. 支持 `Host/HostName/User/Port/IdentityFile/ProxyJump` 映射。
3. 跳过 pattern Host。
4. `ProxyCommand`、forward、Match 不支持时 warning。
5. `--from-ssh-config` 与 `--format` 互斥。
6. `--from-ssh-config` 与 `--from-clipboard` 互斥。
7. 未传 `-f` 时默认 `~/.ssh/config`。
8. 支持 `--group`、`--tags`、`--auth`、`--import-identity-file`。
9. 调用现有 `PlanHostImport` 和 `ApplyHostImport`。
10. dry-run 输出 warning、add/update/skip/conflict 统计。

测试：

```text
TestHostImportFromSSHConfig
TestHostImportFromSSHConfigDefaultPath
TestHostImportFromSSHConfigSkipsPatterns
TestHostImportFromSSHConfigMapsProxyJump
TestHostImportFromSSHConfigWarnsProxyCommand
TestHostImportFromSSHConfigAuthSkipsUserAndIdentityByDefault
TestHostImportFromSSHConfigImportIdentityFileWithAuth
TestHostImportFromSSHConfigAppliesGroupAndTags
TestHostImportFromSSHConfigDryRunDoesNotSave
TestHostImportFromSSHConfigOverwrite
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host import --help | Out-String
git diff --check -- internal README.md README.zh-CN.md
```

提交：

```text
feat(host): import hosts from ssh config
```

### P1.5: batch-run summary log

目标：

- 每次 batch-run 写 batch summary JSONL。
- 命令输出展示 batch_id。

范围：

```text
internal/core/batch_log.go
internal/core/batch_log_test.go
internal/command/batch_run.go
internal/command/batch_run_test.go
README.md
README.zh-CN.md
```

实现：

1. 新增 `BatchRunRecord`。
2. 新增 `NewBatchID(startedAt)`。
3. 新增 `AppendBatchRunLog(record)`。
4. 日志路径为 `{logs_path}/batch/{yyyyMMdd}.jsonl`。
5. `runBatch` 聚合每台 host 的 result/task_id/duration/status/error。
6. batch-run 完成后写 summary log。
7. 输出增加：

```text
Batch ID: 20260708-120102-a1b2
Summary: total=2 success=1 failed=1 skipped=0 elapsed=3.2s
```

8. `--summary table` 作为默认值保留参数位；`--summary json` 可先不做，传入时报 not supported。
9. command_proxy、raw IP、hosts-file、group 来源都要写 source 信息。

测试：

```text
TestAppendBatchRunLogWritesJSONL
TestBatchRunWritesBatchSummary
TestBatchRunSummaryIncludesTaskIDs
TestBatchRunSummaryIncludesSource
TestBatchRunPrintsBatchID
TestBatchRunSummaryRejectsUnsupportedMode
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe batch-run --help | Out-String
git diff --check -- internal README.md README.zh-CN.md
```

提交：

```text
feat(batch): write batch run summaries
```

### P1.6: batch-run --rerun-failed

目标：

- 通过 batch_id 精确重跑上次失败的 hosts。
- 复用原始 command/script/run options。

范围：

```text
internal/core/batch_log.go
internal/core/batch_log_test.go
internal/command/batch_run.go
internal/command/batch_run_test.go
README.md
README.zh-CN.md
```

实现：

1. 新增 `ReadBatchRunByID(batchID)`。
2. 新增 `FailedHosts(record)`。
3. batch summary 记录足够的 rerun 参数：
   - command
   - script path 或 script metadata
   - cwd
   - timeout/kill_after/env/env_file/sudo/sudo_user/remote_script_dir/keep_remote_script
   - source kind/value
4. `batch-run --rerun-failed BATCH_ID`：
   - 读取旧 batch。
   - 找出 failed hosts。
   - 如果无失败 host，输出 no failed hosts 并退出 0。
   - 重新解析这些 host。
   - 复用旧 run options。
   - 允许覆盖 `--parallel`、`--fail-fast`。
   - 生成新 batch summary，记录 `rerun_of`。
5. 互斥规则：
   - `--rerun-failed` 与 `--hosts/--hosts-file/--group` 互斥。
   - `--rerun-failed` 与 inline command、`--script`、`--task` 互斥。
6. 如果旧 batch 包含 raw IP，rerun 必须能从 summary 重建 raw host 所需 auth_ref/overrides；若缺少敏感密码明文则报错并提示使用保存 host 或 auth profile。

测试：

```text
TestReadBatchRunByID
TestBatchRunRerunFailed
TestBatchRunRerunFailedNoFailures
TestBatchRunRerunFailedRejectsExtraSource
TestBatchRunRerunFailedRejectsInlineCommand
TestBatchRunRerunFailedRecordsRerunOf
TestBatchRunRerunFailedMissingBatchID
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe batch-run --help | Out-String
git diff --check -- internal README.md README.zh-CN.md
```

提交：

```text
feat(batch): rerun failed batch hosts
```

### P1.7: 文档、计划状态与最终验收

目标：

- README/中文 README 完整覆盖本轮能力。
- 本计划更新阶段状态。
- 必要时更新 TODO，但不混入用户未确认的 TODO 改动。

范围：

```text
README.md
README.zh-CN.md
docs/2026-07-08-sshc-usability-enhancements-design.md
docs/plan/2026-07-08-sshc-usability-p1-plan.md
docs/TODO.md
```

实现：

1. README 增加：
   - host tags 示例。
   - group defaults 示例。
   - check 示例。
   - `host import --from-ssh-config` 示例。
   - batch summary/rerun 示例。
2. 中文 README 同步。
3. usability design 标记本轮能力已进入实施或已完成。
4. 本计划更新每阶段状态。
5. 如需更新 `docs/TODO.md`，先确认当前无关脏改来源，避免覆盖用户内容。

验证：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe --help | Out-String
.\tmp\sshc.exe check --help | Out-String
.\tmp\sshc.exe host import --help | Out-String
.\tmp\sshc.exe batch-run --help | Out-String
git diff --check -- .
```

提交：

```text
docs: document p1 usability features
```

## 完整验收

自动验证：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
npm --prefix web run build
go build -tags embed_web -o tmp\sshc.exe ./cmd/sshc
git diff --check -- .
```

CLI smoke：

```powershell
$env:SSHC_CONFIG_DIR = "$PWD\tmp\p1-smoke"
.\tmp\sshc.exe auth add dev-root -u root -p --remark "testing auth"
.\tmp\sshc.exe group set testing auth=dev-root port=22
.\tmp\sshc.exe host add --ip 10.0.0.8 --name devhost --group testing --tags app,gpu --auth dev-root
.\tmp\sshc.exe host set devhost remark="app server" tags=app,gpu,testing
.\tmp\sshc.exe list --tag app
.\tmp\sshc.exe cfg doctor
.\tmp\sshc.exe check --tag app --json
.\tmp\sshc.exe host import --from-ssh-config -f tmp\ssh_config --dry-run
.\tmp\sshc.exe batch-run --hosts devhost -- echo ok
.\tmp\sshc.exe batch-run --rerun-failed <batch-id>
```

如果没有可连接远端，`check` 和 `batch-run` 的执行型 smoke 可只验证预期失败输出，核心链路由 mock 测试覆盖。

## 风险与处理

| 风险 | 处理 |
| --- | --- |
| tags 与 group 语义混淆 | group 表示配置继承和组织，tags 表示多维筛选；README 明确差异 |
| tags 输出太长影响 list 可读性 | table 中 tags 用逗号连接，后续可截断；JSON 输出保留完整数组 |
| group defaults 合并顺序引入回归 | 增加 effective host 合并优先级测试，覆盖 host 覆盖 group、group auth_ref |
| group defaults 改动影响 existing hosts | 只在 host 设置 group 且字段为空时生效；host 内联字段优先 |
| check 连接大量主机过慢 | 支持 `--parallel` 和 `--timeout`，默认并发保守 |
| check 暴露敏感信息 | 输出只展示认证类型和错误摘要，不打印密码、密文、token |
| ssh config parser 行为和 OpenSSH 不完全一致 | 初版只支持常见字段；复杂 Include/Match/ProxyCommand warning，不强转 |
| ssh config import 覆盖用户配置 | 默认 dry-run 可预览，默认冲突拒绝，覆盖必须显式 `--overwrite` |
| batch summary 记录敏感 env | 写 summary 前对 env key/value 走现有敏感参数 mask 规则 |
| rerun failed 无法恢复 raw password | 不保存明文密码；raw IP rerun 建议依赖 auth profile 或 key，缺失时明确报错 |
| rerun failed 重跑了已修复 host | 只重跑 failed hosts，并生成新的 batch_id，不修改旧记录 |
| 现有 `docs/TODO.md` 脏改混入 | 每次提交只 stage 本阶段文件，提交前检查 `git status` |

## 提交边界

按阶段提交，避免大提交：

```text
feat(host): add host tags
feat(config): add group defaults
feat(check): add host health checks
feat(host): import hosts from ssh config
feat(batch): write batch run summaries
feat(batch): rerun failed batch hosts
docs: document p1 usability features
```

如某阶段实现量超过预期，可继续拆分为 core/command/docs 三个提交，但不跨阶段混合提交。

## 后续扩展

- `sshc task` 和 `run --task`。
- completion host/tag/group 补全。
- recent/pinned hosts。
- tunnel local forward 管理。
- `cfg doctor --security`。
- WebUI check/batch/log 页面增强。

这些不进入本轮 P1。
