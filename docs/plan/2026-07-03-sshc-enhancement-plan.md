# sshc 使用反馈增强计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-03 | Codex | 根据 `tmp/sshc-enh.md` 整理增强范围与实施顺序 |
| v0.2 | 2026-07-03 | Codex | 对照 `docs/TODO.md` 补充可并入计划的主机匹配、add、scp、login/connect 项 |
| v0.3 | 2026-07-03 | Codex | 补充分阶段 Git 提交要求，避免完整实现后一次性大提交 |
| v0.4 | 2026-07-03 | Codex | 提交策略统一为 Conventional Commits 前缀格式 |

## 背景

`tmp/sshc-enh.md` 记录了在部署服务时对 `sshc run`、`sshc scp` 的真实使用反馈。当前版本已经具备基础远程执行、上传、下载、日志、timeout、env/env-file 能力，但部署类任务仍存在几个高频痛点：

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

- target 支持非完整匹配，输入片段能唯一匹配 host 时直接使用，多个匹配时提示选择或报错。
- `run --script`，降低复杂脚本执行的转义成本。
- `run --cwd`，减少 `cd ... && ...` 拼接。
- `run --timeout` 需要远端 `timeout` 包装，避免远端长进程残留。
- `run --sudo` / `--sudo-user`，明确表达提权或切换执行用户。
- `scp/download --sha256`、size、elapsed、上传前清理远端目录、多个本地文件匹配，增强文件传输能力和可验证性。
- `add` 增强：交互录入、key path、备注、分组、剪贴板导入、密码加密。
- 支持读取 `~/.ssh/config` 等已有 SSH 配置。
- `login/connect` 交互式连接。
- 部署示例文档。
- 后续可选的 `run --json`、`run --stdin`。

## 设计原则

- 保持现有命令兼容：已有 `sshc run devhost -- command`、`scp`、`download` 用法不破坏。
- 命令层只负责参数和输出，远端命令构造、脚本上传、路径处理放在 `internal/core`。
- 优先解决部署任务中直接浪费时间的问题，避免一次性加入过多低频选项。
- 对 Linux 远端主机优先，Windows 远端 shell 不作为当前目标。
- 出错时返回清晰错误，不做静默降级，尤其是远端缺少 `timeout`、`sha256sum`、`sudo` 的场景。
- 每个阶段或子阶段完成后必须验证并单独 Git 提交，不允许全部实现后一次性提交一大堆变动。

## 提交策略

实施时按“可独立验证、可独立回滚”的粒度提交：

- 每个子阶段完成后立即运行对应测试和构建。
- 每个子阶段独立提交，提交信息使用 Conventional Commits 前缀并描述该阶段的行为变化。
- 不把纯文档、命令面、核心实现、测试修复混在一个大提交里，除非变动非常小且无法合理拆分。
- 如果一个阶段中途发现设计需要调整，先更新计划文档并提交，再继续实现。
- 阶段收口时更新 `docs/TODO.md` 的完成状态并单独或随该阶段最后一个提交提交。

提交前缀约定：

| 前缀 | 使用场景 |
| --- | --- |
| `feat:` | 新增用户可见能力、命令参数、命令行为 |
| `fix:` | 修复错误行为、兼容性问题或回归 |
| `docs:` | 仅文档、README、help 文案、计划/TODO 更新 |
| `test:` | 仅测试覆盖、测试 fixture、测试辅助函数 |
| `refactor:` | 不改变行为的结构调整、包拆分、内部重命名 |
| `chore:` | 构建、依赖、仓库维护、无业务行为变化的杂项 |

建议提交模板：

```text
feat: add partial host target matching
feat: add run script execution mode
feat: add run cwd support
feat: wrap run timeout on remote host
feat: add sudo options for run command
feat: add transfer sha256 verification
docs: add sshc deploy examples
```

## TODO 纳入判断

`docs/TODO.md` 中未完成项按当前计划归类如下：

