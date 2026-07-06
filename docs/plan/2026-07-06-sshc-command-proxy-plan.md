# sshc command_proxy 实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-06 | Codex | 初版，基于 command_proxy 设计拆分配置模型、run/batch-run、login 和文档验收阶段 |

## 关联文档

- 设计文档：`docs/2026-07-06-sshc-command-proxy-design.md`
- 后续能力设计：`docs/2026-07-04-sshc-next-features-design.md`
- 配置与凭证模型设计：`docs/2026-07-04-sshc-config-auth-design.md`
- jump host 计划：`docs/plan/2026-07-04-sshc-jump-host-plan.md`

## 背景

当前 `sshc` 已支持标准 SSH jump host：

```text
local -> jump host -> target SSH
```

PVE/LXC/vhost 场景经常不是标准 SSH target。实际链路通常是：

```text
local -> pve-host SSH -> pct exec 101 -- <command>
```

目标 `lxc-app` 是逻辑 host，不一定有 sshd。真实 SSH 连接发生在 `via` host
上，例如 `pve-host`。因此该能力不能复用 jump host 语义，应新增独立 backend：

```json
{
  "name": "lxc-app",
  "backend": "command_proxy",
  "via": "pve-host",
  "run_template": "pct exec 101 -- sh -lc {{cmd}}",
  "login_command": "pct enter 101"
}
```

字段决策：

- `backend` 使用 `command_proxy`，不使用容易和 OpenSSH `ProxyCommand` 混淆的 `proxy_command`。
- `run_template` 使用 `string`，不使用 `[]string`。SSH remote exec 本身接收 command string，数组最终仍需渲染成字符串，收益不足。
- `login_command` 使用 `string`，表示完整交互命令，不是 prefix。
- `run_template` 使用 `{{cmd}}` 占位符，`{{cmd}}` 由 sshc 替换为 shell-quoted final command。

## 目标

- 配置层支持 command_proxy host。
- `cfg doctor` 能发现 command_proxy 配置错误。
- `host add/set/unset/show/list` 能维护和展示 command_proxy 字段。
- `sshc run` 支持通过 `via` host 和 `run_template` 执行逻辑 host 命令。
- `sshc batch-run` 可混用普通 SSH host 和 command_proxy host。
- `sshc login` 支持通过 `via` host 执行 `login_command`。
- run log 仍按逻辑 host 记录，并补充 backend/via/proxied_command 信息。
- README、中文 README、TODO 和设计文档状态同步。
- 每个阶段完成后单独提交。

## 非目标

- 不做 PVE/LXC 专用管理平台。
- 不实现 PVE API、LXC 生命周期管理或容器发现。
- 不支持 command_proxy 的 scp/upload/download。
- 不支持多层 command_proxy。
- 不支持 `{{raw_cmd}}` 或未 quote 的用户命令占位符。
- 不引入通用模板引擎，只做受控占位符替换。
- 不改变现有普通 SSH host 和 jump host 行为。

## 命令面

### host add

新增可选参数：

```bash
sshc host add --name lxc-app \
  --backend command_proxy \
  --via pve-host \
  --run-template "pct exec 101 -- sh -lc {{cmd}}" \
  --login-command "pct enter 101" \
  --group lxc \
  --remark "PVE CT 101"
```

顶层 `add` 可同步支持这些字段，保持高频入口一致：

```bash
sshc add --name lxc-app --backend command_proxy --via pve-host --run-template "pct exec 101 -- sh -lc {{cmd}}"
```

规则：

- `backend` 空值等同于 `ssh`。
- `backend=command_proxy` 时 `--ip` 可为空。
- `backend=command_proxy` 时 `--via` 必填。
- `run_template` 和 `login_command` 至少一个非空，避免创建完全不可用的逻辑 host。

### host set/unset

新增：

```bash
sshc host set lxc-app --backend command_proxy --via pve-host
sshc host set lxc-app --run-template "pct exec 101 -- sh -lc {{cmd}}"
sshc host set lxc-app --login-command "pct enter 101"
sshc host unset lxc-app --backend --via --run-template --login-command
```

规则：

- unset `backend` 时恢复默认 `ssh`。
- unset `backend` 不自动删除 `via/run_template/login_command`，避免误删用户配置；`cfg doctor` 可提示这些字段在 `ssh` backend 下被忽略。
- 如果设置 `backend=command_proxy` 后缺少必要字段，应返回明确错误。

### run

```bash
sshc run lxc-app -- hostname
sshc run lxc-app --cwd /opt/app -e APP_ENV=prod -- ./init.sh
```

执行：

```text
via host: pve-host
proxied command: pct exec 101 -- sh -lc '<final command>'
```

### batch-run

```bash
sshc batch-run --hosts web-1,lxc-app,docker-app -- uptime
```

