# sshc 易用性与效率增强设计

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-08 | Codex | 初版，整理与同类 SSH 工具对比后的可优化功能点和优先级 |

## 背景

`sshc` 当前已经具备轻量 SSH 运维 CLI 的主链路能力：

- `host/auth/cfg` 管理主机、凭证和配置。
- `run/login/batch-run/upload/download/log` 覆盖日常远程操作。
- `known_hosts` 校验、`host trust`、jump host、command_proxy、Web console 已形成基础闭环。
- README 已明确定位为面向开发者和 AI Agent 的轻量 SSH operations CLI。

与 `lazyssh`、`dssh`、`susshi`、`assh`、`pssh`、`hop`、`purple` 等项目相比，`sshc` 不需要走成重型 TUI、完整编排平台或云资源管理器。后续增强应围绕：

- 少输入：减少重复主机、认证、命令参数。
- 少出错：连接检查、配置校验、host key 和批量执行反馈更清晰。
- 可复用：常用命令、主机分组、模板和导入能力可沉淀。
- 可追踪：继续强化日志、任务 ID、批量执行结果和失败重跑。
- 可自动化：保持 CLI-first，兼顾脚本、CI 和 AI Agent 调用。

本文只记录设计和取舍，不开始实现。

## 目标

- 梳理下一批可提升易用性和执行效率的功能点。
- 给出推荐优先级，避免功能膨胀。
- 统一命令命名和配置模型方向。
- 明确 `~/.ssh/config` import/sync 是否复用现有 `host import`。
- 为后续实施计划提供输入。

## 非目标

- 不做 Ansible 替代品，不引入 role/playbook/inventory 体系。
- 不做完整云厂商同步平台。
- 不做重型 TUI 作为主入口。
- 不把 `sshc serve` 扩展为多人堡垒机或中心化权限系统。
- 不默认记录 interactive terminal 的完整输入输出。

## 设计原则

### CLI-first

所有核心能力先有可脚本化 CLI，再考虑 WebUI 或交互增强。

### Local-first

配置、日志、凭证和审计继续保存在本机配置目录，不引入远端控制面。

### Small but complete

每个能力只覆盖日常高频 80% 场景，复杂平台能力留给 OpenSSH、Ansible、Teleport、云厂商控制台或专用工具。

### 可组合

新功能应复用现有 host/auth/defaults/jump/command_proxy/log 模型，而不是新增平行体系。

## 推荐优先级总览

| 优先级 | 功能 | 价值 | 建议阶段 |
| --- | --- | --- | --- |
| P1 | `sshc check` 主机健康检查 | 快速定位网络、认证、known_hosts、jump 问题 | 优先 |
| P1 | `task`/`snippet` 常用命令 | 减少重复输入，便于批量和 AI Agent 调用 | 优先 |
| P1 | group defaults / host template | 大量主机场景减少重复配置 | 优先 |
| P1 | selector 增强 | 主机多时提升 run/log/upload/download 体验 | 优先 |
| P1 | `host import --from-ssh-config` | 降低已有 SSH 配置用户迁移成本 | 优先 |
| P2 | SSH key 部署 | 密码迁移到 key、初始化新机器更方便 | 中期 |
| P2 | completion 增强 | 降低命令记忆成本 | 中期 |
| P2 | batch-run 汇总与失败重跑 | 批量初始化和巡检更高效 | 中期 |
| P2 | recent/pinned hosts | 高频主机快速访问 | 中期 |
| P2 | tunnel/port-forward 管理 | 开发调试数据库、Redis、内部 HTTP 服务 | 中期 |
| P3 | WebUI 操作增强 | 非 CLI 使用体验提升 | 后续 |
| P3 | MCP server | 适配 AI Agent，但需安全设计 | 后续 |
| P3 | PVE/Docker discover | 与 command_proxy 联动，但易膨胀 | 后续 |
| P3 | 安全体检增强 | 提升开源可信度 | 后续 |

## P1: 主机健康检查

### 命令面

```bash
sshc check devhost
sshc check --group testing --parallel 10
sshc check --all
sshc check --hosts devhost,dbhost
```

### 检查内容

- host 是否能解析为 effective host。
- `auth_ref` 是否存在。
- key 文件是否存在。
- known_hosts 是否存在目标记录。
- TCP 连接是否可达。
- SSH handshake 是否成功。
- auth 是否成功。
- jump host 链路是否成功。
- command_proxy 的 `via` 是否可达，`run_template/login_command` 是否配置完整。

### 输出

默认表格：

```text
Name       Addr        SSH  Auth  HostKey  Latency  Error
devhost    10.*.*.8    ok   ok    ok       42ms     -
dbhost     10.*.*.9    no   -     ok       -        timeout
```

机器可读输出：

```bash
sshc check --group testing --json
```

