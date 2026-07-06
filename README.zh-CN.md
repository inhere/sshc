# sshc

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/inhere/sshc?style=flat-square)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/inhere/sshc)](https://github.com/inhere/sshc)
[![Unit-Tests](https://github.com/inhere/sshc/actions/workflows/go.yml/badge.svg)](https://github.com/inhere/sshc)

---

[English](./README.md) | 简体中文

`sshc` 是一个轻量 SSH 辅助 CLI，用于管理主机、执行远程命令、传输文件、在远端执行本地脚本，并按主机保存执行日志。

它适合轻量部署、问题排查和日常远程运维。对于这些场景，引入完整自动化平台通常过重，`sshc` 提供的是更直接的命令行工作流。

## 功能特性

- 在 `~/.config/sshc/sshc.config.json` 中管理 SSH 主机
  - 保存主机密码前会先加密，避免明文写入
- 通过 `auth` 管理可复用凭证配置
- 通过 `cfg` 查看和修改本地配置
- 从 `~/.ssh/config` 读取简单主机配置
- 默认使用 `known_hosts` 校验 SSH host key
- 通过主机名、IP 或唯一的模糊匹配结果执行远程命令
  - 将本地 shell 脚本上传到远端执行
- 通过主机配置或 `--jump` 使用单级跳板机连接目标主机
- 注册 command_proxy 逻辑主机，用于 PVE/LXC、Docker 或 vhost 命令代理执行
- 通过 `batch-run/brun` 在多台主机上批量执行命令或脚本
- 支持远端工作目录、超时、环境变量、sudo 和 sudo user
- 通过 `upload` 使用 SFTP 上传和 `download` 下载文件或目录
  - 支持单文件传输 SHA256 校验
- 在 `~/.config/sshc/logs/` 下按主机保存 JSONL 执行日志
- 通过 `login/connect` 打开交互式远端 PTY

## 安装

### 下载 Release

1. **Recommended** 通过 [eget](https://github.com/inherelab/eget) 下载安装: `eget install inhere/sshc`
2. 通过 Golang 安装: `go install github.com/inhere/sshc/cmd/sshc@latest`
3. 从 [GitHub Releases](https://github.com/inhere/sshc/releases) 下载对应平台的归档文件，解压后将 `sshc` 二进制放到 `PATH` 中。

### 从源码构建

```bash
git clone https://github.com/inhere/sshc.git
cd sshc
go build -o sshc ./cmd/sshc
# Windows
go build -o sshc.exe ./cmd/sshc
```

## 快速开始

```bash
sshc add --ip 192.168.1.10 --name devhost -u root -p password
sshc list
sshc run devhost -- uptime
sshc auth add dev-root -u root -p --remark "共享 root 登录"
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root # use auth refer
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
sshc host add --name lxc-app --backend command_proxy --via pve-host --run-template "pct exec 101 -- sh -lc {{cmd}}" --login-command "pct enter 101"
sshc run devhost --script ./deploy.sh
sshc run inner-db --jump bastion -- hostname
sshc run lxc-app -- hostname
sshc batch-run --hosts devhost,web-2 -- uptime
sshc scp -l ./dist -r /opt/app/dist devhost
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
sshc log devhost --tail 20
```

## 命令概览

```text
sshc add             新增或更新主机
sshc list|ls         查看已保存主机
sshc cfg|config      管理本地配置
sshc auth|cred       管理可复用凭证
sshc host|hosts      管理主机
sshc run|exec        执行远程命令
sshc batch-run|brun  在多台主机上执行命令或脚本
sshc login           打开交互式 SSH shell
sshc scp|upload      上传文件或目录
sshc download|dl     下载文件或目录
sshc log             查看或搜索执行日志
```

查看某个命令的完整参数：

```bash
sshc <command> --help
```

## 常用示例

### 添加主机

```bash
sshc add --ip 192.168.1.10 -u root -p password
sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 22
sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
sshc add --ip 192.168.1.10 --name devhost --auth dev-root
sshc add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
sshc add -I
sshc add --from-clipboard
```

`sshc add -I` 会交互式输入主机字段，并隐藏密码输入。

`--from-clipboard` 支持 `key=value`/`key: value` 多行格式，也支持一行 CSV：

```text
ip=192.168.1.10
user=root
password=password
name=devhost
port=22
```

```text
192.168.1.10,root,password,devhost,22
```

### 凭证配置

多台主机复用同一个用户、密码或 key 时，可以使用 `auth`：

```bash
sshc auth add dev-root -u root -p --remark "共享 root 登录"
sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519
sshc auth list
sshc auth show dev-root
sshc auth rm old-profile --yes
```

`sshc auth add -p` 会隐藏读取密码，不支持 `-p secret` 或
`--password secret` 这种命令行明文密码。

把凭证绑定到主机：

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
```

### 管理主机

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
sshc host add --name lxc-app --backend command_proxy --via pve-host --run-template "pct exec 101 -- sh -lc {{cmd}}" --login-command "pct enter 101"
sshc host list --group testing --show-ip
sshc host list --match devhost
sshc host show devhost
sshc host rm devhost --yes
sshc host rename old-name new-name
```

顶层 `add`、`list`、`ls` 仍保留，方便日常快速使用。

### 导入主机

```bash
sshc host import -f ips.txt --format ips --auth dev-root --group testing --yes
sshc host import -f hosts.txt --format plain --dry-run
sshc host import -f hosts.csv --format csv --overwrite --yes
sshc host import --from-clipboard --format plain --auth dev-root
```

`ips` 是每行一个目标的简单格式：

```text
10.0.0.8
10.0.0.9
web.internal
```

`plain` 复用 `add --from-clipboard` 的 `key=value`/`key: value` 写法。
多个主机之间使用空行分隔：

```text
ip=10.0.0.8
name=devhost
auth=dev-root
group=testing

ip: 10.0.0.9
name: dbhost
user: root
password: secret
group: testing
```

CSV 导入必须包含 header：

```csv
name,ip,auth,group,remark,port
devhost,10.0.0.8,dev-root,testing,app server,22
```

默认遇到冲突会失败且不保存。使用 `--skip-existing` 跳过已存在主机，
或使用 `--overwrite` 覆盖更新。`--dry-run` 只预览计划。导入的密码会在保存前加密，
不会打印到输出里。

### 查看主机

```bash
sshc list
sshc ls
sshc list --show-ip
```

`sshc list` 会显示主机名、分组、地址、认证方式和备注。IPv4 地址默认会脱敏显示，
例如 `10.*.*.8`。需要完整地址时使用 `--show-ip`。

如果 `~/.ssh/config` 中的主机同时配置了 `HostName`、`User` 和 `IdentityFile`，
也会被读取展示。

### 执行远程命令

```bash
sshc run 192.168.1.10 -- uptime
sshc run devhost -- docker ps
sshc run devhost --cwd /opt/app -- python -m app
sshc run devhost --timeout 30s --kill-after 5s -- systemctl status nginx
sshc run devhost -e APP_ENV=prod -e DEBUG=1 -- printenv APP_ENV
sshc run devhost --efile ./remote.env -- env
```

远程命令必须放在 `--` 后面。

环境变量文件支持注释、空行、普通 `KEY=value`，以及 `export KEY=value`：

```text
APP_ENV=prod
export DEBUG=1
NAME="hello world"
```

### 跳板机

目标主机通常需要经过堡垒机访问时，可以在目标 host 上设置 `jump`：

```bash
sshc host add --ip 1.2.3.4 --name bastion --auth dev-root
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
```

等价配置：

```json
{
  "hosts": [
    {
      "name": "bastion",
      "ip": "1.2.3.4",
      "auth_ref": "dev-root"
    },
    {
      "name": "inner-db",
      "ip": "10.0.0.8",
      "auth_ref": "dev-root",
      "jump": "bastion"
    }
  ]
}
```

之后可以像普通主机一样使用目标 host：

```bash
sshc run inner-db -- hostname
sshc login inner-db
sshc scp -l app.jar -r /tmp/app.jar inner-db
sshc download -r /var/log/app.log -l tmp/logs inner-db
```

也可以使用 `--jump` 临时覆盖当前命令使用的跳板机：

```bash
sshc run inner-db --jump bastion -- hostname
sshc login inner-db --jump bastion
sshc scp -l app.jar -r /tmp/app.jar inner-db --jump bastion
sshc download -r /var/log/app.log -l tmp/logs inner-db --jump bastion
```

初版只支持一级跳板。暂不支持多级跳板、`ProxyCommand` 和嵌套跳板链路。
jump host 和 target host 都会在本机执行 host key 校验。

### Command Proxy 逻辑主机

当目标不是可直接 SSH 的主机，而是 PVE LXC、Docker container 或 vhost 这类逻辑
目标时，可以使用 `command_proxy`。sshc 会连接 `via` 指向的真实 SSH 主机，渲染
`run_template` 后执行，并把日志仍记录到逻辑主机名下。

```bash
sshc host add --ip 192.168.1.20 --name pve-host --auth dev-root
sshc host add --name lxc-app \
  --backend command_proxy \
  --via pve-host \
  --run-template "pct exec 101 -- sh -lc {{cmd}}" \
  --login-command "pct enter 101" \
  --group lxc \
  --remark "PVE CT 101"

sshc run lxc-app -- hostname
sshc run lxc-app --cwd /opt/app -e APP_ENV=prod -- ./init.sh
sshc batch-run --hosts devhost,lxc-app -- uptime
sshc login lxc-app
```

等价配置：

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

`{{cmd}}` 会替换为 sshc 构造出的最终命令，并且会先做 shell quote；`--cwd`、
`--env`、`--sudo` 和 timeout 等选项会在替换前进入最终命令。
`login_command` 是完整交互命令，会在 `via` 主机的 PTY session 中执行。

`command_proxy` 不是 OpenSSH `ProxyCommand`：它不会代理 TCP stream，逻辑目标
也不需要 sshd。初版 command_proxy 支持 `run`、`batch-run` 和 `login`。
`scp/upload/download` 对 command_proxy 主机会返回明确的不支持错误。
`run --script` 暂不支持 command_proxy，因为临时脚本会上传到 `via` 主机，而不是
逻辑目标内部。

### 执行脚本

```bash
sshc run devhost --script ./deploy.sh
sshc run devhost --script ./deploy.sh --cwd /opt/app
sshc run devhost --script ./deploy.sh --remote-script-dir /opt/app/tmp
sshc run devhost --script ./deploy.sh --keep-remote-script
```

多行 shell、here-doc、`source` 或虚拟环境激活、复杂引号拼接等场景建议使用
`--script`。

脚本模式默认将本地文件上传到远端 `/tmp`，然后用 `bash` 执行。如果远端
`/tmp` 有 noexec、权限或清理策略限制，可以使用 `--remote-script-dir` 指定
远端临时脚本目录。

### 批量执行

```bash
sshc batch-run --hosts devhost,web-2 -- uptime
sshc brun --hosts devhost,web-2 -- hostname
sshc batch-run --group testing --parallel 5 --script ./deploy.sh
sshc batch-run --hosts-file hosts.txt -- hostname
sshc batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh
```

`--hosts` 接收逗号分隔的主机列表。`--hosts-file` 每行读取一个目标，忽略空行和
整行注释。已保存的 host 会优先解析；未保存的 IP 或 hostname 可以配合 `--auth`、
`-u`、`--key` 或 `-p` 等共享认证参数临时执行，不会写入配置。

使用 `--parallel` 控制并发数。设置 `--fail-fast` 后，遇到首个失败会停止启动新的
host，并等待已经运行中的 host 结束。

### sudo

```bash
sshc run devhost --sudo -- apt-get update
sshc run devhost --sudo-user app --cwd /opt/app -- whoami
sshc run devhost --script ./deploy.sh --sudo-user app --remote-script-dir /opt/app/tmp
```

`--sudo` 和 `--sudo-user` 需要远端支持免密 sudo，或当前 SSH 用户已经具备对应权限。

### 上传文件

```bash
sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost --sha256
sshc scp -l ./local-dir -r /tmp/remote-dir devhost
sshc scp -l ./dist -r /opt/app/dist devhost --remove-dir
sshc scp -l "./dist/*.jar" -r /opt/app/lib devhost
sshc scp -l ./a.jar -l ./b.jar -r /opt/app/lib/ devhost
sshc scp --map ./config/app.yml=/etc/app/app.yml --map ./scripts/deploy.sh=/opt/app/deploy.sh devhost
```

多个本地路径上传到同一个远端目录时，使用可重复的 `-l/--local`。如果每个本地
路径都需要指定明确的远端目标，使用可重复的 `--map local=remote`。

### 下载文件

```bash
sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost
sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost --sha256
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
sshc dl -r /tmp/remote-dir -l ./local-dir devhost
```

如果本地目标路径是已存在目录，会自动使用远端 base name。以 `/` 或 `\` 结尾的
本地路径也会被当作目录处理。

### 查看日志

```bash
sshc log
sshc log devhost
sshc log devhost --match uptime
sshc log devhost --tail 50
sshc log devhost -m error --tail 50
sshc log --id 20260704-173012-a1b2c3
sshc log --id 20260704-173012-a1b2c3 --tail 80
sshc log --id 20260704-173012-a1b2c3 --lines 120,180
sshc log devhost --lines 20,80
```

每次 `run` 默认都会向 `~/.config/sshc/logs/<host>.log` 写入一行 JSON 日志。
每次执行都有 `task_id`。短输出会内联在 JSONL 中，较大的输出会保存到
`~/.config/sshc/logs/yyyyMMdd/<task_id>.out.log`，可以通过
`sshc log --id <task_id>` 查看。
交互式 `login` 只记录连接元信息。

### 交互式登录

```bash
sshc login
sshc login devhost
sshc connect devhost
sshc login --term xterm-256color devhost
sshc login lxc-app
```

`login` 和 `connect` 会打开交互式远端 PTY。终端类型默认读取本地 `TERM`，
没有时回退到 `xterm-256color`。
不传目标，或者目标匹配到多个主机时，`sshc` 会打开交互式主机选择器。

## 主机匹配

大多数命令都支持保存的主机名或 IP。`sshc` 会优先使用精确匹配，然后在主机名、
IP、备注和分组中查找唯一的模糊匹配。

例如某个主机名是 `testing-web`，分组是 `testing`，备注是 `gpu runner`，
在匹配结果唯一时，可以这样使用：

```bash
sshc run "testing web" -- hostname
sshc run "testing gpu" -- uptime
```

如果匹配到多个主机，`sshc` 会返回候选列表，而不会自行猜测。

## 配置

默认配置文件：

```text
~/.config/sshc/sshc.config.json
```

配置示例：

```json
{
  "version": 1,
  "logs_path": "logs",
  "defaults": {
    "user": "root",
    "port": 22,
    "connect_timeout": "20s",
    "remote_script_dir": "/tmp",
    "host_key_check": "known_hosts",
    "known_hosts_path": "~/.ssh/known_hosts"
  },
  "auth_profiles": [
    {
      "name": "dev-root",
      "user": "root",
      "password_enc": "v1:...",
      "remark": "共享 root 登录"
    }
  ],
  "hosts": [
    {
      "name": "devhost",
      "ip": "192.168.1.10",
      "auth_ref": "dev-root",
      "group": "testing"
    },
    {
      "name": "inner-db",
      "ip": "10.0.0.8",
      "auth_ref": "dev-root",
      "jump": "devhost",
      "group": "testing"
    },
    {
      "name": "lxc-app",
      "backend": "command_proxy",
      "via": "devhost",
      "run_template": "pct exec 101 -- sh -lc {{cmd}}",
      "login_command": "pct enter 101",
      "group": "lxc"
    }
  ]
}
```

`logs_path` 可以是绝对路径、`~` 开头的路径，或相对于 `~/.config/sshc` 的路径。

默认执行日志目录：

```text
~/.config/sshc/logs/<host>.log
```

使用其他配置文件：

```bash
SSHC_CONFIG=/path/to/sshc.config.json sshc list
```

当保存的主机和 `~/.ssh/config` 读取到的主机同名或同 IP 时，保存的主机优先。
兼容旧版本：如果新的默认配置文件不存在，仍会读取 `~/.config/sshc/hosts.json`。

### 配置导入导出

使用 `cfg export/import` 可以把完整 sshc 配置迁移到另一台机器：

```bash
sshc cfg export -o sshc-export.enc
sshc cfg import -f sshc-export.enc --key "sshc-v1:..."
sshc cfg import -f sshc-export.enc --key "sshc-v1:..." --overwrite
sshc cfg import -f sshc-export.enc --key "sshc-v1:..." --replace
```

`cfg export` 会写入加密导出包，并打印一次性 export key。请单独保存这个 key；
它不会写入导出文件或本地配置。

`cfg import` 写入前会先备份当前配置。默认 `merge` 策略遇到同名 host、同 IP
host 或同名 auth profile 会拒绝导入。需要覆盖冲突条目时使用 `--overwrite`，
需要用导入配置整体替换当前配置时使用 `--replace`。

导出包中的密码在目标机器保存时，会使用目标机器本地的 `~/.config/sshc/key`
重新加密。导入普通 IP 列表、CSV 文件或粘贴的主机片段请使用 `sshc host import`，
不要使用 `sshc cfg import`。

配置辅助命令：

```bash
sshc cfg path
sshc cfg show
sshc cfg show --raw
sshc cfg get logs_path
sshc cfg set logs_path ./runtime/logs
sshc cfg set defaults.user root
sshc cfg set defaults.port 2222
sshc cfg set defaults.host_key_check known_hosts
sshc cfg unset logs_path
sshc cfg doctor
```

`cfg show` 会隐藏密码和加密密码字段。`cfg show --raw` 会打印磁盘上的原始配置，
主要用于本地排障。

## 安全说明

- 保存的密码会先加密再写入 `sshc.config.json`。
- 本地加密密钥保存在 `~/.config/sshc/key`，请同时保护好 key 文件和 hosts 文件。
- 旧版本中的明文 `password` 字段仍可读取，用于兼容已有配置。
- 尽量优先使用 SSH key，而不是密码。
- 如果同时提供密码和 `--key`，会优先尝试 key 认证。
- 默认会使用 `~/.ssh/known_hosts` 校验 SSH host key。
- 如果目标主机还未建立信任，先使用 `ssh devhost` 连接一次，或手动把 host key
  加入 `known_hosts`。
- 只有显式把 `host_key_check` 设置为 `insecure` 时才会跳过 host key 校验。
- 使用 `--script --sudo-user` 时，上传的远端临时脚本会对远端本机用户可读，
  这样目标 sudo 用户才能执行。脚本包含敏感内容时，建议配合 `--remote-script-dir`
  使用权限受控的远端目录。
- `login` 会打开交互式 PTY。会话日志只记录连接元信息，不记录输入命令或终端输出。

## 文档

- [部署常用示例](docs/deploy-examples.md)
- [密码加密设计](docs/password-encryption-design.md)

## 开发

```bash
go test ./...
go build -o tmp/sshc ./cmd/sshc
```

Windows：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
```

Release 构建由 tag 触发：

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT
