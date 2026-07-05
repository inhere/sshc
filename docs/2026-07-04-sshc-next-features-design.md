# sshc 后续能力设计

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-04 | Codex | 初版，整理 TODO 后续能力、优先级和核心设计 |
| v0.2 | 2026-07-04 | Codex | 明确 gcli/v3 迁移、命令分组、host/auth/cfg/batch-run 命名和 jump 方向 |
| v0.3 | 2026-07-04 | Codex | 更新当前实现状态，补充 add/host add --jump 已支持持久化跳板配置 |
| v0.4 | 2026-07-05 | Codex | 对齐当前实现状态，标记 login/cfg defaults/host set-unset 已完成，明确下一步为 cfg export/import |

## 背景

`sshc` 当前已经具备轻量 SSH 运维工具的主链路能力：

- 主机配置保存在 `~/.config/sshc/sshc.config.json`。
- 支持 `add/list/run/login/scp/download/log`。
- 支持密码加密存储、SSH key、`~/.ssh/config` 读取、脚本执行、文件传输、日志记录。
- 支持 `logs_path`、`defaults.*` 等全局配置项扩展。
- 支持 `auth`、`host`、`cfg` 管理命令，支持 batch-run 和标准 SSH jump host。

后续需求集中在三个方向：

- 配置和凭证模型继续完善，让多主机、多账号、多环境管理更自然。
- 单机命令能力扩展到批量执行和跳板连接，但保持轻量边界。
- 发布为开源工具前补齐基础安全、可诊断、可维护能力。

本文只做设计，不开始实现。

## 目标

- 保持 `sshc` 的核心定位：轻量 SSH 运维 CLI，而不是完整编排平台。
- 优先补齐配置管理、主机管理、凭证复用这些基础能力。
- 为批量执行、跳板机、导入导出建立稳定配置结构。
- 引入 `github.com/gookit/gcli/v3` 支持多级命令、命令分组和 completion。
- 明确哪些需求暂不做，避免工具复杂度失控。

## 非目标

- 不读取 `~/.ssh/` 下的 password 文件。
- 不新增明文 password 文件约定。
- 不做完整 Ansible 类 playbook、inventory、role、template 系统。
- 不优先实现 PVE/LXC/vhost 特定执行器。
- 不默认记录 `login` 的完整终端输入输出。

## 总体定位

`sshc` 应保持在如下边界内：

```text
手写 ssh/scp < sshc < Ansible/Teleport/完整堡垒机
```

适合做：

- 管理常用 SSH 主机。
- 对单台或少量主机执行命令、脚本和文件传输。
- 保存可检索日志。
- 复用账号和 key 配置。
- 通过标准 SSH jump host 访问内网机器。

不适合做：

- 大规模复杂编排。
- 长期 daemon 或中心化控制面。
- 权限审计堡垒机。
- 面向云厂商或 PVE 的专用资源管理平台。

## CLI 框架迁移

从这个版本开始需要支持多级命令，例如：

```bash
sshc host add
sshc auth add
sshc cfg export
```

建议先从 `github.com/gookit/goutil/cflag/capp` 迁移到 `github.com/gookit/gcli/v3`。

迁移依据：

- `gcli/v3` 原生支持多级命令，命令结构比继续扩展 `cflag/capp` 更直接。
- `gcli/v3` 内置 completion 生成能力，可复用全局 `--gen-completion`，无需新增顶层 `completion` 命令。
- `gcli.Command` 支持 `Category string`，可以把低频管理命令分组展示，避免干扰常用命令。
- `tools/gofer` 已使用 `github.com/gookit/gcli/v3 v3.8.1`，可参考其 `internal/commands` 的命令组织方式。

### 命令分类

顶层 help 建议按 gcli category 分组。

高频命令保持默认分组：

```text
run
login
scp
download
log
add
list
batch-run
```

管理命令设置：

```go
Category: "Management"
```

包含：

```text
host
auth
cfg
```

如果后续觉得 `batch-run` 不应和单机热路径混在一起，可设置：

```go
Category: "Operations"
```

初版建议 `batch-run` 先保持默认分组，因为它是执行型命令，和 `run` 使用关系更近。

### 迁移兼容要求

迁移到 `gcli/v3` 时，已有命令面必须继续可用：