### 边界

- `check` 不执行用户命令。
- command_proxy 初版只检查 `via` host 和配置完整性，不进入容器内部执行探测命令。
- 大量主机检查必须支持 `--parallel`，默认并发保守，例如 5。

## P1: 常用命令 task/snippet

### 目标

沉淀常用远程命令，减少重复手写 shell，并让 batch-run 和 AI Agent 调用更稳定。

### 命令面

建议命令名使用 `task`，`snippet` 可作为后续 alias 或文档概念，不建议两个资源并存。

```bash
sshc task add restart-nginx -- sudo systemctl restart nginx
sshc task add app-status --cwd /opt/app -- ./status.sh
sshc task list
sshc task show restart-nginx
sshc task rm restart-nginx --yes
sshc task run restart-nginx devhost
sshc task run restart-nginx --group testing --parallel 5
```

### 配置模型

```json
{
  "tasks": [
    {
      "name": "restart-nginx",
      "command": "systemctl restart nginx",
      "sudo": true,
      "cwd": "",
      "env": {},
      "timeout": "30s",
      "remark": "restart nginx service"
    }
  ]
}
```

### 变量支持

初版可以不做模板变量。第二阶段再支持：

```bash
sshc task run deploy devhost --var version=v1.2.3
```

模板：

```text
cd /opt/app && ./deploy.sh {{version}}
```

变量必须走显式 `--var`，避免直接拼接自由 shell 造成不可控注入。

### 与 run/batch-run 的关系

- `task run NAME HOST` 内部复用 `run`。
- `task run NAME --group GROUP` 内部复用 `batch-run`。
- 每次执行仍写入 per-host run log，并记录 `task_name`。

## P1: group defaults / host template

### 目标

大量主机共享 `auth_ref`、`jump`、`port`、`remote_script_dir` 等配置时，减少重复填写。

### 配置模型

建议新增 `groups`，不使用单独 `templates` 起步，避免两个继承系统同时存在：

```json
{
  "groups": {
    "testing": {
      "auth_ref": "dev-root",
      "jump": "bastion",
      "port": 22,
      "remote_script_dir": "/tmp",
      "connect_timeout": "10s"
    }
  }
}
```

host：

```json
{
  "name": "app-01",
  "ip": "10.0.0.8",
  "group": "testing"
}
```

### 合并优先级

在当前 effective host 规则中插入 group defaults：

```text
命令行显式参数 > host 内联字段 > group defaults > auth_ref > defaults > 内置默认值
```

说明：

- host 内联字段优先级高于 group，便于个别主机覆盖。
- group 里的 `auth_ref` 会继续解析 auth profile。
- 如果 host 和 group 都设置 `auth_ref`，host 优先。

### 命令面

```bash
sshc group list
sshc group show testing
sshc group set testing auth_ref dev-root
sshc group set testing jump bastion
sshc group unset testing jump
```

也可以先不做 `group` 命令，只支持配置和 `cfg set groups.testing.auth_ref`。但从可用性看，建议后续有独立 `group` 管理命令。

## P1: selector 增强

### 当前基础

`login` 无 target 或多匹配时已有交互式选择能力。

### 扩展方向

把 selector 扩展到常用命令：

```bash
sshc run --select -- uptime
sshc upload --select -l ./dist -r /opt/app/
sshc download --select -r /var/log/app.log -l ./logs/
sshc log --select
```

### 选择列表字段

- name
- group
- masked addr
- auth type
- remark
- recent/pinned 标记

### 边界

- 不做完整 TUI 主程序。
- selector 只在命令需要选择 host 时打开。
- 非交互环境检测到 `--select` 应失败并提示使用明确 host。

## P1: `~/.ssh/config` import/sync

### 是否使用 `host import --from-ssh-config`

建议使用现有 `host import` 增加选项：

```bash
sshc host import --from-ssh-config
sshc host import --from-ssh-config -f ~/.ssh/config --dry-run
sshc host import --from-ssh-config --group imported --skip-existing --yes
sshc host import --from-ssh-config --overwrite --yes
```

理由：

- 语义上仍是“导入主机”，放在 `host import` 下最自然。
- 可以复用现有 import 的 `--dry-run`、`--skip-existing`、`--overwrite`、`--yes`、冲突检查和保存逻辑。
- 避免新增 `host import-ssh-config` 或顶层 `ssh-config` 命令导致命令面变散。
- `--from-ssh-config` 可以作为一种 source，而不是 `--format ssh-config`。因为它默认读取 `~/.ssh/config`，不只是文件格式。

### 参数设计

`--from-ssh-config`

- bool 选项。
- 表示 source 是 OpenSSH config。
- 如果未传 `-f/--file`，默认读取 `~/.ssh/config`。
- 如果传了 `-f/--file`，读取指定 ssh config 文件。

