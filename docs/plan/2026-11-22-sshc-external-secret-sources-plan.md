<!-- template_id: plan; template_version: 1.3.1 -->
# sshc 外部密钥来源实施计划

> 状态：Draft 0.1 / 待人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=按 `docs/design/2026-11-22-sshc-external-secret-sources-design.md` Draft 0.2 实现 `password_cmd`/`key_passphrase_cmd`：运行时执行命令取秘密、进程内按命令缓存、值/命令互斥校验、CLI 录入与 `check --no-secret-cmd`；allowed_scope=`internal/core`（store/config_resolve/secret_cmd/ssh/check）、`internal/command`（add/auth/check）、README 与上述代码测试；non_goals=跨进程缓存、`key_cmd`、内置 gopass/sops 集成、远端取值、加密格式变更；expansion_policy=DEFER_OR_REQUEST；review budget=本机工具档一轮合并轴评审；停止条件=每个任务 owner 文件、验证命令与可观察完成标准明确并追溯到设计决策。暴露度：本机工具档；仅本机以当前用户权限执行配置命令，可删除字段回退，不涉及 Secret 落盘与出机器动作。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-22 | Suga | 初稿：配置与校验、命令执行器与缓存、运行时接线、CLI 录入、check 开关、文档与真实验证 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 目标与完成定义

完成定义（全部满足才算完成）：

1. 配置里可写 `password_cmd` / `key_passphrase_cmd`（Host 与 AuthProfile），运行时执行该命令取秘密；配置中不出现解析结果。
2. 同一次 sshc 调用内相同命令只执行一次（共享 auth profile 的批量 check 不重复解锁）；跨进程无缓存。
3. 值与命令同时配置时加载即失败；`check --no-secret-cmd` 跳过命令执行且 `Auth` 显示 `-`。
4. 命令空输出、多行输出、非零退出、超时均失败，且错误文本不含 stdout；秘密不进入 `sshc log` 记录。
5. `sshc add`/`auth add` 可通过 `--password-cmd`/`--key-passphrase-cmd` 录入，并与 `-p`/`--key-passphrase` 互斥（退出码 2）。
6. `go build ./...`、`go vet ./...`、`go test ./...` 通过；本机用真实命令完成一次端到端验证。

## 范围、排除项与授权

范围：`internal/core/store.go`、`config_resolve.go`、新增 `secret_cmd.go` + `secret_cmd_windows.go`/`secret_cmd_unix.go`、`ssh.go`、`check.go`；`internal/command/add.go`、`auth.go`、`check.go`、`list.go`（`auth: cmd` 展示）；`README.md`、`README.zh-CN.md`；对应 `_test.go`。

排除项：`key_cmd`、跨进程缓存、gopass/sops 专用集成、远端（跳板）侧取值、`*_enc` 格式变更、任何 push/tag/发布动作。

授权边界：本计划只授权实施与本地验证所需的改动和本地提交；推送、打 tag、发布需实施后的外部动作 Gate 单独批准。

- `host_or_non_offline_action=NOT_APPLICABLE`

## 输入与批准证据

