# sshc 配置管理与凭证模型实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-04 | Codex | 初版，基于配置管理与凭证模型设计拆分实施阶段、提交边界和验收项 |
| v0.2 | 2026-07-04 | Codex | 确认待确认事项，复审并调整 auth/host 实施顺序、known_hosts 接入和兼容解密要求 |

## 关联文档

- 设计文档：`docs/2026-07-04-sshc-config-auth-design.md`
- 后续能力总览：`docs/2026-07-04-sshc-next-features-design.md`
- 现有密码说明：`docs/password-encryption-design.md`

## 背景

当前 `sshc` 已经具备 `add/list/run/login/scp/download/log` 等主链路能力，并已把默认配置文件固定为：

```text
~/.config/sshc/sshc.config.json
```

现有配置结构仍是简单 `Store{logs_path, hosts}`，密码加密也只覆盖 host 内联 `password`。后续要支持多级命令、`cfg`、`host`、`auth`、`jump`、`batch-run`、`export/import`，必须先稳定配置模型和凭证模型。

本计划只覆盖配置管理与凭证模型落地，不实现 batch、jump、export/import 的完整业务能力。

## 目标

- 将配置模型升级为 versioned config：`version/defaults/auth_profiles/hosts`。
- 保持旧配置读取兼容，执行保存类命令后写入新结构。
- 增加 auth profile，允许多个 host 复用同一套用户、密码或 key。
- 增加 effective host 解析，统一处理 defaults、auth profile、host 内联字段和命令行覆盖。
- 扩展密码加密逻辑，覆盖 host 和 auth profile。
- 新增 `cfg`、`host`、`auth` 多级管理命令。
- 保持顶层 `add/list/ls` 兼容入口。
- 每个阶段独立验证、独立提交，不做大批量一次性提交。

## 非目标

- 不支持 `.ssh/password` 或其他明文密码文件。
- 不引入 OS keyring。
- 不强制把已有 host 内联凭证迁移成 auth profile。
- 不实现完整 `cfg export/import`，仅保留配置结构兼容边界。
- 不实现 jump host 连接逻辑，只允许配置模型预留 `jump` 字段。
- 不重做 `run/scp/download/login` 的核心传输执行逻辑，除非为了接入 effective host 必须调整。

## 当前状态

当前核心文件：

| 文件 | 当前职责 | 本计划影响 |
| --- | --- | --- |
| `internal/core/store.go` | `Host`、`Store`、配置路径、load/save、legacy `hosts.json` 读取、`~/.ssh/config` 解析 | 拆出 config 模型和 store 逻辑，保留兼容入口 |
| `internal/core/password_crypto.go` | host password 加密、解密和 key file 管理 | 扩展到 auth profile |
| `internal/core/ssh.go` | SSH client、run/login/upload/download | 接入 effective host 或兼容转换 |
| `internal/core/run_logs.go` | run log 路径读取 `logs_path` | 改为读取新 config settings |
| `internal/command/add.go` | 顶层 add 命令，保存 host | 后续复用 `host add` handler |
| `internal/command/list.go` | 顶层 list/ls 命令 | 后续复用 `host list` handler，并展示 auth profile 来源 |
| `internal/bootstrap/init.go` | capp app 和顶层命令注册 | P0 迁移到 gcli/v3 |

当前工作区已有未跟踪设计文档和 `docs/TODO.md` 脏改。实施时不要误提交无关脏改，除非明确把它们纳入阶段提交。

## 设计落点

目标配置结构：

