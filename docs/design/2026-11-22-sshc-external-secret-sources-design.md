<!-- template_id: design; template_version: 1.2.0 -->
# sshc 外部密钥来源（命令取值）设计

> 状态：Draft 0.2 / 待人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=允许 sshc 只保存“获取密码/私钥口令的命令”（如 gopass、sops），连接时执行命令取值，配置里不再存放明文或加密后的秘密；allowed_scope=Host/AuthProfile 新增命令型秘密来源字段、运行时解析与校验、连接入口接线、`host add`/`auth add` 录入方式、README 与测试；non_goals=通用 secret 管理、内置 gopass/sops 集成、跨进程秘密缓存、远程/跳板侧取值、密钥文件内容命令化（`key_cmd` 二期）、任何 secret 落盘加密格式变更；expansion_policy=DEFER_OR_REQUEST；review budget=本机工具档一轮合并轴评审；停止条件=字段名与优先级、执行时机与失败语义、输出裁剪规则、安全边界与验收均明确。暴露度：本机工具档；仅在维护者本机以当前用户权限执行配置中的命令，错误可回退（删除字段即恢复原行为），不新增 Secret 备份或出机器动作。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-22 | Suga | 初稿：`password_cmd`/`key_passphrase_cmd`、运行时执行、单行裁剪、冲突即拒、失败即停 |
| 0.2 | 2026-11-22 | Suga | 按批准反馈补充：Windows 按 pwsh→powershell→cmd 探测；同一次调用内按命令缓存（共享 auth 只执行一次）；check 默认执行并提供 `--no-secret-cmd` |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 背景与目标

目前 `sshc` 的认证信息只能以“值”的形式写在配置里（`password`、`password_enc`、`key_path`、`key_data`、`key_passphrase*`）。`sshc auth add --key-passphrase input|clip|env` 也只在**写入时**取值一次，随后把结果存进配置。

用户已用 gopass / sops 管理秘密，希望 sshc 配置里只留“取值的命令”，运行连接时再执行命令拿密码或私钥口令。目标：

- 配置中可声明 `password_cmd` / `key_passphrase_cmd`，值是一条本机命令；
- 连接（run/scp/check/login/tunnel 等所有需要认证的入口）时执行该命令，取 stdout 作为秘密；
- 命令失败、输出为空或输出不是单行时明确报错，不回退到其他认证方式；
- 秘密既不落盘、也不进入日志（含 `sshc log` 的运行记录）。

## 名词

- **命令型来源**：配置里保存的是命令字符串，秘密在运行时由该命令输出提供。
- **秘密值**：`password`（SSH 密码）与 `key_passphrase`（私钥口令）两类；密钥文件内容本身不在本期范围。
- **解析时机**：`ResolveEffectiveHost` 合并出 `EffectiveHost` 之后、`hostAuth()` 构造 goph 认证之前。

## 范围与非目标

- `AuthProfile` 与 `Host` 新增 `password_cmd`、`key_passphrase_cmd` 两个可选字段；`GroupDefaults` 不新增（秘密来源不随分组继承，避免一次误配置扩散）。
- 优先级与冲突：同一 scope 内 `password` 与 `password_cmd` 同时出现即校验失败；`key_passphrase` 与 `key_passphrase_cmd` 同理。profile 与 host 之间沿用既有覆盖规则（profile 优先），但被覆盖的一方若自身冲突仍在加载时失败。
- 执行时机：每次需要一个已解析的秘密时读取进程内缓存，未命中才执行命令；同一次 sshc 调用内使用相同命令（如同一 auth profile 被 `check --all` 的多个 host 共用）只执行一次，跨进程不缓存。
- Windows shell：按 `pwsh` → `powershell` → `cmd` 顺序 `LookPath` 探测，用第一个可用的（前两者加 `-NoProfile -Command`，`cmd` 用 `/C`）；unix 固定 `/bin/sh -c`。
- 输出规则：取 stdout，去掉结尾的 `\r`/`\n`；结果若仍包含内部换行则报错（避免把多行输出误当密码）。非零退出码、空输出、超时均失败。
- 非目标：不内置 gopass/sops 专用集成（用户直接在命令里写 `gopass show -on …`）；不做跨进程缓存（缓存只活在一次 sshc 调用内）；不改变 `*_enc` 现有加密字段语义；不支持从远端（跳板机）执行取值命令。
- `check`：默认与其它命令一样执行命令型来源（`Auth` 列需要真实验证认证）；新增 `--no-secret-cmd`，设置后本次 check 跳过命令执行，`Auth` 置 `-`、`Error` 写 `secret command skipped`。
- `key_cmd`（用命令提供私钥内容）列为二期：会涉及密钥格式校验与 `key_data_enc` 语义，本期不做。

