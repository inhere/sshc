# sshc 脚本执行反馈改进计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-03 | Codex | 根据 `tmp/sshc-enh-2.md` 整理下一轮脚本执行与部署排障改进计划 |

## 背景

`tmp/sshc-enh-2.md` 记录了另一个 Codex 在使用新版 `sshc` 部署 parsedoc worker 时的真实反馈。当前 `scp`、`run --cwd`、`run --script`、`run --timeout`、`download` 已经能覆盖主要部署动作，但实际使用暴露出一个高优先级缺陷：

- `sshc run --script --sudo-user <user>` 会先把脚本上传到 `/tmp/sshc-run-*.sh`，再通过 `sudo -u <user> bash -lc 'bash /tmp/sshc-run-*.sh'` 执行。
- 当前远端脚本上传后会 `chmod 700`，当 SSH 登录用户和 `--sudo-user` 目标用户不一致时，目标用户无法读取该脚本，导致 `Permission denied` / exit 126。

除此之外，反馈还提出了几个部署排障可用性增强：

- 支持自定义远端脚本临时目录，避开 `/tmp` 的挂载、权限或清理策略。
- 失败时直接显示远端脚本路径、目标执行用户等上下文，减少查日志成本。
- 在 help/examples 中更明确地区分短命令用 `--cwd`、复杂 shell 用 `--script`。
- 为 `download` 补充拉取日志场景示例。

本计划只覆盖这轮实际反馈中和脚本执行、部署排障直接相关的改进。`scp/download --json` 有价值，但涉及结构化输出模型和多文件明细设计，本轮暂缓。

## 当前状态

已具备能力：

- `run --script FILE`：上传本地脚本到远端临时路径并执行。
- `run --keep-remote-script`：保留远端临时脚本。
- `run --cwd DIR`：在指定远端目录下执行命令或脚本。
- `run --timeout` / `--kill-after`：通过远端 `timeout` 包装命令。
- `run --sudo` / `--sudo-user USER`：通过 sudo 提权或切换用户执行。
- `scp/upload`：支持文件/目录上传、glob、多 `-l`、`--map`、`--sha256`。
- `download/dl`：支持文件/目录下载和单文件 sha256 校验。

已知问题：

- `run --script --sudo-user USER` 上传脚本默认 `0700`，切换用户后不可读。
- `run --script` 远端临时目录固定在 `/tmp`。
- 执行失败时，用户需要去日志里找 `remote_script` 等排障信息。
- `run` help 对复杂 shell 更适合 `--script` 的提示还不够明确。
- `download` help 缺少拉日志场景示例。

## 设计原则

- 优先修复真实部署中已出现的失败场景。
- 保持现有命令兼容，不改变已有 `run --script`、`--cwd`、`--sudo-user` 行为语义。
- 命令层负责参数、输出和失败提示；远端路径生成、脚本上传和执行命令构造放在 `internal/core`。
- 对脚本权限采取最小可理解规则：普通脚本仍偏私有；切换用户执行时保证目标用户能读取脚本。
- 不在本轮引入复杂权限管理、chown、ACL 或交互式 sudo 密码输入。
- 每个阶段完成后单独验证和提交，提交信息使用 Conventional Commits。

## 范围

本轮纳入：

- 修复 `--script --sudo-user` 目标用户不可读脚本的问题。
- 新增 `run --remote-script-dir DIR`。
- 失败时输出脚本排障上下文。
- 更新 `run` / `download` help、README 或部署示例文档中的相关说明。

本轮暂缓：

- `scp/download --json`。
- `run --json`。
- `run --stdin`。
- 远端脚本 owner/chown 策略。
- 交互式 sudo 密码输入。
- Windows 远端 shell 支持。

## 阶段计划

### P1: 修复 `--script --sudo-user` 脚本权限

目标：

- `sshc run host --script file.sh --sudo-user app` 不再因为远端临时脚本 `0700` 而被目标用户拒绝读取。

建议语义：

- 普通 `run --script` 继续上传后设置为 `0700`。
- 当 `RunOptions.SudoUser` 非空时，远端脚本设置为 `0644`。
- 脚本执行方式仍是 `bash <remote-script>`，不依赖 execute bit。
- `--sudo` 但不指定用户时，仍按 root/当前 sudo 语义处理，可继续使用 `0700`。

实现要点：

- 在 `internal/core` 中集中决定脚本 chmod mode，例如新增 `remoteScriptMode(opts RunOptions) string`。
- `executeRemoteScript` 中把固定的 `chmod 700` 改为根据 options 决定。
- 不引入 chown。因为 chown 需要 root 权限，且跨主机权限模型复杂。
- 在 help 或 docs 中提示：`--script --sudo-user` 会让远端临时脚本对本机用户可读，敏感脚本可结合 `--remote-script-dir` 和远端目录权限控制。

验收：

- 单测覆盖：
  - 无 `--sudo-user` 时 chmod 为 `700`。
  - 有 `--sudo-user` 时 chmod 为 `644`。
  - 脚本仍通过 `bash <remote-script>` 执行。
- `go test ./...` 通过。
- `go build -o tmp/sshc.exe ./cmd/sshc` 通过。

阶段提交：

```text
fix: allow sudo-user script execution
```

### P2: 新增 `run --remote-script-dir`

目标：

- 允许用户指定远端临时脚本目录，避开 `/tmp` 的权限、挂载或清理策略。

命令面：

```bash
sshc run devhost --script ./deploy.sh --remote-script-dir /opt/app/tmp
sshc run devhost --script ./check.sh --remote-script-dir /opt/app/tmp --sudo-user app
```

建议语义：

- 未设置时保持现有路径：`/tmp/sshc-run-{timestamp}-{rand}.sh`。
- 设置后生成：`<remote-script-dir>/sshc-run-{timestamp}-{rand}.sh`。
- `--remote-script-dir` 只对 `--script` 生效；未设置 `--script` 时如果指定该参数，应报错。
- 远端目录不存在时，沿用当前 `mkdirRemoteParent` 行为自动创建父目录。
- `--keep-remote-script` 时继续保留生成后的远端脚本路径。

实现要点：

- `RunOptions` 增加 `RemoteScriptDir string`。
- `runFlagOptions` 增加 `RemoteScriptDir string`。
- `buildRunOptions` 校验：
  - `RemoteScriptDir` 非空但 `Script` 为空，返回错误。
- 新增路径生成函数，例如：

```go
func NewRemoteScriptPathInDir(value time.Time, dir string) string
```

- command 层生成 `RemoteScriptPath` 时根据 `RemoteScriptDir` 选择路径。

验收：

- `--script --remote-script-dir /opt/app/tmp` 生成 `/opt/app/tmp/sshc-run-*.sh`。
- 未设置 `--script` 但设置 `--remote-script-dir` 报错。
- `--keep-remote-script` 日志里记录自定义目录下的远端路径。
- `go test ./...`、build 通过。

阶段提交：

```text
feat: add remote script dir option
```

### P3: 失败时输出脚本排障上下文

目标：

- `run --script` 执行失败时，用户不用先查日志就能看到关键排障信息。

建议输出：

```text
sshc: local_script=tmp\parsedoc-cron-check.sh
sshc: remote_script=/tmp/sshc-run-xxx.sh
sshc: sudo_user=parsedoc
sshc: use --keep-remote-script to inspect the uploaded script
```

如果能稳定识别远端 exit code，可追加：

```text
sshc: exit_status=126
```

建议语义：

- 仅在 `run --script` 失败时输出。
- 不改变命令 exit status。
- 不打印脚本内容。
- `--keep-remote-script` 已经开启时，不再提示开启该参数。
- 如果只有 `--sudo` 没有 `--sudo-user`，可输出 `sudo=true`。

实现要点：

- 在 `internal/command/run.go` 的 `runRemote` 返回后判断：
  - `err != nil`
  - `runOptions.ScriptPath != ""`
- 新增小函数，例如 `writeRunScriptFailureContext(c, runOptions, err)`。
- exit code 可先尝试 `errors.As(err, *ssh.ExitError)`；如果当前 goph 返回包装后不稳定，可先不做 exit code，避免脆弱实现。
- 失败上下文也可以进入 log 字段，但本阶段优先保证命令行可见。

