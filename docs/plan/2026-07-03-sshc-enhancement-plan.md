# sshc 使用反馈增强计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-03 | Codex | 根据 `tmp/sshc-enh.md` 整理增强范围与实施顺序 |

## 背景

`tmp/sshc-enh.md` 记录了在部署 YLPY 服务时对 `sshc run`、`sshc scp` 的真实使用反馈。当前版本已经具备基础远程执行、上传、下载、日志、timeout、env/env-file 能力，但部署类任务仍存在几个高频痛点：

- 复杂命令在本地 PowerShell、sshc 参数解析、远端 shell 之间需要多层转义。
- 客户端侧 timeout 可能导致本地命令结束，但远端长进程仍继续运行。
- 部署命令经常需要 `cwd`、sudo/user 切换、上传后校验等明确能力。
- README 和 help 已覆盖基础示例，但缺少面向部署场景的连续示例。

本计划只整理要做的增强项和建议顺序，暂不开始代码实现。

## 当前状态

已覆盖项：

- 顶层 `sshc --help` 已列出 `scp/upload`、`download/dl`、`log`、`run/exec`。
- `download/dl` 已支持远程文件/目录下载。
- `run` 已支持 `--timeout`、`-e/--env`、`--env-file/--efile`。
- `run` 已记录 JSON 执行日志，日志时间已统一为毫秒精度且无时区后缀。

仍需增强项：

- `run --script`，降低复杂脚本执行的转义成本。
- `run --cwd`，减少 `cd ... && ...` 拼接。
- `run --timeout` 需要远端 `timeout` 包装，避免远端长进程残留。
- `run --sudo` / `--sudo-user`，明确表达提权或切换执行用户。
- `scp/download --sha256` 和 size/elapsed 输出，增强大文件传输可验证性。
- 部署示例文档。
- 后续可选的 `run --json`、`run --stdin`。

## 设计原则

- 保持现有命令兼容：已有 `sshc run devhost -- command`、`scp`、`download` 用法不破坏。
- 命令层只负责参数和输出，远端命令构造、脚本上传、路径处理放在 `internal/core`。
- 优先解决部署任务中直接浪费时间的问题，避免一次性加入过多低频选项。
- 对 Linux 远端主机优先，Windows 远端 shell 不作为当前目标。
- 出错时返回清晰错误，不做静默降级，尤其是远端缺少 `timeout`、`sha256sum`、`sudo` 的场景。

## 阶段计划

### P1: run 脚本模式和 cwd

目标：

- 新增 `run --script FILE`，支持把本地 shell 脚本上传到远端临时路径并执行。
- 新增 `run --keep-remote-script`，用于调试时保留远端临时脚本。
- 新增 `run --cwd DIR`，让普通命令和脚本模式都能在指定远端目录执行。

命令面：

```bash
sshc run devhost --script ./deploy.sh
sshc run devhost --script ./deploy.sh --cwd /opt/ylpy/app
sshc run devhost --script ./deploy.sh --keep-remote-script
sshc run devhost --cwd /opt/ylpy/app -- python -m app
```

建议语义：

- `--script` 和普通 `command after --` 二选一，不能同时为空，也不建议同时设置。
- 本地读取脚本后通过 SFTP 上传到 `/tmp/sshc-run-{timestamp}-{rand}.sh`。
- 上传后远端执行 `chmod 700`，再执行 `bash <remote-script>`。
- 默认执行完成后删除远端临时脚本。
- `--keep-remote-script` 时不删除，并在本地输出或日志里记录远端脚本路径。
- `--cwd` 通过 `cd '<cwd>' && ...` 包装普通命令或脚本执行命令。

日志字段建议：

```json
{
  "script": "./deploy.sh",
  "remote_script": "/tmp/sshc-run-20260703-xxx.sh",
  "keep_remote_script": false,
  "cwd": "/opt/ylpy/app"
}
```

实现要点：

- `internal/command/run.go` 增加 flags，命令层只校验互斥关系和必填关系。
- `internal/core` 增加脚本上传/执行辅助逻辑。
- 复用现有 SFTP 连接能力，避免新建重复 SSH/SFTP 实现。
- shell quote 统一走已有或扩展后的 quote 方法。

验收：

- `sshc run devhost --script ./a.sh` 能执行本地脚本内容。
- `--keep-remote-script` 保留远端脚本并输出路径。
- 未设置 `--keep-remote-script` 时远端脚本被清理。
- `--cwd` 对普通命令和脚本模式都生效。
- `go test ./...`、`go build -o tmp/sshc.exe ./cmd/sshc` 通过。

### P2: 远端 timeout 清理长进程

目标：

- 调整 `run --timeout`，让超时限制作用在远端命令进程上，而不仅是 SSH 客户端等待。
- 新增 `--kill-after`，用于超时后等待进程优雅退出的时间。

命令面：

```bash
sshc run devhost --timeout 600s -- pip install ...
sshc run devhost --timeout 600s --kill-after 30s -- pip install ...
```

建议语义：

- `--timeout` 继续接受 Go duration 和裸秒数，保持兼容。
- 远端命令包装为：

```bash
timeout --kill-after=30s 600s bash -lc '<wrapped-command>'
```

- 客户端 SSH context timeout 设置为 `timeout + kill_after + buffer`，buffer 可先取 5s。
- 如果远端没有 `timeout` 命令，返回明确错误，提示安装 coreutils 或移除 `--timeout`。

实现要点：

- 远端 timeout 包装放在 `internal/core` 的命令构造流程。
- `--script`、`--cwd`、`--env`、普通命令应共用同一套最终命令构造顺序。
- 注意包装顺序建议：

```text
timeout -> sudo/user -> cwd -> env -> command/script
```

