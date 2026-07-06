# sshc command_proxy 设计

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-06 | Codex | 初版，设计 PVE/LXC/vhost 等通过宿主机代理执行命令的 command_proxy backend |
| v0.2 | 2026-07-06 | Codex | 标记 command_proxy 初版已完成，明确传输和脚本注入仍属后续能力 |

## 背景

`sshc` 当前已经支持标准 SSH jump host：

```text
local -> jump host -> target SSH
```

这种模式要求 target 本身可以通过 SSH 连接。PVE/LXC/vhost 这类场景经常不是这样：

```text
local -> pve-host SSH -> pct exec 101 -- <command>
```

目标 `lxc-app` 可能没有 sshd，也可能不暴露 22 端口。真正能 SSH 连接的是
`pve-host`，目标命令需要通过 `pct exec`、`pct enter`、`docker exec`
或内部 vhost 管理命令代理执行。

早期设计里提过 `proxy_command`，但该名称容易和 OpenSSH `ProxyCommand`
混淆。OpenSSH `ProxyCommand` 代理的是 TCP stream，最终仍然连接 target SSH；
本设计要做的是“命令代理执行”，因此统一命名为 `command_proxy`。

## 名词

| 名称 | 含义 |
| --- | --- |
| `ssh` backend | 默认后端，直接 SSH target 或通过标准 jump SSH target |
| `command_proxy` backend | 通过 `via` 指向的真实 SSH host 执行代理命令 |
| `via` | 代理执行所在的真实 SSH host，例如 `pve-host` |
| `run_template` | `sshc run` 的命令模板，必须包含 `{{cmd}}` |
| `login_command` | `sshc login` 的完整交互命令，例如 `pct enter 101` |

## 目标

- 支持把 PVE LXC、Docker container、vhost 等注册为 sshc 逻辑 host。
- 复用现有 host/auth/defaults 解析模型，不新增 PVE 专用资源类型。
- `sshc run <logical-host>` 可以在 `via` host 上通过模板执行目标命令。
- `sshc login <logical-host>` 可以在 `via` host 上运行交互命令。
- 初版保持轻量，不实现 upload/download/scp 的 command_proxy 传输语义。
- 避免和标准 SSH jump host 混淆。

## 非目标

- 不做 PVE/LXC 专用管理平台。
- 不实现 LXC/container 生命周期管理，例如 create/start/stop/reboot。
- 不读取 PVE API，不依赖 PVE token。
- 不把 command_proxy 伪装成标准 SSH target。
- 初版不支持 command_proxy 的 upload/download/scp。
- 初版不支持多层 command_proxy，例如 `local -> pve -> docker -> app`。
- 初版不支持在 `run_template` 中执行未经过 quote 的用户命令，除非后续明确增加高风险开关。

## 总体思路

command_proxy host 是一个逻辑 host：

```text
sshc run lxc-app -- hostname
```

不会直接连接 `lxc-app`，而是：

```text
1. 解析 lxc-app 配置，发现 backend=command_proxy。
2. 解析 via=pve-host。
3. 使用现有 SSH 逻辑连接 pve-host。
4. 按 run_template 包装用户命令。
5. 在 pve-host 上执行包装后的命令。
6. 日志仍按 lxc-app 记录，而不是按 pve-host 记录。
```

推荐配置：

```json
{
  "name": "lxc-app",
  "backend": "command_proxy",
  "via": "pve-host",
  "run_template": "pct exec 101 -- sh -lc {{cmd}}",
  "login_command": "pct enter 101",
  "group": "lxc",
  "remark": "PVE CT 101"
}
```

## 与 jump host 的区别

标准 jump：

```text
local -> bastion SSH -> target SSH
```

command_proxy：

```text
local -> pve-host SSH -> run "pct exec 101 -- sh -lc <cmd>"
```

差异：

| 项 | jump | command_proxy |
| --- | --- | --- |
| target 是否需要 sshd | 需要 | 不需要 |
| 底层连接 | 对 target 建立 SSH client | 只连接 via host |
| run | 在 target 上执行 | 在 via 上执行代理命令 |
| login | 登录 target SSH | 在 via 上执行 `login_command` |
| scp/download | 可以复用 SSH/SFTP | 初版不支持 |
| 典型场景 | bastion 到内网机器 | PVE LXC、Docker、vhost |

## 配置模型

在 `Host` 上新增字段：