| TODO 项 | 建议处理 | 原因 |
| --- | --- | --- |
| run host 非完整匹配 | 纳入 P1 前置小阶段 | 对所有命令都有帮助，改动小，能改善日常使用 |
| add interactive | 纳入 P6 | 属于主机配置录入体验，依赖 cliui，引入新交互模式 |
| add pwd 加密 | 纳入 P6，但需要单独设计 | 涉及配置重写、密钥来源和安全边界 |
| add 从 clipboard 读取 ip/user/pwd | 纳入 P6 | 与 interactive/add 批量录入同类，需定义输入格式 |
| 支持 keypath file | 纳入 P6，优先级较高 | key 登录是 SSH 常规能力，会影响 Host/Auth 模型 |
| 新增 remark/group | 纳入 P6 | 配置模型扩展，可与 list/log/filter 后续联动 |
| 支持读取 `~/.ssh/config` | 纳入 P6 或独立 P6.5 | 需要解析 SSH config，和显式 `sshc.config.json` 的优先级要明确 |
| scp local-path 支持 glob/多个文件 | 纳入 P4 后半段 | 与传输增强相关，但路径规则和错误模型要独立设计 |
| scp `--remove-dir` | 纳入 P4 | 部署目录覆盖常用，但默认不能破坏远端目录 |
| login/connect PTY | 纳入 P7 | 需要 PTY、终端模式和日志策略，和非交互 run 差异较大 |

已完成项不再进入后续计划：run 执行日志、run timeout/env/env-file、scp 上传、download/dl、项目结构整理。

## 阶段计划

### P1: target 匹配、run 脚本模式和 cwd

目标：

- 支持 target 片段唯一匹配 saved host，降低输入完整 host name 的成本。
- 新增 `run --script FILE`，支持把本地 shell 脚本上传到远端临时路径并执行。
- 新增 `run --keep-remote-script`，用于调试时保留远端临时脚本。
- 新增 `run --cwd DIR`，让普通命令和脚本模式都能在指定远端目录执行。

命令面：

```bash
sshc run devhost --script ./deploy.sh
sshc run devhost --script ./deploy.sh --cwd /opt/ylpy/app
sshc run devhost --script ./deploy.sh --keep-remote-script
sshc run devhost --cwd /opt/ylpy/app -- python -m app
sshc run dev -- hostname
```

建议语义：

- target 先按现有精确规则匹配 name/IP。
- 精确匹配失败后，按空格拆分输入片段，所有片段都命中同一个 host 的 name/IP/remark/group 时视为候选。
- 候选唯一时使用该 host；候选多个时输出候选列表并返回错误，不做交互选择。
- 初版不做模糊编辑距离匹配，避免误连主机。
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

- target 匹配逻辑放在 `internal/core.Store`，让 run/scp/download/log 都可复用。
- `internal/command/run.go` 增加 flags，命令层只校验互斥关系和必填关系。
- `internal/core` 增加脚本上传/执行辅助逻辑。
- 复用现有 SFTP 连接能力，避免新建重复 SSH/SFTP 实现。
- shell quote 统一走已有或扩展后的 quote 方法。

验收：

- 精确 target 仍保持现有行为。
- 片段唯一匹配时能执行对应 host。
- 片段匹配多个 host 时返回候选列表，不执行远端命令。
- `sshc run devhost --script ./a.sh` 能执行本地脚本内容。
- `--keep-remote-script` 保留远端脚本并输出路径。
- 未设置 `--keep-remote-script` 时远端脚本被清理。
- `--cwd` 对普通命令和脚本模式都生效。
- `go test ./...`、`go build -o tmp/sshc.exe ./cmd/sshc` 通过。

阶段提交：

- P1.1 提交：`feat: add partial host target matching`，仅包含 `Store` 匹配逻辑、命令层接入和测试。
- P1.2 提交：`feat: add run script execution mode`，包含脚本上传、执行、清理和日志字段。
- P1.3 提交：`feat: add run cwd support`，让普通命令和脚本模式都支持工作目录。
- P1.4 提交：`docs: update run help for script and cwd`，help/README/TODO 更新，如前面提交已包含足够文档可省略。

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

阶段提交：

- P2.1 提交：`feat: add run kill-after option`，包含参数解析和 RunOptions 扩展。
- P2.2 提交：`feat: wrap run timeout on remote host`，包含远端 `timeout` 包装和客户端保护 timeout。
- P2.3 提交：`docs: update run timeout help`，help/README/TODO 更新。

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

阶段提交：

- P3.1 提交：`feat: add sudo options for run command`，包含 `--sudo` / `--sudo-user` 参数、校验和远端命令包装。
- P3.2 提交：`docs: document run sudo options`，包含组合场景测试和 help/README/TODO 更新；如果只有测试补强则使用 `test: cover run sudo option combinations`。

### P4: scp/download 传输增强和 sha256 校验

