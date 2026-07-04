# sshc

[English](README.md)

`sshc` 是一个轻量 SSH 辅助 CLI，用于管理主机、执行远程命令、传输文件、在远端执行本地脚本，并按主机保存执行日志。

它适合轻量部署、问题排查和日常远程运维。对于这些场景，引入完整自动化平台通常过重，`sshc` 提供的是更直接的命令行工作流。

## 功能特性

- 在 `~/.config/sshc/sshc.config.json` 中管理 SSH 主机
- 从 `~/.ssh/config` 读取简单主机配置
- 保存主机密码前会先加密，避免明文写入 `sshc.config.json`
- 通过主机名、IP 或唯一的模糊匹配结果执行远程命令
- 将本地 shell 脚本上传到远端执行
- 支持远端工作目录、超时、环境变量、sudo 和 sudo user
- 通过 SFTP 上传和下载文件或目录
- 支持单文件传输 SHA256 校验
- 在 `~/.config/sshc/logs/` 下按主机保存 JSONL 执行日志
- 通过 `login/connect` 打开交互式远端 PTY

## 安装

### 下载 Release

1. **Recommended** 通过 [eget](https://github.com/inherelab/eget) 下载安装: `eget install sshc`
2. 通过 Golang 安装: `go install github.com/inhere/sshc/cmd/sshc@latest`
3. 从 GitHub Releases 下载对应平台的归档文件，解压后将 `sshc` 二进制放到 `PATH` 中。

### 从源码构建

```bash
git clone https://github.com/inhere/sshc.git
cd sshc
go build -o sshc ./cmd/sshc
```

Windows 本地开发时，可以构建到 `tmp`：

```powershell
go build -o tmp\sshc.exe ./cmd/sshc
```

## 快速开始

```bash
sshc add --ip 192.168.1.10 --name devhost -u root -p password
sshc list
sshc run devhost -- uptime
sshc run devhost --script ./deploy.sh
sshc scp -l ./dist -r /opt/app/dist devhost
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
sshc log devhost --tail 20
```

## 命令概览

```text
sshc add             新增或更新主机
sshc list|ls         查看已保存主机
sshc run|exec        执行远程命令
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
sshc add -I
sshc add --from-clipboard
```

`sshc add -I` 会交互式输入主机字段，并隐藏密码输入。

`--from-clipboard` 支持 `key=value` 多行格式，也支持一行 CSV：

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
```

每次 `run` 默认都会向 `~/.config/sshc/logs/<host>.log` 写入一行 JSON 日志。
交互式 `login` 只记录连接元信息。

### 交互式登录

```bash
sshc login devhost
sshc connect devhost
sshc login --term xterm-256color devhost
```

`login` 和 `connect` 会打开交互式远端 PTY。终端类型默认读取本地 `TERM`，
没有时回退到 `xterm-256color`。

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
  "logs_path": "logs",
  "hosts": []
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

## 安全说明

- 保存的密码会先加密再写入 `sshc.config.json`。
- 本地加密密钥保存在 `~/.config/sshc/key`，请同时保护好 key 文件和 hosts 文件。
- 旧版本中的明文 `password` 字段仍可读取，用于兼容已有配置。
- 尽量优先使用 SSH key，而不是密码。
- 如果同时提供密码和 `--key`，会优先尝试 key 认证。
- 当前 host key 校验是宽松模式，不强制校验 `known_hosts`。高安全要求环境不建议
  直接使用当前实现。
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