- 设计：`docs/design/2026-11-22-sshc-external-secret-sources-design.md`（Draft 0.2，candidate `ada0217`）。
- 设计批准证据（用户原话，2026-11-22）：`批准 Draft 0.2，进入实施计划`。
- 设计已确认决策：Windows shell `pwsh`→`powershell`→`cmd`；`check` 默认执行命令并提供 `--no-secret-cmd`；同一次调用内共享 auth 只执行一次。
- 适用规范：SR1403、SR1405、SR1410、SR1204、SR1301、SR2003、SR1402.1、GR104、GR110。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | 秘密字段与冲突校验 | `internal/core/store.go:95-106`、`config_resolve.go:280-300,448+` | 复用现有 Host/AuthProfile 与校验入口 | 现有结构加两个字段 | DIRECT_REUSE | none | 无重复实现，随 sshc 版本发布 |
| CAP-02 | 执行本机命令取 stdout | `internal/core` 内既有 exec 用法（`scan_neighbors_windows.go`、`cfg` 编辑） | `os/exec` + `exec.LookPath` | 新增 `core/secret_cmd*.go` 薄封装 | OWNER_EXTENSION | 无既有“取值命令”能力 | 单一 owner；不引入第三方库 |
| CAP-03 | 进程内结果缓存 | stdlib `sync.Mutex`/`map` | 直接使用 | 缓存封装在 `secret_cmd.go` | DIRECT_REUSE | none | 无 |
| CAP-04 | CLI 选项与互斥校验 | `internal/command/add.go:41-170`、`auth.go:18-80`、`gcli` `StrOpt` | 复用现有选项绑定与 handler 校验 | 现有命令加选项 | DIRECT_REUSE | none | 无 |
| CAP-05 | check 跳过命令执行 | `internal/core/check.go:17-22,111-136`、`internal/command/check.go:31-80` | 复用 CheckOptions 与命令选项 | 加 `NoSecretCmd` 字段与 `--no-secret-cmd` | OWNER_EXTENSION | 无既有开关 | 无 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 内置 gopass/sops 适配层 | CAP-02 | 用户命令已覆盖所有工具；内置适配删除后无功能损失，反增依赖与版本跟随成本 |
| 独立 secret 缓存进程/文件缓存 | CAP-03 | 设计已限定进程内缓存；文件缓存删除后无功能损失，且会增加秘密落盘面 |

## 前置检查与 fail-closed 条件

- workspace baseline：Git root `D:/work/inhere/my-tools-dev/inhere-tools/sshc`，branch `main`，HEAD `ada02177736a8c63d5b718eedd0f944b7c355b6b`，`git status --short --branch` 为 `## main...origin/main`（干净）。
- expected paths/symbols：`internal/core/store.go`（`Host.PasswordCmd`、`AuthProfile.PasswordCmd`、`KeyPassphraseCmd`）、`config_resolve.go`（冲突校验、`applyAuthProfile`）、新增 `internal/core/secret_cmd.go`（`ResolveSecretCommand`、`ResolveHostSecrets`）、`internal/core/ssh.go`（`hostAuth` 前解析）、`internal/core/check.go`（`CheckOptions.NoSecretCmd`）、`internal/command/add.go`/`auth.go`/`check.go`、`README*.md`。
- 环境假设：Go 1.26（`go.mod` 已声明）；unix 有 `/bin/sh`，Windows 有 `pwsh`/`powershell`/`cmd` 之一。
- fail-closed：出现本计划 owner 之外的 dirty/untracked 文件即停；`go build ./...` 在实施前已失败时先停下报告。

## 波次与依赖

- Wave 1：T01 字段与校验、T02 执行器与缓存（无依赖）。
- Wave 2：T03 运行时接线、T04 CLI 录入（依赖 T01/T02）。
- Wave 3：T05 check 开关与展示、T06 文档与真实样例（依赖 T03/T04）。
- Wave 4：T07 全量回归与真实边界验证（依赖全部）。

## 任务

### T01 Host/AuthProfile 命令字段与冲突校验

- 文件: `internal/core/store.go`、`internal/core/config_resolve.go`、`internal/core/store_test.go`（或现有对应测试文件）
- 动作: 给 `Host` 与 `AuthProfile` 增加 `PasswordCmd`、`KeyPassphraseCmd`（json `password_cmd`、`key_passphrase_cmd`，omitempty）；`NormalizeHostFields` 与 profile 规范化 TrimSpace；`applyAuthProfile` 复制这两个字段（profile 覆盖 host）；在 `validateHost` 与 `validateEffectiveHost` 加“值 + 命令同时存在”报错（文案 `password and password_cmd cannot both be set`，passphrase 同理）。
- 验证: `go test ./internal/core -run 'TestHostSecretCommandFields|TestValidateHostSecretConflict' -count=1`
- 完成标准: 字段可 round-trip（SaveConfig → LoadConfig 保留）；值+命令同时配置在 host 与 profile 两处均返回错误；命令为空字符串视为未设置；profile 的命令覆盖 host 的值。
- 依赖: 无