```bash
sshc add ...
sshc list
sshc ls
sshc run ...
sshc login ...
sshc connect ...
sshc scp ...
sshc upload ...
sshc download ...
sshc dl ...
sshc log ...
sshc logs ...
```

其中 `add/list` 作为兼容和快捷入口保留在顶层，内部复用 `host add/list` 的 handler。

### 目标命令树

```text
sshc
|- run
|- login      # alias: connect
|- scp        # alias: upload
|- download   # alias: dl
|- log        # alias: logs
|- add        # compat shortcut -> host add
|- list       # compat shortcut -> host list, alias: ls
|- batch-run  # alias: brun
|- host       # aliases: hosts,h; category: Management
|  |- add
|  |- list    # alias: ls
|  |- show
|  |- rm      # aliases: remove,del
|  |- rename
|  |- edit
|- auth       # aliases: cred,creds; category: Management
|  |- add
|  |- list    # alias: ls
|  |- show
|  |- rm
|- cfg        # alias: config; category: Management
|  |- path
|  |- show
|  |- get
|  |- set
|  |- unset
|  |- edit
|  |- doctor
|  |- export
|  |- import
```

## 配置模型

当前配置结构已经可作为后续扩展基础：

```json
{
  "logs_path": "logs",
  "hosts": []
}
```

建议演进为：

```json
{
  "logs_path": "logs",
  "defaults": {
    "user": "root",
    "port": 22,
    "connect_timeout": "20s",
    "run_timeout": "60s",
    "remote_script_dir": "/tmp",
    "host_key_check": "known_hosts",
    "known_hosts_path": "~/.ssh/known_hosts"
  },
  "auth_profiles": [
    {
      "name": "dev-root",
      "user": "root",
      "password_enc": "v1:..."
    },
    {
      "name": "deploy-key",
      "user": "deploy",
      "key_path": "~/.ssh/id_rsa"
    }
  ],
  "hosts": [
    {
      "name": "devhost",
      "ip": "192.168.1.10",
      "auth_ref": "dev-root",
      "group": "testing",
      "remark": "testing host",
      "port": 22
    }
  ]
}
```

### 配置优先级

主机连接参数建议按以下优先级合并：

```text
命令行显式参数 > host 内联字段 > auth_ref 凭证配置 > defaults > 内置默认值
```

说明：

- host 内联字段继续保留，保证简单场景仍然直接。
- `auth_ref` 用于多个主机共享账号、密码或 key。
- `defaults` 只放全局默认值，不放主机列表。
- 保存配置时仍由本地 `~/.config/sshc/key` 加密敏感字段。

### 敏感字段

敏感字段包括：

- `password`
- `password_enc`
- 后续可能增加的 `key_passphrase_enc`
- export/import 的临时解密内容

默认输出配置时必须 mask：

```json
{
  "password_enc": "***"
}
```

## cfg 命令

`cfg` 基础能力已经完成，用于让用户管理 `sshc.config.json`，避免手动改 JSON。

建议命令：

```bash
sshc cfg path
sshc cfg show
sshc cfg get logs_path
sshc cfg set logs_path ./logs
sshc cfg set defaults.user root
sshc cfg set defaults.port 2222
sshc cfg set defaults.host_key_check known_hosts
sshc cfg unset logs_path
sshc cfg edit
sshc cfg doctor
sshc cfg export -o sshc-export.enc
sshc cfg import -f sshc-export.enc --key "one-time-key"
```

`cfg` 顶级命令设置 `Category: "Management"`，别名为 `config`。

### cfg path

输出实际配置文件路径：

```bash
sshc cfg path
```

需要说明是否来自 `SSHC_CONFIG`，例如：

```text
~/.config/sshc/sshc.config.json
```

或：

```text
/path/to/sshc.config.json (from SSHC_CONFIG)
```

### cfg show

输出配置内容，默认 mask 敏感字段：

```bash
sshc cfg show
sshc cfg show --raw
```

`--raw` 仅用于明确排障，不建议在 README 主示例中强调。

### cfg get/set/unset

当前支持白名单配置项：

```bash
sshc cfg get logs_path
sshc cfg set logs_path ./runtime/logs
sshc cfg set defaults.user root
sshc cfg set defaults.port 2222
sshc cfg set defaults.host_key_check known_hosts
sshc cfg unset logs_path
```