```json
{
  "version": 1,
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

effective host 合并顺序：

```text
命令行显式参数 > host 内联字段 > auth_ref 指向的 auth profile > defaults > 内置默认值
```

## 提交策略

每个阶段完成后必须：

1. 运行该阶段对应测试。
2. 运行 `go test ./...`。
3. 运行 `go build -o tmp\sshc.exe ./cmd/sshc`。
4. 运行 `git diff --check -- <本阶段涉及文件>`。
5. 检查 `git status --short --branch`。
6. 使用 Conventional Commits 单独提交。

提交前缀约定：

| 前缀 | 使用场景 |
| --- | --- |
| `refactor:` | CLI 框架迁移、内部结构调整、不改变用户行为 |
| `feat:` | 新增配置模型、命令、用户可见能力 |
| `fix:` | 修复兼容、解密、路径、保存等错误行为 |
| `test:` | 仅测试补充 |
| `docs:` | 仅文档、help、README、计划更新 |
| `chore:` | 依赖、构建、仓库维护 |

阶段提交建议：

```text
refactor: migrate cli to gcli v3
feat(config): add versioned config model
feat(config): resolve effective host settings
feat(auth): encrypt and mask credential profiles
feat(config): add cfg management commands
feat(host): add host management commands
feat(auth): add credential profile commands
docs: update config and auth documentation
```

## 阶段总览

| 阶段 | 目标 | 主要提交 | 状态 |
| --- | --- | --- | --- |
| P0 | 迁移到 gcli/v3，保持现有命令兼容 | `refactor: migrate cli to gcli v3` | 已完成 |
| P1 | 新增 versioned config 模型和兼容读取保存 | `feat(config): add versioned config model` | 已完成 |
| P2 | 新增 effective host 解析并接入现有命令 | `feat(config): resolve effective host settings` | 已完成 |
| P3 | 扩展密码加密、mask 和 doctor 基础能力 | `feat(auth): encrypt and mask credential profiles` | 已完成 |
| P4 | 新增 `cfg` 管理命令 | `feat(config): add cfg management commands` | 已完成 |
| P5 | 新增 `auth` 凭证命令 | `feat(auth): add credential profile commands` | 待实施 |
| P6 | 新增 `host` 管理命令并兼容顶层 add/list | `feat(host): add host management commands` | 待实施 |
| P7 | 文档和最终验收 | `docs: update config and auth documentation` | 待实施 |

## P0: 迁移到 gcli/v3

目标：

- 从 `github.com/gookit/goutil/cflag/capp` 迁移到 `github.com/gookit/gcli/v3`。
- 保持已有顶层命令和 alias 可用。
- 为后续多级命令 `cfg/host/auth` 建立注册结构。
- 管理命令后续可以设置 `Category: "Management"`。
- completion 使用 gcli 内置能力，不新增自定义顶层 `completion` 命令。

范围：

- `go.mod`
- `go.sum`
- `cmd/sshc/main.go`
- `internal/bootstrap/init.go`
- `internal/command/*.go`
- `internal/command/*_test.go`

实施步骤：

1. 参考 `tools/gofer` 的 gcli app 初始化方式，建立 `bootstrap.NewApp()` 的 gcli 版本。
2. 将现有命令注册从 capp 转为 gcli。
3. 保持命令名和 alias：
   - `add`
   - `list/ls`
   - `run/exec`
   - `login/connect`
   - `scp/upload`
   - `download/dl`
   - `log/logs`
4. 保持现有 flag 名称、短名称、参数顺序和 LongHelp。
5. 调整测试 helper，确保命令输出和错误断言仍可用。

验收：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe --help | Out-String
.\tmp\sshc.exe run --help | Out-String
.\tmp\sshc.exe list --help | Out-String
.\tmp\sshc.exe --gen-completion bash | Select-Object -First 5
git diff --check -- go.mod go.sum cmd internal
```

提交：

```text
refactor: migrate cli to gcli v3
```

风险点：

- gcli 和 capp 的 variadic arg、`--` 后远端命令解析可能不同。P0 必须优先用测试固定 `run target -- command args` 行为。
- LongHelp 输出格式可能变化，测试不要依赖无意义空白。

## P1: 配置模型与兼容读取保存

目标：

- 新增 versioned config 数据结构。
- 旧配置继续可读。
- 新保存写入 `version: 1`。
- 保留 `LoadStore/SaveStore` 兼容入口，避免一次性改完所有命令。

建议新增文件：

```text
internal/core/config.go
internal/core/config_store.go
internal/core/config_validate.go
```

建议保留或迁移：

- `ConfigEnvKey`
- `ConfigFileName`
- `LegacyConfigFileName`
- `StorePath()`
- `LegacyStorePath()`
- `configRoot()`

核心结构：

```text
Config
Defaults
AuthProfile
Host
```

实施步骤：

1. 新增 `Config`：
   - `Version int`
   - `LogsPath string`
   - `Defaults Defaults`
   - `AuthProfiles []AuthProfile`
   - `Hosts []Host`
2. 扩展 `Host` 字段：
   - `AuthRef string`
   - `Jump string`
   - `Port` 改为 `omitempty` 语义，但兼容现有 JSON。
3. 新增 `LoadConfig()`：
   - 优先读取 `SSHC_CONFIG`。
   - 默认读取 `~/.config/sshc/sshc.config.json`。
   - 默认文件不存在且未设置 `SSHC_CONFIG` 时继续读取 legacy `hosts.json`。
   - 空文件返回空 config。
4. 新增 `SaveConfig(*Config)`：
   - 创建配置目录。
   - 写入 `0600` 临时文件，再 rename。
   - 保存前设置 `Version=1`。
5. 保留 `LoadStore()`、`SaveStore()`：
   - 短期包装到新 config。
   - 返回值尽量保持现有命令不感知新模型。
6. 保持现有 host `password_enc` 解密兼容：
   - P1 不新增 auth profile 加解密，但不能破坏已有 host 密文读取。
   - `LoadStore()` 仍需返回可直接用于现有 SSH 认证的内存态 `Host.Password`。
7. 调整 `LoadConfigSettings()` 和 run log 读取逻辑，读取新 config 的 `logs_path`。

验收：

- 旧结构 JSON 可读取。
- 新结构 JSON 可读取。
- legacy `hosts.json` 可读取。
- `SSHC_CONFIG` 覆盖路径仍有效。
- 保存后 JSON 包含 `version: 1`。
- 既有 host `password_enc` 配置仍能读取并解密到内存态。
- 空配置不会报错。

测试建议：

```text
TestLoadConfigLegacyStoreShape
TestLoadConfigVersionedShape
TestLoadConfigFromLegacyHostsFile
TestLoadConfigWithEnvPath
TestSaveConfigWritesVersionOne
TestLoadStoreStillDecryptsHostPasswordEnc
TestLoadConfigSettingsReadsLogsPath
```

验证命令：

```powershell
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal/core
```

提交：

```text
feat(config): add versioned config model
```

风险点：

- 不能因为新增 `Config` 破坏现有 `LoadStoreWithSSHConfig()` 合并 `~/.ssh/config` 的行为。
- 保存时不能丢失当前命令还不理解的字段，尽量以完整 `Config` 为主，而不是来回转换只保留 `Store` 字段。

## P2: Effective Host 解析并接入现有命令

目标：

- 新增 effective host 解析函数。
- 让 `run/login/scp/download/log` 使用统一解析结果。
- 先不做 jump 连接逻辑，只保留字段解析。

建议新增文件：

```text
internal/core/config_resolve.go
```

建议结构：

```text
EffectiveHost
ResolveEffectiveHost(target string, overrides HostOverrides)
FindAuthProfile(name string)
```

命令行覆盖初版只预留结构，不必一次性给所有命令增加覆盖参数。当前已有命令主要依赖配置字段。

实施步骤：

1. 新增 `EffectiveHost`。
2. 新增内置默认值：
   - `port=22`
   - `connect_timeout=20s`
   - `remote_script_dir=/tmp`
   - `host_key_check=known_hosts`
   - `known_hosts_path=~/.ssh/known_hosts`
3. 实现合并顺序：
   - built-in defaults
   - config defaults
   - auth profile
   - host inline
   - command overrides
4. 实现校验：
   - `ip` 必填。
   - `user` 必填。
   - `password/password_enc/key_path` 至少存在一种。
   - `port` 合法。
   - `auth_ref` 存在时必须能找到 profile。
5. 调整 `LoadStoreWithSSHConfig()` 或新增 `LoadConfigWithSSHConfig()`，保证现有 `~/.ssh/config` 主机仍可用于执行。
6. 调整现有命令内部解析：
   - `run`
   - `login`
   - `scp`
   - `download`
   - `log` 只需 host name 解析和日志路径兼容。
7. 接入 host key 策略：
   - `host_key_check=known_hosts` 使用 `known_hosts_path` 构建 SSH host key callback。
   - `host_key_check=insecure` 才使用 `ssh.InsecureIgnoreHostKey()`。
   - 目标不在 known hosts 中时返回明确错误，提示用户先建立信任或显式配置 `insecure`。
8. 为 `newSSHClient` 提供兼容方式：
   - 方案 A：让 `EffectiveHost` 转成 `Host` 传入现有函数。
   - 方案 B：逐步让 SSH 函数接收 `EffectiveHost`。
   - 初版建议方案 A，但 `Host` 或转换结果必须携带 host key 相关字段，不能因为转换丢失安全策略。

验收：

- host 内联凭证仍可执行现有命令。
- host 通过 `auth_ref` 复用 auth profile 时能解析出 user/password/key。
- host 内联 user/key/password 能覆盖 auth profile。
- defaults port/user 能被补齐。
- 默认 `known_hosts` 策略会实际用于 SSH callback。
- 显式 `insecure` 才会跳过 host key 校验。
- 缺失 auth profile 返回明确错误。
- 未配置认证方式返回明确错误。

测试建议：

```text
TestResolveEffectiveHostFromInlineAuth
TestResolveEffectiveHostFromAuthProfile
TestResolveEffectiveHostHostOverridesAuthProfile
TestResolveEffectiveHostUsesDefaults
TestResolveEffectiveHostMissingAuthRef
TestResolveEffectiveHostRequiresAuthMethod
TestResolveEffectiveHostDefaultKnownHosts
TestSSHClientUsesInsecureCallbackOnlyWhenConfigured
```

验证命令：

```powershell
go test ./internal/core
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal/core internal/command
```

提交：

```text
feat(config): resolve effective host settings
```

风险点：

- `list` 展示仍应展示原始 host 列表，而不是只展示 effective host，避免用户看不到配置来源。
- `log` 文件名仍应优先使用 host name，不应因为 effective 转换丢失 name。
- 如果 P2 接入 `known_hosts` 导致现有测试环境无法连接，测试应使用显式 `insecure` 配置或 mock callback，不要把默认值改回 insecure。

## P3: 密码加密、mask 和本地校验基础

目标：

- 扩展密码加密到 auth profile。
- 保证保存加密不修改调用方内存对象。
- 新增 mask helper，给后续 `cfg show/host show/auth show` 共用。
- 新增本地 doctor 校验函数，但命令可在 P4 接入。

建议新增文件：

```text
internal/core/config_mask.go
internal/core/config_doctor.go
```

修改文件：

```text
internal/core/password_crypto.go
internal/core/config_store.go
internal/core/config_validate.go
```

实施步骤：

1. 扩展 `encryptStorePasswords` 或新增 `encryptConfigPasswords`：
   - host password 加密。
   - auth profile password 加密。
2. 扩展 `decryptStorePasswords` 或新增 `decryptConfigPasswords`：
   - host `password_enc` 解密到内存 `password`。
   - auth profile `password_enc` 解密到内存 `password`。
3. 保存前深拷贝：
   - `Hosts` slice。
   - `AuthProfiles` slice。
   - 不能修改调用方传入对象。
4. 新增 mask：
   - `MaskConfig(config Config) Config`
   - `MaskHost(host Host) Host`
   - `MaskAuthProfile(profile AuthProfile) AuthProfile`
5. 新增 auth label helper：
   - `password`
   - `key`
   - `key+password`
   - `auth:<name>`
6. 新增 doctor 本地检查结果结构：
   - level: `ok/warn/error`
   - item
   - message
7. doctor 初版只做本地静态检查，不连接远端。

验收：

- host 明文 password 保存为 `password_enc`。
- auth profile 明文 password 保存为 `password_enc`。
- 保存后调用方对象仍保留内存 password。
- 缺少 key 解密时报错，不自动创建新 key。
- `MaskConfig` 不输出 `password` 和真实 `password_enc`。
- doctor 能报告重复 host name/IP、重复 auth profile name、缺失 auth_ref、非法 port、非法 host_key_check。

测试建议：

```text
TestSaveConfigEncryptsHostPassword
TestSaveConfigEncryptsAuthProfilePassword
TestSaveConfigDoesNotMutateInput
TestDecryptConfigPasswordMissingKey
TestMaskConfigHidesSensitiveFields
TestDoctorReportsDuplicateHosts
TestDoctorReportsMissingAuthRef
```

验证命令：

```powershell
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal/core
```

提交：

```text
feat(auth): encrypt and mask credential profiles
```

风险点：

- 读取配置时如果 auth profile 密码解密失败，命令应明确失败；不要静默当作无密码继续尝试 key。
- mask 后的结构不能被误保存，否则会把真实密文替换成 `***`。

## P4: cfg 基础命令

目标：

- 新增 `cfg` 顶级管理命令。
- `cfg` 设置 `Category: "Management"`。
- 别名：`config`。
- 初版实现本地配置查看和修改，不连接远端。

命令面：

```bash
sshc cfg path
sshc cfg show
sshc cfg show --raw
sshc cfg get logs_path
sshc cfg set logs_path ./logs
sshc cfg unset logs_path
sshc cfg edit
sshc cfg doctor
```

范围：

```text
internal/command/cfg.go
internal/command/cfg_test.go
internal/bootstrap/init.go
```

实施步骤：

1. 新增 `NewCfgCmd()`。
2. 注册子命令：
   - `path`
   - `show`
   - `get`
   - `set`
   - `unset`
   - `edit`
   - `doctor`
3. `cfg path` 输出实际路径，并标识是否来自 `SSHC_CONFIG`。
4. `cfg show` 默认输出 mask 后 JSON。
5. `cfg show --raw` 输出原始 JSON 结构。
6. `cfg get/set/unset` 初版只支持：
   - `logs_path`
7. `cfg edit`：
   - 优先 `VISUAL`。
   - 其次 `EDITOR`。
   - 找不到编辑器时输出配置路径，不猜测 GUI 编辑器。
8. `cfg doctor` 输出本地检查结果：
   - 无 error 时退出码 0。
   - 有 error 时退出码非 0。
   - warn 不导致失败。

验收：

- `sshc cfg path` 能输出配置路径。
- `sshc cfg show` 不泄露真实密码或密文。
- `sshc cfg show --raw` 明确输出原始配置。
- `sshc cfg set logs_path ./runtime/logs` 后配置生效。
- `sshc cfg unset logs_path` 后恢复默认日志路径。
- `sshc cfg doctor` 能报告配置问题。
- help 中 `cfg` 位于 Management 分组。

测试建议：

```text
TestCfgPathCommand
TestCfgShowMasksSecrets
TestCfgShowRaw
TestCfgSetGetUnsetLogsPath
TestCfgDoctorReturnsErrorForInvalidConfig
```

验证命令：

```powershell
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe cfg --help | Out-String
.\tmp\sshc.exe cfg path
git diff --check -- internal/command internal/bootstrap
```

提交：

```text
feat(config): add cfg management commands
```

风险点：

- `cfg show --raw` 可能输出敏感信息，LongHelp 中必须明确只用于排障。
- `cfg set` 初版不要做 dotted path，避免 JSON path 规则不稳定。

## P5: auth 凭证命令

目标：

- 新增 `auth` 凭证管理命令。
- 别名：`cred`、`creds`。
- `auth` 设置 `Category: "Management"`。
- 密码输入默认隐藏，不支持命令行明文密码值。

命令面：

```bash
sshc auth add dev-root -u root -p
sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519
sshc auth list
sshc auth show dev-root
sshc auth show dev-root --raw
sshc auth rm dev-root
sshc auth rm dev-root --yes
```

范围：

```text
internal/command/auth.go
internal/command/auth_test.go
internal/bootstrap/init.go
internal/core/config.go
internal/core/config_store.go
```

实施步骤：

1. 新增 `NewAuthCmd()`。
2. `auth add`：
   - 第一个参数是 profile name。
   - `-u/--user`。
   - `-p/--password` 作为 bool，触发隐藏输入。
   - 不支持 `-p secret` 或 `--password secret`。
   - `--key` 写入 `key_path`。
   - 至少需要 password 或 key。
3. 密码读取使用当前已有的隐藏读取能力。
4. `auth list`：
   - 展示 name、user、auth type。
   - 不展示密文。
5. `auth show`：
   - 默认 mask。
   - `--raw` 输出原始。
6. `auth rm`：
   - 如果仍有 host 引用该 auth profile，拒绝删除。
   - 初版不提供 `--force`。
   - `--yes` 跳过确认。
7. 删除或更新 auth profile 后保存配置。

验收：

- `auth add dev-root -u root -p` 可隐藏输入密码并加密保存。
- `auth add deploy-key -u deploy --key ~/.ssh/id_ed25519` 不创建 password key。
- `auth add dev-root -u root -p secret` 会报错或拒绝把 `secret` 当明文密码值。
- `auth list` 不泄露密码。
- `auth show` 默认 mask。
- host 通过 `auth_ref` 可以解析到 profile。
- 被 host 引用的 auth profile 不能删除。

测试建议：

```text
TestAuthAddPasswordProfile
TestAuthAddRejectsInlinePasswordValue
TestAuthAddKeyProfile
TestAuthListMasksSecrets
TestAuthShowMasksSecrets
TestAuthRemoveRefusedWhenUsedByHost
TestAuthRemoveWithYes
```

验证命令：

```powershell
go test ./internal/command
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe auth --help | Out-String
git diff --check -- internal/command internal/core internal/bootstrap
```

提交：

```text
feat(auth): add credential profile commands
```

风险点：

- `-p` 在旧 `add` 中是 password string，在 `auth add` 中是 bool 隐藏读取。help 必须明确 `auth add -p` 不接收明文值。
- 如果 gcli 对 bool 短选项后接值的行为宽松，测试需要覆盖误用提示。

## P6: host 管理命令和顶层兼容入口

目标：

- 新增 `host` 管理命令，别名 `hosts`、`h`。
- `host` 设置 `Category: "Management"`。
- 顶层 `add/list/ls` 保持兼容，内部复用 host handler。

命令面：

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host list
sshc host list --group testing
sshc host list --match dev
sshc host list --show-ip
sshc host list --json
sshc host show devhost
sshc host show devhost --raw
sshc host rm devhost
sshc host rm devhost --yes
sshc host rename old-name new-name
```

范围：

```text
internal/command/host.go
internal/command/add.go
internal/command/list.go
internal/command/host_test.go
internal/bootstrap/init.go
```

实施步骤：

1. 抽取当前 `add` handler 为可复用函数。
2. 抽取当前 `list` handler 为可复用函数。
3. 新增 `host add`：
   - 支持当前 add 选项。
   - 新增 `--auth` 写入 `auth_ref`。
   - 保持 `-I`、`--from-clipboard` 行为。
4. 顶层 `add` 复用 `host add`。
5. 新增 `host list`：
   - 复用现有 cliui table。
   - 支持 `--group`。
   - 支持 `--match`。
   - 支持 `--json`。
   - 保持 `--show-ip`。
6. 顶层 `list/ls` 复用 `host list`。
7. 新增 `host show`：
   - 默认 mask。
   - `--raw` 输出原始。
   - `--json` 输出 JSON。
8. 新增 `host rm`：
   - 默认需要确认。
   - `--yes` 跳过确认。
9. 新增 `host rename`：
   - 检查新名称不重复。
   - 保持 ip 不变。

验收：

- 旧命令 `sshc add ...` 可用。
- 旧命令 `sshc list/ls` 可用。
- 新命令 `sshc host add/list/show/rm/rename` 可用。
- `host add --auth dev-root` 会写入 `auth_ref`。
- `host list` 默认仍 mask IP。
- `host show` 默认不泄露密码和密文。
- 删除和 rename 会保存新 config 结构。

测试建议：

```text
TestHostAddWithAuthRef
TestTopLevelAddStillWorks
TestHostListFiltersByGroup
TestHostListMatch
TestHostShowMasksSecrets
TestHostRemoveRequiresYesInNonInteractiveTest
TestHostRename
TestTopLevelListStillWorks
```

验证命令：

```powershell
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe host --help | Out-String
.\tmp\sshc.exe add --help | Out-String
.\tmp\sshc.exe list --help | Out-String
git diff --check -- internal/command internal/bootstrap
```

提交：

```text
feat(host): add host management commands
```

风险点：

- 顶层 add/list 的 help 不能和 host help 产生大量重复维护成本。建议共用 LongHelp 片段或保持顶层 help 简短。
- 删除确认在测试和非交互场景要有清晰路径，避免测试卡住。

## P7: 文档、help 和最终验收

目标：

- 更新 README 和中文 README 的配置、凭证、安全说明。
- 更新 LongHelp，避免重复 option 描述。
- 更新设计/计划状态。
- 做完整回归。

范围：

```text
README.md
README.zh-CN.md
docs/password-encryption-design.md
docs/2026-07-04-sshc-config-auth-design.md
docs/plan/2026-07-04-sshc-config-auth-plan.md
```

实施步骤：

1. README 增加：
   - `sshc cfg path/show/doctor`
   - `sshc host add/list/show`
   - `sshc auth add/list`
   - `host --auth` 使用示例。
2. README 安全说明更新：
   - 本机 key file 模型。
   - `known_hosts` 默认策略。
   - `insecure` 是显式降级。
3. 中文 README 同步。
4. LongHelp 清理：
   - 示例保留常用路径。
   - 不重复 option 本身已经说明的内容。
5. 更新本计划阶段状态。

完整验收命令：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe --help | Out-String
.\tmp\sshc.exe cfg --help | Out-String
.\tmp\sshc.exe host --help | Out-String
.\tmp\sshc.exe auth --help | Out-String
.\tmp\sshc.exe list --help | Out-String
git diff --check -- .
git status --short --branch
```

提交：

```text
docs: update config and auth documentation
```

风险点：

- README 不应变成完整 man page，保持开源项目入口页定位。
- 中英文 README 示例 host 名称继续使用 `devhost`，不要回退成不明确的 `dev`。

## 兼容性要求

实施完成后必须保持：

```bash
sshc add --ip 192.168.1.10 -u root -p password
sshc add -I
sshc list
sshc ls
sshc run devhost -- uptime
sshc login devhost
sshc scp -l ./app.jar -r /tmp/app.jar devhost
sshc download -r /var/log/app.log -l tmp/logs devhost
sshc log devhost
```

新增能力示例：

```bash
sshc cfg path
sshc cfg show
sshc cfg doctor
sshc auth add dev-root -u root -p
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host list --group testing --show-ip
sshc run devhost -- uptime
```

## 回滚策略

如果某阶段实现中发现风险过大：

- P0 gcli 迁移失败：回滚 P0，不进入多级命令实现。
- P1 配置模型失败：保留旧 `Store`，先补充测试，重新拆分 config store。
- P2 effective host 接入影响 run/scp：先只在 core 测试中实现解析，不接入命令。
- P4-P6 命令实现复杂：优先保留 core 模型和解析，命令阶段后移。

每个阶段独立提交后，回滚粒度应能控制在一个提交内。

## 已确认事项

以下事项已按推荐方案确认，实施时不再二次询问：

1. `cfg get/set/unset` 初版只支持 `logs_path`，暂不支持 dotted path。
2. `auth add -p` 只作为隐藏读取开关，不支持 `-p secret` 或 `--password secret`。
3. `auth rm` 被 host 引用时拒绝删除，初版不提供 `--force`。
4. `host rm` 默认需要确认；非交互环境无法确认时直接失败，并提示使用 `--yes`。
5. `known_hosts` 默认切换和 P2 effective host 接入一起落地；需要跳过校验时显式配置 `insecure`。

## 复审结论

本计划按当前设计可以进入实施。复审后调整点：

- `auth` 命令提前到 `host` 管理命令之前，保证 `host add --auth` 有自然的用户流程。
- P1 明确要求继续兼容既有 host `password_enc` 解密，避免配置模型拆分时破坏现有发布前安全改动。
- P2 明确把 `known_hosts` SSH callback 接入纳入验收，不能只解析字段但连接仍使用 insecure callback。
- P5/P6 都补充了对非交互确认和 `-p secret` 误用的测试要求。

后续实施时如果出现比计划更大的连锁改动，应先更新本计划，再进入对应代码阶段。