### T02 命令执行器与进程内缓存

- 文件: `internal/core/secret_cmd.go`、`internal/core/secret_cmd_windows.go`、`internal/core/secret_cmd_unix.go`、`internal/core/secret_cmd_test.go`
- 动作: 实现 `ResolveSecretCommand(command string, timeout time.Duration) (string, error)`：unix `/bin/sh -c`，windows 依次 `LookPath` `pwsh`→`powershell`→`cmd`（`-NoProfile -Command` / `/C`）；取 stdout，仅裁结尾 `\r`/`\n`，内部含换行即报错；非零退出、空输出、超时返回错误（stderr 截断 ≤200 字符）；实现按命令字符串的进程内缓存（仅缓存成功），并提供 `ResolveHostSecrets(host Host) (Host, error)` 就地写回 `Password`/`KeyPassphrase` 并清空命令字段；超时默认 10s，`SSHC_SECRET_CMD_TIMEOUT` 可覆盖。
- 验证: `go test ./internal/core -run 'TestResolveSecretCommand|TestResolveHostSecrets' -count=1`
- 完成标准: 成功命令返回值已裁结尾换行；多行输出、空输出、非零退出、超时各有用例；同一命令连续调用只执行一次（用注入计数器断言）；失败不写入缓存（失败两次计数为 2）；错误文本不含 stdout 内容。
- 依赖: 无

### T03 运行时接线（连接与校验入口）

- 文件: `internal/core/ssh.go`、`internal/core/check.go`、`internal/core/ssh_test.go`、`internal/core/check_test.go`
- 动作: 在 `newSSHClientWithOptions` 与 `checkSSHConnect` 调用 `hostAuth()` 之前执行 `ResolveHostSecrets`（失败即返回错误，不回退）；`CheckOptions` 增加 `NoSecretCmd bool`，为真时跳过命令解析并把 `Auth` 置 `CheckStatusSkipped`、`Error` 写 `secret command skipped`；`validateCheckHostConfig` / `hasPasswordAuth` 等判定把命令字段算作可用认证来源；`hostAuth` 仍只消费已解析的值。
- 验证: `go test ./internal/core -run 'TestCheckHostSecretCommand|TestSecretCommandSkipsAuth' -count=1`
- 完成标准: 配置 `password_cmd` 的 host 在 check 中会被执行（用注入执行器断言调用次数）；`NoSecretCmd` 时执行次数为 0 且 `Auth == "-"`；命令失败时 `Auth == "fail"` 且 `Error` 含 `secret command`；已解析值不进入 `CheckResult` 的任何字段。
- 依赖: T01、T02

### T04 CLI 录入与互斥

- 文件: `internal/command/add.go`、`internal/command/auth.go`、`internal/command/add_test.go`（或 `auth_test.go`）
- 动作: `host add` 与 `auth add` 增加 `--password-cmd`、`--key-passphrase-cmd`；与 `-p/--password`、`--key-passphrase <source>` 同时给出即返回参数错误；写入 Host/AuthProfile 的新字段；`sshc list` 的认证列对命令型来源显示 `cmd`（不显示命令内容）。
- 验证: `go test ./internal/command -run 'TestAddHostPasswordCommand|TestAuthAddPasswordCommand|TestSecretCommandFlagConflict' -count=1`
- 完成标准: `add --password-cmd` 与 `auth add --password-cmd` 落盘字段正确；与 `-p` 或 `--key-passphrase` 冲突时退出码 2 且未写配置；`list` 输出包含 `cmd` 且不包含命令字符串。
- 依赖: T01

### T05 check 命令开关与表格行为