每个 host 按自己的 backend 执行。普通 SSH host 走现有路径，command_proxy host 走代理路径。

### login

```bash
sshc login lxc-app
```

执行：

```text
via host: pve-host
interactive command: pct enter 101
```

### scp/upload/download

初版遇到 command_proxy host 直接报错：

```text
host lxc-app uses command_proxy backend; upload/download is not supported yet
```

## 数据模型

修改 `internal/core/store.go`：

```go
type Host struct {
    Backend      string `json:"backend,omitempty"`
    Via          string `json:"via,omitempty"`
    RunTemplate  string `json:"run_template,omitempty"`
    LoginCommand string `json:"login_command,omitempty"`
}
```

常量建议：

```go
const (
    HostBackendSSH          = "ssh"
    HostBackendCommandProxy = "command_proxy"
    CommandProxyCmdToken    = "{{cmd}}"
)
```

兼容规则：

- 旧配置缺少 `backend` 时按 `ssh` 处理。
- `backend=ssh` 时保留现有 direct/jump 行为。
- `backend=command_proxy` 时逻辑 host 不要求 `ip/port/user/key_path/password`。

## 模板规则

初版只支持：

```text
{{cmd}}
```

渲染规则：

1. 先复用现有 run 逻辑构造 final command，包含 env/cwd/sudo/timeout/script 处理。
2. 对 final command 使用现有 shell quote 规则转义。
3. 将 `{{cmd}}` 替换为 quoted final command。
4. 不解释其他模板语法。

示例：

```text
run_template: pct exec 101 -- sh -lc {{cmd}}
final command: cd /opt/app && APP_ENV=prod ./init.sh
proxied command: pct exec 101 -- sh -lc 'cd /opt/app && APP_ENV=prod ./init.sh'
```

校验：

- `run_template` 非空时必须包含 `{{cmd}}`。
- 缺少 `{{cmd}}` 的模板在保存或 `cfg doctor` 时报告错误。
- `run` 执行时再次校验，避免用户手改配置后绕过。

## 解析规则

新增或扩展 command_proxy 解析：

```go
type ResolvedCommandProxy struct {
    Target Host
    Via    Host
}
```

解析顺序：

1. 先按现有规则解析 target host。
2. 如果 target backend 为空或 `ssh`，走现有 direct/jump 逻辑。
3. 如果 target backend 为 `command_proxy`：
   - 使用 target.Via 解析 via host。
   - via host 必须存在。
   - via host 不能是 target 自己。
   - via host 不能是 `command_proxy`。
   - via host 可以有标准 jump 配置，允许链路为 `local -> bastion -> via -> command_proxy`。

说明：

- 允许 via host 使用标准 jump，因为它仍然是一个真实 SSH host。
- 不允许 command_proxy 嵌套 command_proxy，避免 quote、日志和错误定位复杂化。

## 日志

扩展 `RunLogRecord`：

```go
Backend        string `json:"backend,omitempty"`
Via            string `json:"via,omitempty"`
ProxiedCommand string `json:"proxied_command,omitempty"`
```

规则：

- 普通 SSH host 可不写 `backend` 或写 `ssh`，建议为空保持日志简洁。
- command_proxy run 写：
  - `backend=command_proxy`
  - `via=<via host log name>`
  - `proxied_command=<rendered command>`
- 日志文件仍使用逻辑 host：

```text
logs/lxc-app.log
```

## 阶段计划

### P1: 配置模型、校验和 host 管理

目标：

- 配置能保存 command_proxy host。
- `cfg doctor` 能报告常见配置错误。
- `host add/set/unset/show/list` 可维护相关字段。
- 不接入 run/login 执行。

范围：

```text
internal/core/store.go
internal/core/config_doctor.go
internal/core/config_resolve.go
internal/core/config_mask.go
internal/core/config_test.go
internal/core/jump_test.go
internal/command/add.go
internal/command/host.go
internal/command/command_test.go
internal/command/host_test.go
```

实现：

1. `Host` 增加 `Backend/Via/RunTemplate/LoginCommand`。
2. 增加 backend 常量和 normalize helper。
3. `CheckConfig` 增加 command_proxy 校验。
4. `host add` 支持 `--backend/--via/--run-template/--login-command`。
5. 顶层 `add` 同步支持上述参数。
6. `host set/unset` 支持上述字段。
7. `host show/list` 展示 backend/via 信息；list 保持简洁，只在非默认 backend 时展示。
8. host import plain 格式支持字段：

```text
backend=command_proxy
via=pve-host
run_template=pct exec 101 -- sh -lc {{cmd}}
login_command=pct enter 101
```

测试：