目标：

- 上传/下载输出 size、elapsed，方便部署任务确认传输规模和耗时。
- 可选启用 sha256 校验，验证大文件传输结果。
- 支持上传前移除远端目录。
- 支持多个本地文件匹配上传。

命令面：

```bash
sshc scp -l model.pt -r /data/Models/model.pt devhost --sha256
sshc download -r /var/log/app.log -l tmp/app.log devhost --sha256
sshc scp -l ./dist -r /opt/app/dist devhost --remove-dir
sshc scp -l "./dist/*.jar" -r /opt/app/lib devhost
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
- `--remove-dir` 只允许在 local 是目录时使用，执行上传前删除远端目标目录后重建。
- 多文件上传初版只支持本地 glob，不支持远端 glob。
- 多文件上传的 remote 必须是目录语义；如果 remote 不以 `/` 结尾，也按目录处理并自动创建。

实现要点：

- 本地 hash 使用 Go `crypto/sha256`。
- 远端 hash 通过 `sha256sum <path>` 获取。
- 远端缺少 `sha256sum` 时返回明确错误。
- 路径 quote 必须复用统一 shell quote。
- Windows 下 glob 不能依赖 shell 展开，应由 Go 自己展开。
- `--remove-dir` 必须在 help 中明确是危险操作，并要求 remote path 非空且不为 `/`。

验收：

- 文件上传/下载输出 size 和 elapsed。
- `--sha256` 成功时输出本地/远端 hash 和 ok=true。
- hash 不一致时命令失败并输出明确错误。
- `--remove-dir` 上传目录前会删除远端目录并重新上传。
- glob 匹配多个文件时逐个上传并输出汇总；匹配为空时报错。

阶段提交：

- P4.1 提交：`feat: show transfer size and elapsed time`，上传/下载 size、elapsed 统计输出。
- P4.2 提交：`feat: add transfer sha256 verification`，文件级 `--sha256` 校验。
- P4.3 提交：`feat: add scp remove-dir option`，scp 目录上传 `--remove-dir`。
- P4.4 提交：`feat: support scp local glob uploads`，scp 本地 glob 多文件上传。
- P4.5 提交：`docs: update transfer help and TODO`，help/README/TODO 更新。

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

阶段提交：

- P5.1 提交：`docs: add sshc deploy examples`，新增 `docs/deploy-examples.md`。
- P5.2 提交：`docs: link deploy examples from README and help`，README 和 LongHelp 链接/提示更新。

### P6: add 和主机配置增强

目标：

- 改善 host 配置录入、认证方式和组织能力。
- 使用固定配置文件 `~/.config/sshc/sshc.config.json`。
- 为后续分组过滤、备注搜索、读取已有 SSH 配置打基础。

命令面草案：

```bash
sshc add -I
sshc add --ip 192.168.1.10 --name devhost -u root -p password --remark "testing gpu host" --group testing
sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
sshc add --from-clipboard
sshc list --group testing
```

配置模型建议：

```json
{
  "name": "devhost",
  "ip": "192.168.1.10",
  "user": "root",
  "password": "...",
  "key_path": "~/.ssh/id_rsa",
  "remark": "testing gpu host",
  "group": "testing",
  "port": 22
}
```

建议拆分：

- P6.1: `remark` / `group` / `key_path` 字段，保持明文 password 兼容。
- P6.2: `add -I` 交互录入，引入 `github.com/gookit/cliui`。
- P6.3: clipboard 导入，先定义固定格式再实现。
- P6.4: password 加密，单独设计密钥来源、迁移和降级策略。
- P6.5: 读取 `~/.ssh/config`，明确 `sshc.config.json` 与 ssh config 的优先级。

验收：

- `sshc.config.json` 旧结构 JSON shape 能正常读取。
- 新字段保存后 list/log/run/scp/download 不受影响。
- key path 认证可执行 run。
- 未配置 password 但配置 key path 时不再报 password required。

阶段提交：

- P6.1 提交：`feat: extend host config fields`，包含 `remark` / `group` / `key_path` 配置模型、保存读取和 list 展示。
- P6.2 提交：`feat: add key path ssh authentication`，key path SSH 认证接入。
- P6.3 提交：`feat: add interactive host entry`，`add -I` 交互录入。
- P6.4 提交：`feat: add clipboard host import`，clipboard 导入固定格式。
- P6.5 提交：`feat: load hosts from ssh config`，`~/.ssh/config` 读取。
- P6.6 提交：`docs: design password encryption` 或 `feat: encrypt stored host passwords`；若进入实现，必须单独提交，不和其他 add 能力混入。

### P7: login/connect 交互式连接

目标：

- 新增交互式 SSH 连接能力，用于连续操作远端主机。

命令面草案：

```bash
sshc login devhost
sshc connect devhost
```

建议语义：

- `login` 和 `connect` 互为别名。
- 打开 PTY，进入远端 shell。
- 默认只记录连接开始、结束、目标、退出状态，不记录完整输入输出，避免日志包含敏感内容。
- 后续如需完整 session log，再加显式参数：

```bash
sshc login devhost --record
```

实现要点：

- 使用 `golang.org/x/term` 或 goph/ssh 底层 session PTY 能力。
- 处理 Windows 终端 raw mode 恢复。
- Ctrl+C、窗口 resize、退出状态需要测试。

验收：

- 能进入远端 shell 并正常退出。
- 退出后本地终端状态恢复。
- run 日志或独立 login 日志记录连接元信息。

阶段提交：

- P7.1 提交：`feat: add interactive ssh login command`，PTY 连接基础能力和 `login/connect` 命令。
- P7.2 提交：`fix: restore terminal state after login`，终端状态恢复、resize、Ctrl+C 等行为完善；如果不是修复而是首次补全行为，可使用 `feat: handle terminal resize in login`。
- P7.3 提交：`docs: document login command behavior`，连接日志和文档更新。

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

调整：

- 本计划已将“本地 glob 多文件上传”纳入 P4 后半段。
- 逗号分隔多个文件仍暂缓，优先用 glob 或多次命令覆盖。

### password 加密

原因：

- 需要决定密钥来源，例如 OS keyring、机器本地密钥、用户口令派生、还是仅做弱混淆。
- 会影响配置迁移、备份恢复、跨机器复制配置等行为。
- 做不好容易形成“看似安全但实际不可控”的状态。

建议先完成 key path 支持和交互录入，再单独设计。

## 建议实施顺序

1. P1.1: target 非完整匹配，验证后用 `feat:` 提交。
2. P1.2: `run --script`、`--keep-remote-script`，验证后用 `feat:` 提交。
3. P1.3: `run --cwd`，验证后用 `feat:` 提交。
4. P2.1-P2.2: 远端 `timeout` 包装、`--kill-after`，按子阶段用 `feat:` 提交。
5. P3.1-P3.2: `--sudo`、`--sudo-user`，实现用 `feat:`，文档用 `docs:`，测试补强用 `test:`。
6. P4.1-P4.2: `scp/download --sha256`、size、elapsed 输出，按子阶段用 `feat:` 提交。
7. P4.3-P4.4: scp `--remove-dir`、本地 glob 多文件上传，按子阶段用 `feat:` 提交。
8. P5.1-P5.2: 部署示例文档和 README/help 链接，用 `docs:` 提交。
9. P6.1-P6.2: `remark` / `group` / `key_path` 与 key 认证，按子阶段用 `feat:` 提交。
10. P6.3-P6.6: `add -I`、clipboard、password 加密、`~/.ssh/config`，逐项用 `feat:` 或 `docs:` 提交。
11. P7.1-P7.3: `login/connect`，新增行为用 `feat:`，终端恢复修复用 `fix:`，文档用 `docs:`。
12. 暂缓项按后续使用反馈再排期。

## 待确认事项

- `--script` 是否只支持 bash 脚本，还是需要 `--shell sh|bash`。建议初版固定 bash。
- `--timeout` 是否允许在远端缺少 `timeout` 时回退到客户端 timeout。建议不回退，避免用户误以为远端进程已清理。
- `--sudo` 与 `--sudo-user` 同时出现时是否判定冲突。建议判定冲突。
- `--sha256` 是否覆盖目录。建议初版不覆盖目录。
- target 非完整匹配是否纳入 `remark/group` 字段。建议 P1 先匹配 name/IP，P6 字段落地后再扩展到 remark/group。
- `add --from-clipboard` 的输入格式。建议后续明确为一行 `ip,user,password,name,port` 或多行 `key=value`，不要自动猜任意文本。
- password 加密是否真的需要。若主要使用 key path，password 加密可以后置。
- `login/connect` 是否需要完整 session log。建议默认不记录完整内容。