## 已确认事实与规范

- 现状字段：`internal/core/store.go:38-65`（Host）、`store.go:95-106`（AuthProfile）只有值型秘密字段；`GroupDefaults`（`store.go:82-93`）已有 `AuthRef`/`KeyPath` 但无秘密来源字段。
- 运行时取值点：`internal/core/ssh.go:1052-1075` 的 `hostAuth()` 直接读 `host.Password`、`host.KeyPath`/`KeyData`、`host.KeyPassphrase`，没有任何外部来源解析。
- 合并与校验入口：`internal/core/config_resolve.go:280-300`（`applyAuthProfile`，profile 覆盖 host）、`config_resolve.go:448+`（`validateEffectiveHost`）是新增校验的落点。
- 现有一次性取值语义：`internal/command/util.go:164-190` 的 `resolveKeyPassphrase`（input/clip/env）在 add/auth add 时解析并存储，证明“命令型来源”是新增能力而非现有行为。
- 运行记录：`internal/core/run_logs.go`（`RunLogRecord`/`AppendRunLog`）记录命令、输出、路径等字段，不含密码/口令字段；本期要求解析出的秘密不得进入该记录或任何错误文本。
- 适用 SR1403（复用现有连接入口，不新建认证层）、SR1405（本期只做两条命令型来源）、SR1410（失败即停，不做回退链）、SR1204（预计 4 个逻辑文件、>100 行，需批准）、SR1301（秘密不进日志/输出、入参校验）、SR2003（CLI 选项绑定）。

## 总体方案

1. **配置字段**：`Host` 与 `AuthProfile` 增加 `password_cmd`、`key_passphrase_cmd`（json `password_cmd`、`key_passphrase_cmd`，omitempty）。`NormalizeHostFields`/profile 规范化只做 `TrimSpace`。
2. **校验**：`validateEffectiveHost` 与配置加载路径检查“值 + 命令”同时存在即报错，错误文本形如 `host "x": password and password_cmd cannot both be set`；命令为空字符串视为未设置。
3. **解析器**：新增 `internal/core/secret_cmd.go`（+ `_windows.go`/`_unix.go` 的 shell 选择），提供
   - `ResolveSecretCommand(command string, timeout time.Duration) (string, error)`：`unix` 用 `/bin/sh -c`；`windows` 依次探测 `pwsh`、`powershell`、`cmd`（`-NoProfile -Command` / `/C`）；`CombinedOutput` 的 stderr 只在失败时截断后附在错误里（≤200 字符），stdout 裁剪规则见范围；
   - `ResolveSecretCommandCached(...)`：以命令字符串为 key 的进程内缓存（`sync.Mutex`+`map`），命中即返回，仅缓存成功结果，失败每次重试（账号未解锁时用户可重新运行命令）；
   - `ResolveHostSecrets(host Host) (Host, error)`：就地解析 `password_cmd`/`key_passphrase_cmd`，写回运行时的 `Password`/`KeyPassphrase`（运行时不落盘），并清空命令字段，避免同一次调用内重复执行。