`-f/--file`

- 复用现有 `host import -f`。
- 在 `--from-ssh-config` 下表示 ssh config 文件路径。

`--group`

- 给导入的 host 设置默认 group。
- 如果后续支持从 ssh config comment/tag 提取 group，再单独设计。

`--auth`

- 强制所有导入 host 使用同一个 auth profile。
- 设置后可以选择不导入 `User`/`IdentityFile` 为 host 内联认证字段，避免配置分散。

`--import-identity-file`

- 可选增强。默认建议导入 `IdentityFile` 到 host `key_path`。
- 如果用户传 `--auth`，默认不导入 `IdentityFile`，除非显式设置该选项。

### 字段映射

| SSH config | sshc host | 说明 |
| --- | --- | --- |
| `Host` | `name` | 跳过包含 `*`、`?` 的通配 Host |
| `HostName` | `ip` | 可以是 IP 或 DNS 名称 |
| `User` | `user` | `--auth` 存在时默认不导入 |
| `Port` | `port` | 合法范围 `1..65535` |
| `IdentityFile` | `key_path` | 多个时初版取第一个，并在 warning 中提示 |
| `ProxyJump` | `jump` | 仅支持单个 jump host name |
| `ProxyCommand` | `remark`/warning | 初版不转换，提示不支持 |
| `LocalForward`/`RemoteForward` | skip/warning | 后续 tunnel 功能处理 |

### 解析边界

- 跳过 `Host *`、`Host *.example.com` 等 pattern 条目。
- 跳过缺少 `HostName` 且无法作为直接主机名使用的条目。
- `Include` 初版可以支持标准库/第三方 parser 能力；如果 parser 不支持，先 warning。
- `Match` block 初版不支持，warning。
- `ProxyJump a,b` 多级跳板初版不支持，warning。
- `IdentityFile` 的 `~` 保留原样或规范化为 `~` 开头路径，不强制展开成绝对路径。

### import 与 sync 的边界

初版只做 import，不做持续 sync。

```bash
sshc host import --from-ssh-config --dry-run
sshc host import --from-ssh-config --overwrite --yes
```

后续如果要做 sync，需要记录来源元数据，例如：

```json
{
  "name": "devhost",
  "ip": "192.168.1.10",
  "source": {
    "type": "ssh_config",
    "path": "~/.ssh/config",
    "host": "devhost"
  }
}
```

有了来源元数据后，才适合做：

```bash
sshc host sync --from-ssh-config
```

暂不建议初版直接叫 sync，因为覆盖、删除、本地修改保留策略都需要更严格设计。

## P2: SSH key 部署

### 命令面

```bash
sshc key push devhost --pub ~/.ssh/id_ed25519.pub
sshc key push --group testing --pub ~/.ssh/id_ed25519.pub --parallel 5
sshc key push devhost --pub ~/.ssh/id_ed25519.pub --verify
```

### 行为

- 确保远端 `~/.ssh` 存在，权限 `700`。
- 追加 public key 到 `~/.ssh/authorized_keys`。
- 去重。
- 设置 `authorized_keys` 权限 `600`。
- `--verify` 可尝试使用对应 private key 重新连接验证。

### 边界

- 不生成 key pair，至少初版不做。
- 不管理远端用户生命周期。
- 不删除已有 authorized_keys。

## P2: completion 增强

### 当前基础

gcli/v3 已支持 completion 生成能力。

### 增强方向

```bash
sshc --gen-completion bash
sshc --gen-completion zsh
sshc --gen-completion fish
sshc --gen-completion powershell
```

后续增强 host name 补全：

```bash
sshc run <TAB>
sshc login <TAB>
sshc log <TAB>
sshc host show <TAB>
```

### 边界

- completion 不应解密密码。
- host 补全只输出 name，不输出 IP 或 remark。
- 如果读取配置失败，静默返回空候选，避免 shell 卡住。

## P2: batch-run 汇总和失败重跑

### 增强命令

```bash
sshc batch-run --group testing --summary table -- uptime
sshc batch-run --group testing --output-dir tmp/batch-logs -- ./init.sh
sshc batch-run --rerun-failed 20260708-120102-a1b2
```

### batch log

建议在现有 per-host log 之外新增 batch summary：

```text
{logs_path}/batch/{yyyyMMdd}.jsonl
```

字段：

- batch_id
- started_at
- ended_at
- hosts
- success_count
- failed_count
- timeout_count
- task_ids
- command/script

失败重跑基于 `batch_id` 找到失败 host，再复用原命令参数。

### 边界

- 不实现复杂 DAG。
- 不做自动 retry 策略，初版只做手动 `--rerun-failed`。

## P2: recent/pinned hosts

### 目标

主机数量增加后，让高频主机更容易找到。

### 命令面