支持的 key 包括 `logs_path`、`defaults.user`、`defaults.port`、
`defaults.connect_timeout`、`defaults.run_timeout`、`defaults.remote_script_dir`、
`defaults.host_key_check` 和 `defaults.known_hosts_path`。不做通用 JSON path，
避免任意字段写入破坏配置结构。

### cfg edit

使用环境变量打开编辑器：

```text
VISUAL > EDITOR > 系统默认提示
```

如果找不到编辑器，不自动猜测，直接打印配置路径。

### cfg doctor

检查项：

- 配置文件是否存在。
- JSON 是否可解析。
- 配置文件权限是否明显过宽。
- `logs_path` 是否可创建或可写。
- `key` 文件是否存在、格式是否可用。
- `password_enc` 是否能解密。
- host 名称或 IP 是否重复。
- `key_path` 是否存在。
- `auth_ref` 是否指向存在的 auth profile。
- `host_key_check` 是否为已知值。

`doctor` 不应该连接远程主机；连接测试可后续加 `sshc doctor devhost --connect`。

## auth profile

TODO 里原写法是 `cfg cert`，不建议使用 `cert`，因为它容易被理解为 SSH certificate。确定使用顶层 `auth` 资源命令。

`auth` 和 `host` 是同级资源：host 通过 `auth_ref` 引用 auth profile。因此它不放到 `cfg` 下面。

`auth` 顶级命令设置 `Category: "Management"`，别名为 `cred`、`creds`。

建议命令：

```bash
sshc auth add dev-root -u root -p
sshc auth add deploy-key -u deploy --key ~/.ssh/id_rsa
sshc auth list
sshc auth show dev-root
sshc auth rm dev-root
```

### auth add

密码输入必须隐藏：

```bash
sshc auth add dev-root -u root -p
```

不要要求用户在命令行传密码，避免 shell history 泄漏。

### auth list

默认只展示 profile 名、user、认证类型：

```text
Name        User    Auth
dev-root    root    password
deploy-key  deploy  key:~/.ssh/id_rsa
```

### host 使用 auth_ref

`add` 命令可以增加：

```bash
sshc add --ip 192.168.1.10 --name devhost --auth dev-root
```

配置：

```json
{
  "name": "devhost",
  "ip": "192.168.1.10",
  "auth_ref": "dev-root"
}
```

如果 host 同时有 `auth_ref` 和内联 `user/key_path/password_enc`，以内联字段优先。

## host 主机管理命令

当前 `add` 是 upsert，`list` 是查看，但还缺少管理闭环。

主机管理统一放在 `host` 资源命令下，`hosts` 和 `h` 作为别名。`host` 顶级命令设置 `Category: "Management"`。

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host list
sshc host show devhost
sshc host rm devhost
sshc host rename old-name new-name
sshc host edit devhost
```

为兼容现有使用和保持高频入口简短，顶层继续保留：

```bash
sshc add ...
sshc list
sshc ls
```

它们复用 `host add/list` 的 handler。

### show

展示单个 host，默认 mask 敏感字段：

```bash
sshc host show devhost
sshc host show devhost --json
```

### rm

删除 host：

```bash
sshc host rm devhost
sshc host rm devhost --yes
```

默认需要确认；`--yes` 用于脚本。

### edit

打开配置中对应 host 的编辑界面有两种实现路径：

- 初版：打印配置文件路径和 host JSON 片段，不做局部编辑。
- 进阶：写入临时 JSON，编辑后校验再合并回配置。

建议初版先不做复杂局部编辑，避免引入 JSON patch 合并风险。

### list 增强

建议增加：

```bash
sshc host list --group testing
sshc host list --match gpu
sshc host list --json
sshc host list --show-ip
```

其中 `--show-ip` 已存在。顶层 `sshc list` 也应支持同样参数，作为兼容快捷入口。

## login 交互选择

`login` 无输入、未匹配或多匹配时交互选择已完成。

行为：

```bash
sshc login
```

- 如果没有可用 host，提示使用 `sshc add -I`。
- 如果有 host，使用 cliui newui 展示可搜索列表。
- 展示字段：name、group、masked address、remark。
- 选择后进入登录。

当用户输入 target 但匹配多个时：

```bash
sshc login testing
```

不要直接报错，可以进入候选选择列表。

当前只支持 `login`，后续可再考虑：

```bash
sshc run --select -- uptime
sshc scp --select -l app.jar -r /tmp/app.jar
```

## batch-run 批量执行

批量执行有用，但不要把现有 `run` 参数撑得过大。确定新增顶层 `batch-run`，别名 `brun`，少一级命令。

`batch-run` 初版保持在默认命令分组，因为它是执行型命令，和 `run` 使用关系更近。后续如果批量能力增多，可考虑设置 `Category: "Operations"`。

命令：

```bash
sshc batch-run --hosts dev1,dev2 -- uptime
sshc batch-run --hosts-file hosts.txt --script ./deploy.sh
sshc batch-run --group testing --parallel 5 --script ./deploy.sh
sshc brun --hosts dev1,dev2 -- uptime
```

### host 来源

支持三类：

- `--hosts dev1,dev2`
- `--hosts-file hosts.txt`
- `--group testing`

三者初版建议互斥，避免合并规则复杂。

`hosts.txt` 格式：

```text
# comments
dev1
dev2
192.168.1.10
```

### 并发和失败策略

参数：

```bash
--parallel 5
--fail-fast
```

默认：

- `--parallel 1` 或一个较保守的默认值，例如 3。
- 不 fail-fast，跑完所有 host 后汇总。
- 任意 host 失败，整体退出码非 0。

### 输出

默认按 host 分块：

```text
==> dev1
ok