- 文件: `internal/command/check.go`、`internal/command/check_test.go`
- 动作: `check` 增加 `--no-secret-cmd` 并传入 `core.CheckOptions.NoSecretCmd`；表格 `Auth` 列在跳过时显示 `-`；命令失败时 `Error` 前缀保持 `secret command`。
- 验证: `go test ./internal/command -run 'TestCheckNoSecretCmd' -count=1`
- 完成标准: `check --no-secret-cmd --json` 中 `auth` 为 `-`；不带该开关时（注入执行器）会被调用一次；两条路径的退出码与现有 check 语义一致（无失败 0、失败非 0 由现有规则决定）。
- 依赖: T03

### T06 README 与设计批准留痕

- 文件: `README.md`、`README.zh-CN.md`、`docs/design/2026-11-22-sshc-external-secret-sources-design.md`
- 动作: 两个 README 增加“命令型秘密来源”小节（gopass/sops 示例、互斥规则、Windows shell 顺序、缓存与超时、`check --no-secret-cmd`、信任边界）；设计文档状态行补批准证据（provenance，不升版本）。
- 验证: `idevstd script validate_document --kind design docs/design/2026-11-22-sshc-external-secret-sources-design.md` 与 `grep -c "password_cmd" README.md README.zh-CN.md`
- 完成标准: 设计校验 `ok: true`；两份 README 各含至少一处 `password_cmd` 可复制示例，且 flag 名与实现一致。
- 依赖: T04、T05

### T07 真实命令端到端与全量回归

- 文件: 无新增（只运行验证，发现缺陷按 implementation-discovery 分类）
- 动作: `go build ./...`、`go vet ./...`、`go test ./... -count=1`；本机构建二进制到 `tmp/`，用一个真实取密码命令（如 `sshc host add 127.0.0.1 ... --password-cmd 'cmd /C echo demo-secret'` 或 unix 等价）执行 `sshc check --json 127.0.0.1`，确认命令被执行（错误应来自 SSH 认证而非 `secret command`），再用同命令 `check --no-secret-cmd` 确认跳过。
- 验证: 上述命令退出码与输出片段；真实验证不可执行的部分写 `UNVERIFIED_REAL_ENV: <面>（复现：<命令>）`。
- 完成标准: 全量测试通过；真实终端输出证明“命令执行→认证尝试”链路成立且秘密未出现在输出或 `tmp/` 下日志中；`--no-secret-cmd` 路径跳过执行。
- 依赖: T01–T06

## 回滚与恢复

- 回滚：删除两个新字段与 `secret_cmd*.go`、移除接线与 CLI 选项即可；已写入的 `password_cmd` 字段对旧版本是未知 json 字段，读取时忽略，不会破坏配置。
- 恢复点：每任务一个原子提交（T01→T07），失败从上一提交继续，不 reset 他人改动。
- dirty 保护：提交前 `git status --short` 只包含该任务 owner 文件。

## 人工 Gate

1. **计划批准**：本计划 Draft 0.1 需要你明确批准后才能实施（`批准实施计划 0.1, 开始实施`）。
2. **外部动作 Gate（实施完成后逐项批准）**：`git push`；打 tag/发布。

## 可追溯性

| 设计决策/验收 | 任务 | 验证 |
|---|---|---|
| D1 字段名与 profile 覆盖 | T01 | round-trip 与覆盖用例 |
| D2 值/命令互斥 | T01、T04 | 加载失败用例、CLI 退出码 2 |
| D3 输出裁剪与多行报错 | T02 | 执行器用例 |
| D4 进程内按命令缓存、失败不缓存 | T02 | 计数器用例 |
| D5 Windows shell 顺序 | T02 | `secret_cmd_windows.go` 实现 + 本机 e2e（T07） |
| D6 check 默认执行 + `--no-secret-cmd` | T03、T05 | check 用例与 JSON 断言 |
| 完成定义 6（构建/vet/测试/真实样例） | T07 | 三条命令 + 真实终端输出 |

## 完成 Gate 与剩余工作

- 完成 Gate：T01–T07 全部 done，`go build ./...`、`go vet ./...`、`go test ./...` 通过，真实样例输出已记录；完成后在 progress 写 ≤8 行完成总结。
- 剩余工作：`key_cmd`、跨进程缓存、内置 gopass/sops 适配（均需另行设计）。