验收：

- 单测用 fake `runRemote` 返回 error，断言输出包含 `local_script`、`remote_script`、`sudo_user`。
- 非 script 失败不输出这些脚本上下文。
- script 成功不输出这些脚本上下文。
- `go test ./...`、build 通过。

阶段提交：

```text
fix: print script failure context
```

### P4: 更新帮助和部署示例

目标：

- 把本轮真实使用经验沉淀到 help / docs，减少后续部署绕路。

建议改动：

- `run` LongHelp 增加复杂 shell 推荐：

```text
- Use --script for multiline shell, here-doc, source/venv activation, or heavy quoting.
```

- `run` Examples 增加 `--remote-script-dir` 示例。
- `download` Examples 增加日志拉取示例：

```bash
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
```

- `docs/deploy-examples.md` 增加：
  - 用 `--script` 执行复杂检查脚本。
  - 用 `--remote-script-dir` 避开 `/tmp` 的场景。
  - 拉取 systemd/journal 或应用日志示例。

验收：

- `sshc run --help` 显示 `--remote-script-dir` 和复杂 shell 提示。
- `sshc download --help` 包含日志下载示例。
- `go test ./...`、build 通过。

阶段提交：

```text
docs: update script and download examples
```

## 暂缓项：`scp/download --json`

反馈中的结构化输出建议是合理的，但不建议和本轮脚本执行修复一起做。

暂缓原因：

- `scp/upload` 已支持多路径、glob、`--map`，JSON 输出如果没有 per-item 明细，价值有限。
- 当前 `TransferResult` 主要是聚合字段，缺少每个文件的 local/remote/bytes/hash 明细。
- `download` 单文件、目录、多文件未来也需要统一输出结构。
- `run --json` 也可能需要统一状态字段，最好单独设计。

后续建议单独设计：

```json
{
  "status": "success",
  "target": "devhost",
  "host": "devhost",
  "bytes": 12345,
  "files": 2,
  "dirs": 0,
  "elapsed_ms": 1532,
  "sha256_ok": true,
  "items": [
    {
      "local": "a.jar",
      "remote": "/opt/app/lib/a.jar",
      "bytes": 12345,
      "sha256_local": "...",
      "sha256_remote": "..."
    }
  ]
}
```

建议未来计划：

```text
feat: add transfer result items
feat: add json output for transfers
```

## 提交与验证策略

每个阶段单独提交，不合并成一个大提交：

1. P1 完成后提交 `fix: allow sudo-user script execution`。
2. P2 完成后提交 `feat: add remote script dir option`。
3. P3 完成后提交 `fix: print script failure context`。
4. P4 完成后提交 `docs: update script and download examples`。

每阶段至少运行：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- <本阶段涉及文件>
```

涉及 help 的阶段额外运行：

```powershell
.\tmp\sshc.exe run --help | Out-String
.\tmp\sshc.exe download --help | Out-String
```

提交边界：

- 不提交当前已有的 `docs/TODO.md` 脏改，除非用户明确要求一并整理。
- 不改 `tmp/sshc-enh-2.md`，它作为反馈输入保留。
- 如果实施中发现 `--script --sudo-user` 仍被远端目录权限阻断，再评估是否需要把 `--remote-script-dir` 的目录权限说明升级为运行时提示。

## 待确认事项

- `--script --sudo-user` 时脚本 mode 使用 `0644` 是否可接受；如果脚本常含敏感内容，可能需要改成“上传到指定目录后 chown/chmod”，但这要求登录用户具备 root/chown 权限。
- `--remote-script-dir` 是否允许相对路径。建议允许，但文档推荐绝对路径。
- 失败上下文是否输出到 stdout 还是 stderr。当前命令输出统一用 `c.Output()`，建议先保持一致；如后续区分 stderr，再统一调整。
- 是否需要在 P3 中解析 exit status。建议先尝试，若 goph error 包装不稳定则暂不实现。