```text
TestHostAddCommandProxy
TestHostSetCommandProxyFields
TestHostUnsetCommandProxyFields
TestHostImportPlainCommandProxy
TestCfgDoctorReportsMissingVia
TestCfgDoctorReportsMissingCommandProxyAction
TestCfgDoctorReportsTemplateWithoutCmd
TestCfgDoctorRejectsNestedCommandProxy
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host add --help | Out-String
.\tmp\sshc.exe host set --help | Out-String
.\tmp\sshc.exe cfg doctor
git diff --check -- internal
```

提交：

```text
feat(host): add command proxy config
```

### P2: run 和 batch-run 支持 command_proxy

目标：

- `sshc run` 可以对 command_proxy host 执行命令。
- `batch-run` 可混用普通 SSH host 和 command_proxy host。
- 日志记录逻辑 host、via 和 proxied command。

范围：

```text
internal/core/command_proxy.go
internal/core/command_proxy_test.go
internal/core/ssh.go
internal/core/run_logs.go
internal/core/run_logs_test.go
internal/core/run_test.go
internal/command/run.go
internal/command/batch_run.go
internal/command/batch_run_test.go
internal/command/run_test.go
```

实现：

1. 新增 `RenderCommandProxyRun(template, finalCommand string) (string, error)`。
2. 新增 `ExecuteCommandProxy(host Host, command string, opts RunOptions)`。
3. `ExecuteRemote` 入口识别 `backend=command_proxy`。
4. command_proxy 执行时解析 via host，并使用现有 `newSSHClient(via)`。
5. 复用现有 `BuildRemoteCommandWithCWD`、sudo、timeout、script 逻辑生成 final command。
6. 渲染 `run_template` 后在 via host 上执行。
7. 扩展 run log 字段。
8. `run` 写日志时记录 backend/via/proxied_command。
9. `batch-run` 复用同一 `runRemote` 路径，避免重复实现。

关于 script：

- 初版允许 `--script`，但脚本上传发生在 via host。
- 代理执行时 final command 应是 via host 上的临时脚本路径。
- 这意味着 `pct exec` 目标内部无法直接读取 via host 上的临时脚本。
- 因此 P2 初版建议对 command_proxy 禁用 `--script`，返回明确错误：

```text
--script is not supported for command_proxy hosts yet
```

后续如要支持，应单独设计通过 stdin、heredoc 或 target 临时文件注入脚本。

测试：

```text
TestRenderCommandProxyRunQuotesCommand
TestRenderCommandProxyRunRejectsMissingToken
TestExecuteCommandProxyUsesViaHost
TestExecuteCommandProxyBuildsEnvAndCWD
TestRunCommandProxyWritesLogicalHostLog
TestBatchRunCommandProxyMixedHosts
TestRunCommandProxyRejectsScript
```

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal
```

建议 smoke 使用 fake 或本机可控 host；没有 PVE 环境时不做真实 LXC 验证。

提交：

```text
feat(run): execute command proxy hosts
```

### P3: login 支持 command_proxy

目标：

- `sshc login lxc-app` 可以通过 via host 执行 `login_command`。
- 保持现有 PTY、终端 resize、退出处理能力。

范围：

```text
internal/core/ssh.go
internal/core/command_proxy.go
internal/core/command_proxy_test.go
internal/core/pty_resize_unix.go
internal/core/pty_resize_windows.go
internal/command/login.go
internal/command/login_test.go
```

实现：

1. `LoginRemoteWithOptions` 入口识别 `backend=command_proxy`。
2. 新增 `LoginCommandProxy(host Host, opts LoginOptions) error`。
3. 解析 via host，连接 via。
4. 创建 session 和 PTY。
5. 在 PTY session 中执行 `login_command`。
6. 复用现有 stdin/stdout/stderr 连接和 resize 逻辑。
7. `login_command` 为空时报错。

测试：

```text
TestLoginCommandProxyUsesViaHost
TestLoginCommandProxyRejectsMissingLoginCommand
TestLoginCommandProxyRejectsNonTerminal
TestLoginCommandProxyKeepsLoginLogBoundary
```

说明：

- login 的集成测试仍以 mock/fake session 为主。
- 不要求真实 PVE `pct enter` 环境。

验证：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe login --help | Out-String
git diff --check -- internal
```

提交：

```text
feat(login): support command proxy hosts
```

### P4: 明确传输不支持和文档收口

目标：

- scp/upload/download 遇 command_proxy host 给出明确错误。
- README、中文 README、TODO、设计文档和 LongHelp 同步。
- 完成完整验收。

范围：

```text
internal/core/ssh.go
internal/core/transfer_test.go
internal/command/upload.go
internal/command/download.go
internal/command/upload_dl_test.go
README.md
README.zh-CN.md
docs/TODO.md
docs/2026-07-04-sshc-next-features-design.md
docs/2026-07-06-sshc-command-proxy-design.md
docs/plan/2026-07-06-sshc-command-proxy-plan.md
```

