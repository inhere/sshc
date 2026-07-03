# sshc 部署常用示例

本文记录常见部署任务的 `sshc` 命令组合。示例主机统一使用 `devhost`。

## 检查主机

```bash
sshc run devhost -- hostname
sshc run devhost -- uptime
sshc run devhost -- "pwd && whoami"
```

## 上传部署包

上传单个文件并校验 sha256：

```bash
sshc scp -l tmp/pkg.tar.gz -r /tmp/pkg.tar.gz devhost --sha256
```

上传构建目录，上传前清理远端目录：

```bash
sshc scp -l ./dist -r /opt/app/dist devhost --remove-dir
```

上传多个 jar 文件：

```bash
sshc scp -l "./dist/*.jar" -r /opt/app/lib devhost
```

## 解压和准备目录

```bash
sshc run devhost -- "mkdir -p /opt/app && tar -xzf /tmp/pkg.tar.gz -C /opt/app"
```

如需在目标目录下执行命令：

```bash
sshc run devhost --cwd /opt/app -- "ls -lah && ./bin/app --version"
```

## 使用脚本部署

复杂部署逻辑建议放在本地脚本中，再通过 `--script` 上传执行，避免多层 shell 转义。

```bash
sshc run devhost --script ./scripts/deploy.sh --cwd /opt/app
```

调试脚本时保留远端临时脚本：

```bash
sshc run devhost --script ./scripts/deploy.sh --keep-remote-script
```

## sudo 和执行用户

需要 root 权限时：

```bash
sshc run devhost --sudo -- systemctl restart app.service
```

切换到应用用户执行：

```bash
sshc run devhost --sudo-user app --cwd /opt/app -- ./bin/app migrate
```

`--sudo` 和 `--sudo-user` 需要远端支持免密 sudo，或当前 SSH 用户已经是 root。

## 超时控制

给远端命令设置超时，并在超时后等待 30 秒再强制清理：

```bash
sshc run devhost --timeout 600s --kill-after 30s -- ./scripts/upgrade.sh
```

远端主机需要提供 `timeout` 命令，一般来自 coreutils。

## 查看服务状态

```bash
sshc run devhost -- "systemctl status app.service --no-pager"
sshc run devhost -- "systemctl is-active app.service"
```

## 查看日志

```bash
sshc run devhost -- "journalctl -u app.service --no-pager -n 100"
sshc run devhost -- "tail -n 100 /var/log/app/app.log"
```

## 健康检查

```bash
sshc run devhost -- "curl -fsS http://127.0.0.1:18080/health"
```

## 下载远端日志

```bash
sshc download -r /var/log/app/app.log -l tmp/app.log devhost --sha256
```

下载整个日志目录：

```bash
sshc dl -r /var/log/app -l tmp/app-logs devhost
```