```go
type Host struct {
    Backend      string `json:"backend,omitempty"`
    Via          string `json:"via,omitempty"`
    RunTemplate  string `json:"run_template,omitempty"`
    LoginCommand string `json:"login_command,omitempty"`
}
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `backend` | 否 | 空值等同于 `ssh`；可选值：`ssh`、`command_proxy` |
| `via` | command_proxy 必填 | 真实 SSH host 的 name/IP/唯一匹配关键字 |
| `run_template` | run 必填 | `sshc run` 使用的模板，必须包含 `{{cmd}}` |
| `login_command` | login 可选 | `sshc login` 使用的完整交互命令 |

不建议使用 `command_prefix` 或 `execute_prefix`。prefix 只能做字符串前置拼接，
无法可靠表达复杂 shell 命令：

```bash
cd /opt/app && APP_ENV=prod ./init.sh
```

如果简单拼成：

```bash
pct exec 101 -- cd /opt/app && APP_ENV=prod ./init.sh
```

`&&` 后半段可能跑在 `pve-host` 上，而不是 LXC 内部。因此使用模板：

```json
"run_template": "pct exec 101 -- sh -lc {{cmd}}"
```

其中 `{{cmd}}` 由 sshc 对最终命令做 shell quote 后替换。

## 配置示例

### PVE LXC

```json
{
  "hosts": [
    {
      "name": "pve-host",
      "ip": "192.168.1.20",
      "auth_ref": "ops"
    },
    {
      "name": "lxc-app",
      "backend": "command_proxy",
      "via": "pve-host",
      "run_template": "pct exec 101 -- sh -lc {{cmd}}",
      "login_command": "pct enter 101",
      "group": "lxc",
      "remark": "PVE CT 101"
    }
  ]
}
```

使用：

```bash
sshc run lxc-app -- hostname
sshc run lxc-app --cwd /opt/app -e APP_ENV=prod -- ./init.sh
sshc login lxc-app
```

### Docker container

```json
{
  "name": "docker-app",
  "backend": "command_proxy",
  "via": "docker-host",
  "run_template": "docker exec app sh -lc {{cmd}}",
  "login_command": "docker exec -it app sh",
  "group": "container"
}
```

### 自定义 vhost

```json
{
  "name": "vhost-app",
  "backend": "command_proxy",
  "via": "gateway-host",
  "run_template": "vhostctl exec app -- sh -lc {{cmd}}",
  "login_command": "vhostctl enter app"
}
```

## 命令行为

### run

用户命令：

```bash
sshc run lxc-app -- hostname
```

内部流程：

```text
target = resolve("lxc-app")
if target.backend == command_proxy:
    via = resolve(target.via)
    final_cmd = buildRemoteCommand(user_cmd, env, cwd, sudo, timeout)
    quoted_cmd = shellQuote(final_cmd)
    proxied_cmd = render(target.run_template, {"cmd": quoted_cmd})
    execute via with proxied_cmd