实现：

1. upload/download/scp 解析 host 后，如果 `backend=command_proxy`，返回明确错误。
2. README 增加 command_proxy 配置和 run/login 示例。
3. README.zh-CN 同步。
4. TODO 标记 command_proxy run/login 支持状态，保留传输后续项。
5. 后续能力设计标记 command_proxy 初版完成。
6. 本计划更新 P1-P4 状态和提交号。
7. LongHelp 增加精简示例，不重复 option 描述。

测试：

```text
TestUploadRejectsCommandProxyHost
TestDownloadRejectsCommandProxyHost
```

验证：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host add --help | Out-String
.\tmp\sshc.exe run --help | Out-String
.\tmp\sshc.exe login --help | Out-String
.\tmp\sshc.exe scp --help | Out-String
.\tmp\sshc.exe download --help | Out-String
git diff --check -- README.md README.zh-CN.md docs internal
```

提交：

```text
docs: document command proxy hosts
```

## 完整验收

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe cfg doctor
.\tmp\sshc.exe host add --help | Out-String
.\tmp\sshc.exe host set --help | Out-String
.\tmp\sshc.exe run --help | Out-String
.\tmp\sshc.exe login --help | Out-String
git diff --check -- .
git status --short --branch
```

建议手工 smoke：

```powershell
$env:SSHC_CONFIG = "$PWD\tmp\command-proxy-smoke.json"
Remove-Item $env:SSHC_CONFIG -ErrorAction SilentlyContinue

.\tmp\sshc.exe auth add local-key -u $env:USERNAME --key "$env:USERPROFILE\.ssh\id_rsa"
.\tmp\sshc.exe host add --name via-local --ip 127.0.0.1 --auth local-key --host-key-check insecure
.\tmp\sshc.exe host add --name proxy-local --backend command_proxy --via via-local --run-template "sh -lc {{cmd}}" --login-command "sh"
.\tmp\sshc.exe run proxy-local -- hostname
.\tmp\sshc.exe cfg doctor

Remove-Item Env:SSHC_CONFIG -ErrorAction SilentlyContinue
```

如果本机没有 SSH server，不执行真实 smoke，只保留单元测试和命令 help 验证。

## 风险与处理

| 风险 | 处理 |
| --- | --- |
| 用户误以为 command_proxy 是 SSH ProxyCommand | 文档和 help 明确它是命令代理，不代理 TCP |
| 复杂命令在 via shell 上被拆错 | `run_template` 必须使用 `{{cmd}}`，由 sshc 做 shell quote |
| `--script` 无法进入 LXC/container | 初版对 command_proxy 禁用 `--script`，后续单独设计 |
| upload/download 语义不清 | 初版明确不支持，避免错误上传到 via host |
| via host 也配置 command_proxy 导致嵌套复杂 | 初版禁止嵌套 |
| 日志文件落到 via host 名下导致审计困难 | 日志按逻辑 target 写，并记录 via/backend |
| `login_command` 需要 sudo 或特殊权限 | 用户显式写入模板，例如 `sudo pct enter 101` |
| 旧配置受影响 | backend 空值按 `ssh`，现有 direct/jump 逻辑不变 |

## 后续扩展

- command_proxy 脚本执行支持：通过 stdin/heredoc/target 临时文件注入脚本。
- upload/download 模板：例如 `pct push`、`pct pull`、`docker cp`。
- `login_template`：支持未来变量占位。
- `{{target}}`、`{{name}}` 等安全变量。
- `host import csv` 支持 command_proxy 字段。
- command_proxy 嵌套，需独立设计 quote 和日志链路。

## 待确认事项

1. P2 是否按建议禁用 command_proxy `--script`。建议禁用，避免脚本上传到 via 后 LXC 内不可见。
2. `host add` 是否允许 `backend=command_proxy` 时省略 `--ip`。建议允许。
3. `host list` 是否展示 backend/via。建议只在非默认 backend 时展示。
4. `run_template` 是否在 `host add/set` 时强制包含 `{{cmd}}`。建议强制。
5. `run_template` 是否允许为空。建议允许，但 `run` 时必须报错；这样可以存在只支持 login 的逻辑 host。
6. `login_command` 是否允许为空。建议允许，但 `login` 时必须报错；这样可以存在只支持 run 的逻辑 host。

## 提交计划

按阶段提交：

```text
feat(host): add command proxy config
feat(run): execute command proxy hosts
feat(login): support command proxy hosts
docs: document command proxy hosts
```

每个阶段提交前至少运行对应阶段验证命令；最终阶段完成后运行完整验收。