```bash
sshc host pin devhost
sshc host unpin devhost
sshc list --pinned
sshc list --recent
```

### 配置和状态

`pinned` 可以存在 host 配置中：

```json
{
  "name": "devhost",
  "pinned": true
}
```

`recent` 不建议写入主配置，建议存 runtime state：

```text
~/.config/sshc/state.json
```

避免每次 run/login 都修改主配置，导致配置文件频繁变化。

## P2: tunnel/port-forward 管理

### 命令面

```bash
sshc tunnel add mysql-dev --host inner-db -L 3307:127.0.0.1:3306
sshc tunnel start mysql-dev
sshc tunnel list
sshc tunnel stop mysql-dev
```

### 配置模型

```json
{
  "tunnels": [
    {
      "name": "mysql-dev",
      "host": "inner-db",
      "local": "127.0.0.1:3307",
      "remote": "127.0.0.1:3306",
      "remark": "testing mysql"
    }
  ]
}
```

### 实现边界

- 初版只支持 local forward，即 `-L`。
- 不做长期 daemon 管理，`start` 前台运行即可。
- 后续再考虑后台进程和 pid/state 管理。

## P3: WebUI 增强

### 可增强点

- host 详情页直接 run command。
- 日志详情页按 task_id 展示 stdout/stderr。
- Web 上传/下载文件。
- batch-run 页面。
- check 页面。

### 边界

- WebUI 仍是本地控制台，不做多人权限系统。
- 绑定非 loopback 必须继续要求 token。
- Web API 不返回明文 password/password_enc。

## P3: MCP server

### 价值

`sshc` 面向 AI Agent 的定位明确，MCP 可以让 Agent 安全调用受控 SSH 能力。

### 初版工具

- `list_hosts`
- `check_host`
- `run_command`
- `upload_file`
- `download_file`
- `read_run_log`

### 安全策略

- 默认 readonly，只允许 list/check/log。
- run/upload/download 需要显式配置启用。
- 可限制 allowed groups。
- 可限制危险命令，或要求 confirm。
- 永不暴露 password/password_enc。

### 边界

- 不做云端 MCP 服务。
- 不默认开放网络监听。
- MCP 计划必须单独写安全设计，不直接进入实现。

## P3: PVE/Docker discover

### 目标

复用 command_proxy，自动发现逻辑 host。

### 命令面

```bash
sshc discover pve pve-host --dry-run
sshc discover docker devhost --dry-run
```

### 输出

- PVE LXC 生成 `backend=command_proxy`、`via=pve-host`、`run_template`、`login_command`。
- Docker container 生成 `docker exec` / `docker attach` 相关配置。

### 边界

- 初版最多选择 PVE 或 Docker 一个方向。
- 不做 PVE 资源管理平台。
- 不自动覆盖已有 host，必须 dry-run 预览。

## P3: 安全体检增强

### 命令面

```bash
sshc cfg doctor --security
```

### 检查项

- `sshc.config.json` 和 `key` 权限是否过宽。
- 是否存在明文 `password`。
- 是否配置 `host_key_check=insecure`。
- 是否存在无法解密的 `password_enc`。
- 是否存在不存在的 key_path。
- Web serve 是否配置过弱 token。

### 边界

- 只检查本地配置和文件权限。
- 不尝试修复，除非后续增加显式 `--fix`。

## 不建议近期投入的方向

- 完整 Ansible 替代：role、playbook、inventory、变量矩阵会显著扩大复杂度。
- 重型 TUI 主程序：同类工具已有优势，`sshc` 的差异在 CLI-first 和日志审计。
- 多云自动同步：维护成本高，容易偏离轻量工具定位。
- 多用户 Web 管理台：会变成堡垒机/权限系统，需要完全不同的安全模型。

## 建议实施顺序

### 第一阶段：P1 易用性基础

1. `sshc check`
2. `host import --from-ssh-config`
3. group defaults
4. `task` 常用命令
5. selector 扩展

### 第二阶段：P2 效率增强

1. completion host 补全
2. batch summary 和 failed rerun
3. recent/pinned hosts
4. SSH key push
5. tunnel local forward

### 第三阶段：P3 扩展入口

1. WebUI check/log/run 增强
2. MCP server 安全设计与实现
3. PVE/Docker discover
4. `cfg doctor --security`

## 结论

`sshc` 后续增强应继续围绕“轻量 SSH operations CLI”推进，而不是扩展成完整编排平台。

近期最值得做的是：

```text
check -> host import --from-ssh-config -> group defaults -> task -> selector
```

其中 `~/.ssh/config` 导入建议直接复用现有 `host import`，新增 `--from-ssh-config` 作为 source 选项。初版只做 import，不做 sync；sync 需要先补充来源元数据和覆盖/删除策略，后续再单独设计。