实际实现时可通过测试固定最终远端命令字符串。

验收：

- 超时命令返回非 0。
- 远端长进程不会在客户端超时后继续残留。
- `--timeout 5` 仍按 5s 解释。
- `--kill-after 30` 按 30s 解释。

### P3: sudo 和执行用户切换

目标：

- 支持部署任务中常见的 sudo 和用户切换表达，减少手写 `sudo bash -lc` 或 `runuser`。

命令面：

```bash
sshc run devhost --sudo -- apt-get update
sshc run devhost --sudo-user ylpy --cwd /opt/ylpy/app -- python -m app
```

建议语义：

- `--sudo` 表示通过 sudo 执行当前命令。
- `--sudo-user USER` 表示通过 `sudo -u USER` 执行当前命令。
- `--sudo` 与 `--sudo-user` 同时出现时，以 `--sudo-user` 为更具体表达，或直接判定为冲突；建议判定冲突，减少歧义。
- 明确文档说明：当前只支持免密 sudo 或 root 登录下可用的 sudo；需要交互密码/TTY 的 sudo 不作为当前目标。

实现要点：

- 使用 `sudo bash -lc '<cmd>'` 和 `sudo -u '<user>' bash -lc '<cmd>'` 包装。
- 用户名做基本校验，避免空值和明显危险字符。
- 与 `--cwd`、`--env`、`--script`、`--timeout` 组合测试。

验收：

- `--sudo` 构造的远端命令符合预期。
- `--sudo-user ylpy` 构造的远端命令符合预期。
- sudo 不可用时错误能透传。

### P4: scp/download 传输信息和 sha256 校验

目标：

- 上传/下载输出 size、elapsed，方便部署任务确认传输规模和耗时。
- 可选启用 sha256 校验，验证大文件传输结果。

命令面：

```bash
sshc scp -l model.pt -r /data/Models/model.pt devhost --sha256
sshc download -r /var/log/app.log -l tmp/app.log devhost --sha256
```

输出建议：

```text
uploaded model.pt to devhost:/data/Models/model.pt
size=123456789
elapsed=12.3s
sha256.local=...
sha256.remote=...
sha256.ok=true
```

建议范围：

- 文件上传/下载支持 sha256。
- 目录上传/下载先输出文件数、总大小、elapsed。
- 目录级 sha256 暂缓，避免定义复杂的跨平台目录 hash 规则。

实现要点：

- 本地 hash 使用 Go `crypto/sha256`。
- 远端 hash 通过 `sha256sum <path>` 获取。
- 远端缺少 `sha256sum` 时返回明确错误。
- 路径 quote 必须复用统一 shell quote。

验收：

- 文件上传/下载输出 size 和 elapsed。
- `--sha256` 成功时输出本地/远端 hash 和 ok=true。
- hash 不一致时命令失败并输出明确错误。

### P5: 部署示例文档

目标：

- 增加一份面向部署任务的示例文档，减少 agent 和人工使用时绕路。

建议文件：

```text
docs/deploy-examples.md
```

内容建议：

- 主机信息检查。
- 上传部署包。
- 解压部署包。
- 使用 `--script` 执行部署脚本。
- 查看 systemd 状态。
- 查看 journal 日志。
- 执行 health check。
- 下载远端日志。

示例：

```bash
sshc run devhost -- hostname
sshc scp -l tmp/pkg.tar.gz -r /tmp/pkg.tar.gz devhost
sshc run devhost -- "mkdir -p /opt/app && tar -xzf /tmp/pkg.tar.gz -C /opt/app"
sshc run devhost --script ./scripts/deploy.sh --cwd /opt/app
sshc run devhost -- "systemctl status ylpy-cv-http --no-pager"
sshc run devhost -- "journalctl -u ylpy-cv-http --no-pager -n 100"
sshc run devhost -- "curl -sS http://127.0.0.1:18080/health"
sshc download -r /var/log/ylpy/cv-http.log -l tmp/cv-http.log devhost
```

验收：

- README 中增加到部署示例文档的链接。
- `sshc help run` 中简要提示复杂部署优先使用 `--script`。

## 暂缓项

### run --stdin

原因：

- Windows/PowerShell 和不同 shell 下 stdin 重定向行为差异更大。
- `--script FILE` 能覆盖主要部署场景，且更容易复现和记录。

建议等 `--script` 稳定后再评估。

### run --json

原因：

- 当前 `run` 直接输出远端 stdout，改为 JSON 需要明确 stdout/stderr 分离、exit code、错误返回和日志关系。
- 更适合在执行模型稳定后单独设计。

建议后续命令面：

```bash
sshc run devhost --json -- systemctl is-active app
```

### scp 多文件和 glob

原因：

- Windows glob、逗号分隔、远端目录规则、多个失败时的错误模型都需要更完整设计。
- 当前单文件和目录已覆盖主要部署包传输场景。

建议在 sha256/elapsed 输出之后再做。

## 建议实施顺序

1. P1: `run --script`、`--keep-remote-script`、`--cwd`
2. P2: 远端 `timeout` 包装、`--kill-after`
3. P3: `--sudo`、`--sudo-user`
4. P4: `scp/download --sha256`、size、elapsed 输出
5. P5: 部署示例文档
6. 暂缓项按后续使用反馈再排期

## 待确认事项

- `--script` 是否只支持 bash 脚本，还是需要 `--shell sh|bash`。建议初版固定 bash。
- `--timeout` 是否允许在远端缺少 `timeout` 时回退到客户端 timeout。建议不回退，避免用户误以为远端进程已清理。
- `--sudo` 与 `--sudo-user` 同时出现时是否判定冲突。建议判定冲突。
- `--sha256` 是否覆盖目录。建议初版不覆盖目录。