```

实际在 `pve-host` 上执行：

```bash
pct exec 101 -- sh -lc 'hostname'
```

带 cwd/env：

```bash
sshc run lxc-app --cwd /opt/app -e APP_ENV=prod -- ./init.sh
```

实际在 `pve-host` 上执行：

```bash
pct exec 101 -- sh -lc 'cd '"'"'/opt/app'"'"' && APP_ENV='"'"'prod'"'"' ./init.sh'
```

说明：

- run log 的 host 应记录为 `lxc-app`。
- 日志中可以增加 `via: pve-host` 和 `backend: command_proxy`，方便排查。
- exit code/错误仍来自代理命令。
- `run_template` 缺少 `{{cmd}}` 时应拒绝执行。

### batch-run

`batch-run` 应复用 run 后端。配置 host 和 command_proxy host 可以混用：

```bash
sshc batch-run --hosts web-1,lxc-app,docker-app -- uptime
```

每个 host 按自己的 backend 执行，日志仍按各自目标 host 记录。

### login

用户命令：

```bash
sshc login lxc-app
```

内部流程：

```text
target = resolve("lxc-app")
via = resolve(target.via)
connect via with PTY
run target.login_command in interactive session
```

实际在 `pve-host` 上交互执行：

```bash
pct enter 101
```

规则：

- `login_command` 是完整命令，不是 prefix。
- `login_command` 为空时，`sshc login lxc-app` 应报错。
- command_proxy login 默认不记录完整终端输入输出，保持现有 login 日志边界。
- `login_command` 应直接走 PTY session，不应通过 `Run()` 非交互执行。

### scp/upload/download

初版不支持 command_proxy 传输：

```bash
sshc scp -l app.jar -r /opt/app/app.jar lxc-app
sshc download -r /var/log/app.log -l logs lxc-app
```

应返回明确错误：

```text
host lxc-app uses command_proxy backend; upload/download is not supported yet
```

原因：

- 文件到底传到 `via` 还是传进逻辑 target，需要明确。
- PVE LXC 更合理的是 `pct push/pull`。
- Docker 更合理的是 `docker cp`。
- 这些传输模板和 run/login 不同，应单独设计。

## 模板规则

初版只支持一个占位符：

```text
{{cmd}}
```

规则：

- `{{cmd}}` 表示 shell-quoted final command。
- `run_template` 必须包含 `{{cmd}}`。
- 不支持 `{{raw_cmd}}`，避免用户绕过 quote 后把复杂命令拆到 via shell。
- 模板渲染只做占位符替换，不引入通用模板引擎。

示例：

```json
"run_template": "pct exec 101 -- sh -lc {{cmd}}"
```

如果 final command 是：

```bash
cd /opt/app && APP_ENV=prod ./init.sh
```

渲染结果：

```bash
pct exec 101 -- sh -lc 'cd /opt/app && APP_ENV=prod ./init.sh'
```

## 解析和校验

`cfg doctor` 和保存前校验建议增加：

- `backend` 只能为空、`ssh` 或 `command_proxy`。
- `backend=command_proxy` 时：
  - `via` 必填。
  - `via` 能解析到存在的 host。
  - `via` 不能指向自己。
  - `via` host 不能也是 `command_proxy`，初版禁止嵌套。
  - `run_template` 非空时必须包含 `{{cmd}}`。
  - `run_template` 为空时，`run` 报错，但 `login` 可仅依赖 `login_command`。
  - `login_command` 为空时，`login` 报错，但 `run` 可仅依赖 `run_template`。
- `backend=command_proxy` 时不要求 `ip/port/user/key_path/password` 出现在目标 host 上。
- 认证信息来自 `via` host，不来自逻辑 target。

## 与 auth/defaults 的关系

command_proxy 目标 host 是逻辑资源，它不直接连接 SSH，因此：

- `auth_ref`、`user`、`key_path`、`password_enc` 对 command_proxy target 初版无效。
- `connect_timeout` 应作用于 `via` 的 SSH 连接。
- `run_timeout` 应作用于代理命令整体。
- `host_key_check` 应作用于 `via` 的 SSH host key。
- `group`、`remark`、`name` 仍属于逻辑 target，用于 list/filter/log。

为了减少误解，`host show` 可以展示：

```json
{
  "name": "lxc-app",
  "backend": "command_proxy",
  "via": "pve-host",
  "run_template": "pct exec 101 -- sh -lc {{cmd}}",
  "login_command": "pct enter 101",
  "effective_via": {
    "name": "pve-host",
    "ip": "192.168.1.20"
  }
}
```

## 日志

run log 建议继续按逻辑 target 分文件：

```text
logs/lxc-app.log
```

日志记录新增字段：

```json
{
  "host": "lxc-app",
  "backend": "command_proxy",
  "via": "pve-host",
  "command": "hostname",
  "proxied_command": "pct exec 101 -- sh -lc 'hostname'"
}
```

注意：

- `proxied_command` 可能包含用户命令和 env，和现有 command 一样需要按当前日志策略处理。
- 不记录密码。
- login 不默认记录完整终端输入输出。

## 安全边界

command_proxy 执行能力本质上把 `via` host 的权限扩展到逻辑 target：

- `via` host 上的账号必须有执行 `pct exec`、`pct enter` 或对应代理命令的权限。
- 如果需要 `sudo pct exec`，建议写进模板：

```json
"run_template": "sudo pct exec 101 -- sh -lc {{cmd}}",
"login_command": "sudo pct enter 101"
```

风险和处理：

| 风险 | 处理 |
| --- | --- |
| 复杂命令在 via shell 上被错误拆分 | 使用 `{{cmd}}` shell quote，不提供 raw 占位符 |
| 用户误以为 target 有独立 SSH 信任 | 文档明确 host key 校验发生在 via host |
| via host 权限过大 | 由用户自行控制 via host 账号权限；sshc 不做权限隔离 |
| login_command 执行高权限交互命令 | 配置由本地用户维护，`cfg show` 默认不 mask 这些非 secret 字段 |
| run_template 注入风险 | 模板来自本地配置，用户命令由 sshc quote 后替换 |

## 代码落点建议

建议新增或修改：

```text
internal/core/store.go
internal/core/config_doctor.go
internal/core/config_resolve.go
internal/core/ssh.go
internal/core/command_proxy.go
internal/core/command_proxy_test.go
internal/command/run.go
internal/command/login.go
internal/command/host.go
README.md
README.zh-CN.md
docs/TODO.md
```

建议先抽象执行器：

```go
type RemoteExecutor interface {
    Run(command string, opts RunOptions) ([]byte, error)
    Login(opts LoginOptions) error
}
```

但不必一开始做大重构。更小的实现路径：

1. 在 `ExecuteRemote` 入口识别 `host.Backend == "command_proxy"`。
2. 调用 `ExecuteCommandProxy(host, command, opts)`。
3. `ExecuteCommandProxy` 解析 `via`，连接 via，渲染模板并执行。
4. `LoginRemoteWithOptions` 同样识别 command_proxy，走 `LoginCommandProxy`。
5. upload/download/scp 在命令层或 core 层遇到 command_proxy 直接报错。

## 测试建议

核心测试：

- `TestCommandProxyRunRendersTemplate`
- `TestCommandProxyRunRejectsTemplateWithoutCmd`
- `TestCommandProxyRunUsesViaHostAuth`
- `TestCommandProxyRunKeepsLogicalHostForLogs`
- `TestCommandProxyLoginUsesLoginCommand`
- `TestCommandProxyLoginRejectsMissingLoginCommand`
- `TestCommandProxyRejectsNestedVia`
- `TestCommandProxyRejectsUploadDownload`
- `TestCfgDoctorReportsInvalidCommandProxy`

不需要真实 PVE 环境。通过 fake `RemoteClient` 验证最终执行命令即可。

## 分期建议

### P1: 配置模型和校验

状态：已完成。

- 新增 `backend/via/run_template/login_command` 字段。
- `cfg doctor` 增加 command_proxy 校验。
- `host add/set/unset/show/list` 支持展示和维护字段。

### P2: run 和 batch-run

状态：已完成。

- `run` 支持 command_proxy。
- `batch-run` 复用 run 行为。
- 日志增加 backend/via/proxied_command。

### P3: login

状态：已完成。

- `login` 支持 command_proxy。
- 使用 via host 建立 PTY session。
- 执行 `login_command`。

### P4: 文档和后续计划

状态：已完成。

- README/README.zh-CN 增加 command_proxy 示例。
- TODO 标记 run/login 支持状态。
- 明确 upload/download/scp 暂不支持。

## 待确认事项

1. `backend` 字段是否接受空值等同于 `ssh`。建议接受，保持旧配置兼容。
2. `run_template` 是否必须使用 `sh -lc {{cmd}}`。建议不强制，但文档推荐。
3. `login_command` 是否支持模板变量。初版建议不支持，后续需要再加。
4. `host add` 是否需要一次性支持 `--backend/--via/--run-template/--login-command`。建议支持，方便录入。
5. `host import plain/csv` 是否支持 command_proxy 字段。建议支持 plain，CSV 可后续补。
6. `scp/upload/download` 是否要设计 `upload_template/download_template`。建议另起设计，不进入初版。

## 结论

command_proxy 应作为独立 backend 实现，而不是扩展 jump host。

推荐配置模型：

```json
{
  "name": "lxc-app",
  "backend": "command_proxy",
  "via": "pve-host",
  "run_template": "pct exec 101 -- sh -lc {{cmd}}",
  "login_command": "pct enter 101"
}
```

初版已经支持 `run`、`batch-run` 和 `login`，并明确不支持文件传输。这样可以覆盖
PVE/LXC/vhost 初始化和日常命令执行场景，同时保持 sshc 是轻量 SSH 运维工具，
而不是 PVE 专用管理平台。后续如果要支持脚本注入或文件传输，应单独设计
`pct push/pull`、`docker cp` 或通用传输模板，避免误把文件上传到 `via` 主机。