==> dev2
failed

Summary: total=2 success=1 failed=1 elapsed=3.2s
```

后续可加：

```bash
--json
```

### 日志

每台 host 仍写自己的 run log。批量本身可以后续增加 batch summary log，初版不强制。

## jump host

标准 SSH jump host 有明确价值，建议在 batch 之后或 auth profile 之后做。

`jump` 字段配置在最终目标 host 上，含义是“访问该 host 时通过哪个跳板”。方向是：

```text
local -> jump host -> target host
```

因此：

```bash
sshc login inner-db --jump bastion
```

等价于 OpenSSH 的：

```bash
ssh -J bastion inner-db
```

如果 `inner-db` 配置中已有 `"jump": "bastion"`，则 `sshc login inner-db` 默认走跳板。

当前已支持通过 CLI 持久化该配置：

```bash
sshc host add --ip 1.2.3.4 --name bastion --auth ops
sshc host add --ip 10.0.0.8 --name inner-db --auth ops --jump bastion
sshc add --ip 10.0.0.8 --name inner-db --auth ops --jump bastion
```

不要把目标反向挂在 bastion 上，例如不使用 `jump_from`。一个 bastion 往往服务多个内网目标，反挂目标列表会让配置方向混乱。

配置：

```json
{
  "hosts": [
    {
      "name": "bastion",
      "ip": "1.2.3.4",
      "auth_ref": "ops"
    },
    {
      "name": "inner-db",
      "ip": "10.0.0.8",
      "auth_ref": "ops",
      "jump": "bastion"
    }
  ]
}
```

命令覆盖：

```bash
sshc login inner-db
sshc run inner-db --jump bastion -- hostname
sshc login inner-db --jump bastion
sshc scp -l app.jar -r /tmp/app.jar inner-db --jump bastion
```

### 实现边界

底层连接从：

```text
local -> target
```

变为：

```text
local -> jump -> target
```

需要抽象 `newSSHClient`：

- 解析 target host。
- 解析 jump host。
- 先建立 jump SSH client。
- 通过 jump client dial target TCP。
- 在该 net.Conn 上建立 target SSH client。

影响命令：

- run
- login
- scp/upload
- download

因此 jump host 应该单独阶段做，测试覆盖这四类命令的连接构造。

## PVE/LXC/vhost 执行器

TODO 中的 PVE/LXC/vhost 场景有实际价值，但不建议近期产品化。

原因：

- 它不是标准 SSH jump host，而是远端命令代理。
- 上传下载语义不清楚：文件是传到 PVE 还是传进 LXC。
- login PTY 行为复杂：`pct enter`、`pct exec` 和普通 SSH 不同。
- 引号、shell、sudo、cwd、env 的组合更容易出错。

建议先不做专用 PVE 模式。需要时可先用普通命令完成：

```bash
sshc run pve-host -- pct exec 101 -- hostname
```

如果未来要做，建议设计成通用 `proxy_command` backend，而不是写死 PVE：

```json
{
  "name": "lxc-app",
  "backend": "proxy_command",
  "via": "pve-host",
  "command_prefix": "pct exec 101 --"
}
```

该能力应独立设计，不进入近期实现计划。

## export/import

导入导出有价值，尤其跨机器迁移时，本机 `password_enc` 依赖本机 key，直接复制配置无法在另一台机器解密。

建议命令：

```bash
sshc cfg export -o sshc-export.enc
sshc cfg import -f sshc-export.enc --key "one-time-key"
```

确定放在 `cfg` 下，避免顶层命令过多。导入导出操作的是整个配置文件，不只是 host。

### 导出流程

- 读取当前配置。
- 解密本机 `password_enc` 到内存。
- 生成随机一次性 key string，或要求用户输入 passphrase。
- 用一次性 key 派生 AES-GCM key。
- 加密整个导出包。
- 输出加密文件和一次性 key string。

### 导入流程

- 读取导出文件。
- 用 key 解密。
- 校验版本和结构。
- 将敏感字段用目标机器本地 key 重新加密。
- 写入当前配置。

### 合并策略

初版建议支持：

```bash
--merge
--replace
--overwrite
```

默认 `--merge`，遇到同名 host 或 auth profile 时拒绝并提示使用 `--overwrite`。

### 安全边界

- 一次性 key 不写入导出文件。
- 导入前自动备份当前配置。
- 不在日志中输出明文配置。
- 导出包格式需要带版本号，方便未来兼容。

## host key 校验

当前 README 已说明 host key 校验是宽松模式。项目尚未正式发布，可以接受破坏性安全默认值，因此下一版应直接把默认策略切到 `known_hosts`。

建议默认配置：

```json
{
  "defaults": {
    "host_key_check": "known_hosts",
    "known_hosts_path": "~/.ssh/known_hosts"
  }
}
```

如确实需要兼容临时环境或跳过校验，可显式配置：

```json
{
  "defaults": {
    "host_key_check": "insecure"
  }
}
```

建议阶段：

- 初版即默认 `known_hosts`，因为尚未发布，不需要为旧行为保留默认兼容。
- `known_hosts_path` 默认 `~/.ssh/known_hosts`。
- 提供显式 `insecure` 模式用于临时环境，但 README 应明确这是不安全降级。
- 如果目标 host 不在 known_hosts 中，连接应失败并提示用户先用 `ssh-keyscan` 或 OpenSSH `ssh` 信任目标。

## completion

作为开源 CLI，shell completion 很有价值。

迁移到 `gcli/v3` 后，优先使用其内置 completion 生成能力。参考 `tools/gofer`，不再新增顶层 `completion` 命令。

```bash
sshc --gen-completion bash
sshc --gen-completion zsh
sshc --gen-completion fish
```

后续增强可以支持 host name 补全：

```bash
sshc run <TAB>
sshc login <TAB>
```

## run 输出模式

后续可考虑：

```bash
sshc run devhost --json -- uptime
sshc run devhost --quiet -- uptime
sshc run devhost --no-log -- uptime
```

注意：当前 `goph.Run` 输出可能是 stdout/stderr 合并。如果要返回结构化 stdout/stderr 和 exit code，可能需要改为直接管理 SSH session。该项有价值，但优先级低于 cfg/auth/batch/jump。

## 建议实施顺序

当前 P0-P6 以及 login 交互选择已经完成。下一步建议优先实施 `cfg export/import`，
再进入更高成本的安全和体验增强。

### P0: 迁移到 gcli/v3

范围：

- 替换 `github.com/gookit/goutil/cflag/capp` 为 `github.com/gookit/gcli/v3`。
- 保持现有顶层命令兼容。
- 为后续多级命令建立 `internal/command` 或 `internal/commands` 注册结构。
- 管理命令使用 `Category: "Management"`。
- completion 使用 gcli 内置 `--gen-completion`。

状态：已完成。

验收：

- 现有命令行为和参数兼容。
- `add/list/run/login/scp/download/log` 测试通过。
- `sshc --help` 中管理命令按 category 分组展示。
- `sshc --gen-completion bash` 可生成 completion 内容。

### P1: cfg 基础命令

范围：

- `sshc cfg path`
- `sshc cfg show`
- `sshc cfg get logs_path`
- `sshc cfg set logs_path VALUE`
- `sshc cfg set defaults.user VALUE`
- `sshc cfg set defaults.port 2222`
- `sshc cfg unset logs_path`
- `sshc cfg edit`
- `sshc cfg doctor`

状态：已完成，并已扩展到 `defaults.*` 白名单字段。

验收：

- 可以查看实际配置路径。
- 可以修改 `logs_path`。
- `doctor` 能检查 JSON、key、logs_path、host 重复等基础问题。
- 敏感字段默认 mask。

### P2: host 管理补齐

范围：

- `sshc host show HOST`
- `sshc host rm HOST`
- `sshc host rename OLD NEW`
- `sshc host set HOST ...`
- `sshc host unset HOST ...`
- `sshc host list --group`
- `sshc host list --match`
- `sshc host list --json`
- 顶层 `sshc add/list` 继续兼容并复用 handler。

状态：已完成。

验收：

- 不再需要手动编辑 JSON 完成常见 host 管理。
- JSON 输出可用于脚本。
- 默认列表仍保护 IP。

### P3: auth profile

范围：

- 配置新增 `auth_profiles`。
- host 支持 `auth_ref`。
- `sshc auth add/list/show/rm`。
- `sshc add --auth NAME`。

状态：已完成，`auth add` 支持 `--remark`。

验收：

- 多个 host 可共享同一凭证。
- 密码仍加密保存。
- `auth_ref` 缺失时 doctor 能报错。

### P4: login 交互选择

范围：

- `sshc login` 无 target 时进入选择。
- 多个模糊匹配候选时进入选择。

状态：已完成。

验收：

- 可以通过键盘选择 host 并进入 PTY。
- 无 host 时提示 `sshc add -I`。

### P5: batch-run

范围：

- `sshc batch-run --hosts`
- `sshc batch-run --hosts-file`
- `sshc batch-run --group`
- `sshc brun` 作为 alias
- `--parallel`
- `--fail-fast`

状态：已完成。

验收：

- 支持对多个 host 执行命令或脚本。
- 每个 host 有独立输出块。
- 任一失败时整体退出非 0。

### P6: jump host

范围：

- host 配置 `jump`。
- 命令参数 `--jump`。
- 覆盖 run/login/scp/download。

状态：已完成，且 `add/host add --jump` 支持持久化默认跳板。

验收：

- 可以通过 bastion 访问内网 host。
- 不影响无 jump 的现有连接。

### P7: export/import

范围：

- `sshc cfg export`
- `sshc cfg import`
- portable encrypted export package。

状态：待实施，下一步优先。

验收：

- 跨机器迁移时密码可重新加密到目标机器 key。
- 默认 merge，不覆盖已有 host。
- 导入前自动备份。

### P8: 安全和体验增强

范围：

- host key 辅助信任命令，例如 `host keyscan` 或 `host trust`。
- shell completion。
- `run --json/--quiet/--no-log`。

验收：

- 用户可以更方便地维护 `known_hosts` 信任。
- 主流 shell 可以补全命令和 host。

## 需要先确认的问题

1. export/import 的 key 是自动生成一次性 key，还是用户输入 passphrase。建议初版自动生成一次性 key，同时后续可追加 passphrase 模式。
2. import 冲突默认策略是否为 `--merge` 且冲突拒绝。建议默认 merge，不覆盖已有 host/auth，显式 `--overwrite` 才覆盖。
3. 导入前备份目录和保留策略。建议备份到配置目录下 `backups/`，后续再考虑清理策略。

## 结论

配置管理、凭证模型、batch-run、jump host 和 login 交互选择已经完成。下一步建议优先补齐跨机器迁移能力：

```text
P7 cfg export/import -> P8 run 输出模式 / host key 辅助命令 / completion 增强
```

`cfg export/import` 能解决 `password_enc` 依赖本机 key、直接复制配置无法在另一台机器解密的问题，是发布后最容易遇到的真实迁移场景。