4. **接线**：`newSSHClientWithOptions` 与 `checkSSHConnect` 在 `hostAuth()` 之前调用 `ResolveHostSecrets`；`EffectiveHost.ToHost()` 之后统一执行，确保 `run/scp/check/login/tunnel/upload/download` 全覆盖。超时默认 10s，可用 env `SSHC_SECRET_CMD_TIMEOUT` 覆盖（同 `SSHC_CONFIG_DIR` 风格）。共享同一 auth profile 的批量操作（如 `check --all`）只执行一次命令。
5. **录入**：`host add --password-cmd '<cmd>'`、`auth add <name> --password-cmd '<cmd>'`、`--key-passphrase-cmd '<cmd>'`；与 `-p/--password`、`--key-passphrase <source>` 互斥，同时给出即参数错误（退出码 2）。
6. **可观测**：`sshc list`/`cfg show` 只展示“是否使用命令型来源”（例如 `auth: cmd`），绝不显示命令输出；`sshc check` 的 `Auth` 状态在命令失败时显示 `fail` 并在 `Error` 中给出去敏后的原因（如 `secret command failed: exit status 1`）；`check --no-secret-cmd` 时 `Auth` 为 `-`。

## 架构

```text
internal/command/add.go, auth.go     新增 --password-cmd / --key-passphrase-cmd 选项与互斥校验
    -> internal/core/store.go         Host/AuthProfile 新字段 + 规范化
    -> internal/core/config_resolve.go 值/命令冲突校验、profile 覆盖
    -> internal/core/secret_cmd.go    执行命令、裁剪输出、错误构造（新增单文件）
    -> internal/core/ssh.go           hostAuth 前解析一次，写回运行时字段
```

代码组织维持 `internal/command`（CLI）+ `internal/core`（领域）分工；执行器按 `_windows.go`/`_unix.go` 区分 shell，不新建包、不引入第三方依赖。

## 关键流程

1. `sshc auth add ops --user root --password-cmd 'gopass show -on ssh/ops'` → 配置中只存命令；`sshc run --auth ops host -- uptime` 时执行命令取密码。
2. `sshc host add dev --ip 10.0.0.8 --key ~/.ssh/id_ed25519 --key-passphrase-cmd 'sops -d --extract "[\"passphrase\"]" secrets.yaml'` → 连接时解析口令。
3. 命令返回空 / 多行 / 非零退出 / 超时 → 连接失败并报出可定位的原因，不回退到 `password` 或交互输入。
4. 同时写 `password` 与 `password_cmd` → 加载即失败，提示二者互斥。

## 安全、数据、运维与回滚

- 命令是本机配置的一部分，等价于“以当前用户权限执行你写的脚本”；README 与 `--help` 必须写明这一信任边界，并提醒不要在命令字符串里直接内嵌秘密（那是明文配置）。
- 秘密只在内存中停留一次连接生命周期：不写配置、不写 `sshc log`、不打印到 stdout/stderr；错误文本不含 stdout，stderr 仅在失败时截断附带。
- 子进程继承当前环境（gopass/sops 依赖各自的 env/凭据目录），sshc 不额外注入秘密。
- 超时默认 10s，超时即失败；`check --parallel` 时同一命令的结果在进程内共享，不会因 host 数量而重复解锁。
- 回滚：删除 `password_cmd`/`key_passphrase_cmd` 并写回原值即可；字段只读兼容（旧配置无这些字段时行为不变）。

## 决策

- D1：字段名 `password_cmd`、`key_passphrase_cmd`（与现有 `password`、`key_passphrase` 对齐）。
- D2：值/命令同时配置视为错误，不做隐式优先级。
- D3：只取 stdout、只裁结尾换行，内部换行即报错。
- D4：进程内按命令字符串缓存成功结果，同一次调用内同一命令只执行一次；失败不缓存。
- D5：`key_cmd` 与跨进程缓存列二期，需另行设计。
- D6：Windows shell 按 `pwsh` → `powershell` → `cmd` 探测，优先前者。
- D7：`check` 默认执行命令型来源，`--no-secret-cmd` 可显式跳过。

## 待确认事项

- 无阻塞项。已确认：Windows shell 顺序（`pwsh`→`powershell`→`cmd`）；`check` 默认执行命令型来源并提供 `--no-secret-cmd`；同一次调用内共享 auth 只执行一次命令。

## 结论与人工计划 Gate

本设计确定命令型秘密来源的字段、执行时机、失败语义与安全边界，实施范围约 4 个逻辑文件（新增 `secret_cmd*.go`、改 `store.go`、`config_resolve.go`、`ssh.go`，另加 `add.go`/`auth.go` 选项与 README）。

**需要你批准 Draft 0.2 后才能进入实施计划**（批准进入规划，不等于授权实施）。
