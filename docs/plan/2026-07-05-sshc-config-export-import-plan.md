# sshc cfg export/import 实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-05 | Codex | 初版，基于当前配置和凭证模型拆分 cfg export/import 的实现阶段、提交边界和验收项 |
| v0.2 | 2026-07-05 | Codex | 对齐 `host import` 已完成后的边界，补充 cfg export/import 已确认决策 |

## 关联文档

- 后续能力设计：`docs/2026-07-04-sshc-next-features-design.md`
- 配置与凭证模型设计：`docs/2026-07-04-sshc-config-auth-design.md`
- 密码加密说明：`docs/password-encryption-design.md`

## 背景

当前 `sshc` 配置文件默认在：

```text
~/.config/sshc/sshc.config.json
```

密码字段保存为 `password_enc`，依赖本机：

```text
~/.config/sshc/key
```

这解决了默认明文保存问题，但也带来跨机器迁移问题：直接复制
`sshc.config.json` 到另一台机器后，目标机器没有源机器的 key，无法解密旧的
`password_enc`。因此需要一个显式的可迁移导出包：

- 导出时在源机器解密配置内密码到内存。
- 使用一次性 export key 加密整个导出包。
- 导入时用 export key 解密导出包。
- 使用目标机器本地 key 重新加密 password。

`cfg export/import` 操作整个配置文件，不只操作 hosts。这样可以同时迁移：

- `logs_path`
- `defaults`
- `auth_profiles`
- `hosts`

已有 `ips`、`plain` KV 文本、CSV、剪贴板 hosts 清单的批量导入不属于本计划，已由
`sshc host import` 提供。详细计划和实现状态见：

```text
docs/plan/2026-07-05-sshc-host-import-plan.md
```

## 目标

- 新增 `sshc cfg export -o FILE`。
- 新增 `sshc cfg import -f FILE --key KEY`。
- 导出包使用独立格式版本，便于未来兼容。
- 导出包整体加密，不落明文 JSON。
- export key 不写入导出包，只打印到命令输出。
- import 前自动备份当前配置。
- import 后敏感字段使用目标机器本地 key 重新加密。
- 支持 merge/replace/overwrite 三类导入策略。
- 每个阶段独立验证、独立提交。

## 已确认事项

- 初版使用自动生成的一次性 export key，格式为 `sshc-v1:<base64url random 32 bytes>`。
- 初版不做用户 passphrase 输入；passphrase 模式作为后续扩展。
- import 默认策略为 `--merge`，遇到同名 host、同 IP host 或同名 auth profile 时拒绝导入。
- 只有显式 `--overwrite` 或 `--replace` 才覆盖已有配置。
- 导入前备份到配置目录下的 `backups/`，初版不自动清理旧备份。

## 非目标

- 不实现云同步、远端配置仓库或中心化配置服务。
- 不读取 `.ssh/password` 或其他明文密码文件。
- 不引入 OS keyring。
- 不导出本机 `~/.config/sshc/key`。
- 不把 export key 写入日志或配置文件。
- 不支持只导出单个 host/auth profile；初版只导出整个 config。
- 不支持导出包追加写入或增量更新。
- 不支持普通 `ips`、`plain` KV 文本、CSV 或剪贴板 hosts 清单导入；该能力已由 `host import` 提供。

## 命令面

### export

```bash
sshc cfg export -o sshc-export.enc
sshc cfg export --output sshc-export.enc
```

输出示例：

```text
exported config to sshc-export.enc
export key: sshc-v1:...
```

规则：

- `-o/--output` 必填。
- 输出文件已存在时默认拒绝覆盖。
- `--force` 可覆盖已存在文件。
- 生成随机一次性 export key。
- export key 只输出一次，用户需要自行保存。
- 不在 run log 中记录 export key。
- 不读取或复用 `host import` 的 `ips/plain/csv` 输入格式。

### import

```bash
sshc cfg import -f sshc-export.enc --key "sshc-v1:..."
sshc cfg import --file sshc-export.enc --key "sshc-v1:..." --merge
sshc cfg import --file sshc-export.enc --key "sshc-v1:..." --overwrite
sshc cfg import --file sshc-export.enc --key "sshc-v1:..." --replace
```

规则：

- `-f/--file` 必填。
- `--key` 必填。
- 默认策略是 `--merge`。
- `--merge` 遇到同名 host、同 IP host、同名 auth profile 时拒绝导入。
- `--overwrite` 合并配置并覆盖冲突的 host/auth profile。
- `--replace` 用导入配置整体替换当前配置。
- 导入前自动备份当前配置。
- 导入成功后输出导入统计和备份路径。

## 导出包格式

建议导出文件本身是 JSON，字段中只有密文：

```json
{
  "version": 1,
  "cipher": "AES-256-GCM",
  "kdf": "HKDF-SHA256",
  "created_at": "2026-07-05T10:00:00.000",
  "nonce": "base64...",
  "payload": "base64..."
}
```

`payload` 解密后是导出 payload JSON：

```json
{
  "version": 1,
  "app": "sshc",
  "config_version": 1,
  "exported_at": "2026-07-05T10:00:00.000",
  "config": {
    "version": 1,
    "logs_path": "logs",
    "defaults": {},
    "auth_profiles": [],
    "hosts": []
  }
}
```

说明：

- 外层 `version` 是导出包格式版本，不等同于 config version。
- `payload` 中的 `config` 是内存态配置，password 可为明文字段。
- 保存导入后的目标配置时，现有 `SaveConfig` 会重新写入 `password_enc` 并清空落盘副本 password。
- 时间格式使用当前项目日志时间风格：毫秒精度，不带时区偏移。

## export key 格式

建议 export key 字符串：

```text
sshc-v1:<base64url random 32 bytes>
```

规则：

- 只接受 `sshc-v1:` 前缀。
- 原始随机 key 至少 32 bytes。
- 用 HKDF-SHA256 派生 AES-256-GCM key，info 固定为 `sshc config export v1`。
- 后续如果引入用户 passphrase，可新增 `sshc-pw-v1:` 前缀，不和随机 key 混用。

## 合并策略

### merge

默认策略。行为：

- 顶层 `logs_path`：如果当前为空且导入值非空，则写入；当前非空则保留当前值。
- `defaults`：按字段合并，当前字段为空/0 时才使用导入字段。
- `auth_profiles`：同名冲突则报错。
- `hosts`：同名或同 IP 冲突则报错。

适用场景：

- 在已有配置上追加另一台机器的 hosts/auth。
- 不希望导入操作改变当前已有配置。

### overwrite

行为：

- 顶层 `logs_path`：导入值非空则覆盖。
- `defaults`：导入字段非空/非 0 则覆盖当前字段。
- `auth_profiles`：同名覆盖，未冲突追加。
- `hosts`：同名或同 IP 覆盖，未冲突追加。

适用场景：

- 已确认导出包是更新版本，希望覆盖同名条目。

### replace

行为：

- 用导入配置整体替换当前配置。
- 导入前仍备份当前配置。
- 保存时仍使用目标机器本地 key 重新加密敏感字段。

适用场景：

- 新机器初始化。
- 明确希望完全恢复导出时配置。

互斥规则：

- `--merge`、`--overwrite`、`--replace` 三者最多设置一个。
- 未设置时默认 `--merge`。

## 备份策略

导入前备份当前配置文件：

```text
~/.config/sshc/backups/sshc.config.20260705-100000.json
```

规则：

- 当前配置文件不存在时不创建备份，但输出 `backup: none`。
- 备份目录权限 `0700`。
- 备份文件权限 `0600`。
- 备份是原始磁盘文件内容，不解密、不重写。
- 初版不自动清理旧备份。

## 代码落点

建议新增：

```text
internal/core/config_export.go
internal/core/config_export_test.go
```

建议修改：

```text
internal/command/cfg.go
internal/command/cfg_test.go
README.md
README.zh-CN.md
docs/TODO.md
docs/2026-07-04-sshc-next-features-design.md
```

### core 层建议 API

```go
type ConfigExportFile struct {
    Version   int    `json:"version"`
    Cipher    string `json:"cipher"`
    KDF       string `json:"kdf"`
    CreatedAt string `json:"created_at"`
    Nonce     string `json:"nonce"`
    Payload   string `json:"payload"`
}

type ConfigExportPayload struct {
    Version       int    `json:"version"`
    App           string `json:"app"`
    ConfigVersion int    `json:"config_version"`
    ExportedAt    string `json:"exported_at"`
    Config        Config `json:"config"`
}

type ImportStrategy string

const (
    ImportMerge     ImportStrategy = "merge"
    ImportOverwrite ImportStrategy = "overwrite"
    ImportReplace   ImportStrategy = "replace"
)

type ImportResult struct {
    BackupPath   string
    HostsAdded   int
    HostsUpdated int
    AuthAdded    int
    AuthUpdated  int
}
```

建议函数：

```go
func GenerateExportKey() (string, error)
func EncryptConfigExport(config Config, key string, now time.Time) ([]byte, error)
func DecryptConfigExport(data []byte, key string) (Config, error)
func MergeImportedConfig(current, imported Config, strategy ImportStrategy) (Config, ImportResult, error)
func BackupConfigFile(now time.Time) (string, error)
```

说明：

- `EncryptConfigExport` 接收已经 `LoadConfig()` 后的内存态 config。
- `LoadConfig()` 会把 password_enc 解密成 password，导出 payload 因此可由目标机器重新加密。
- `DecryptConfigExport` 只返回内存态 config，不直接写文件。
- `SaveConfig()` 继续负责目标机器本地加密。

## 阶段计划

### P1: 导出包加密与合并核心

目标：

- 实现 export key、导出包加解密、导入合并策略和配置备份 helper。
- 暂不接命令。

范围：

```text
internal/core/config_export.go
internal/core/config_export_test.go
```

实现：

1. 新增 export key 生成和解析。
2. 新增 HKDF-SHA256 派生 AES-GCM key。
3. 新增导出包加密。
4. 新增导出包解密。
5. 新增 `merge/overwrite/replace` 合并函数。
6. 新增备份路径和备份文件 helper。
7. 导出包时间使用毫秒格式，不带时区偏移。

测试：

```text
TestGenerateExportKey
TestEncryptDecryptConfigExport
TestDecryptConfigExportRejectsWrongKey
TestDecryptConfigExportRejectsBadVersion
TestMergeImportedConfigRejectsConflicts
TestMergeImportedConfigKeepsExistingValues
TestOverwriteImportedConfigUpdatesConflicts
TestReplaceImportedConfigReplacesAll
TestBackupConfigFile
```

验证：

```powershell
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal/core
```

提交：

```text
feat(config): add encrypted config export format
```

### P2: cfg export 命令

目标：

- 新增 `sshc cfg export`。
- 可以生成加密导出文件和一次性 key。

范围：

```text
internal/command/cfg.go
internal/command/cfg_test.go
```

命令：

```bash
sshc cfg export -o sshc-export.enc
sshc cfg export -o sshc-export.enc --force
```

实现：

1. `cfg` 注册 `export` 子命令。
2. `-o/--output` 必填。
3. 输出文件存在且未设置 `--force` 时报错。
4. 调用 `core.LoadConfig()` 获取内存态配置。
5. 生成 export key。
6. 写入导出文件，权限 `0600`。
7. 输出导出路径和 export key。

测试：

```text
TestCfgExportWritesEncryptedFile
TestCfgExportPrintsExportKey
TestCfgExportRejectsExistingFileWithoutForce
TestCfgExportForceOverwritesFile
TestCfgExportRequiresOutput
```

验证：

```powershell
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe cfg export --help | Out-String
git diff --check -- internal/command
```

提交：

```text
feat(cfg): export encrypted config
```

### P3: cfg import 命令

目标：

- 新增 `sshc cfg import`。
- 支持 `merge/overwrite/replace`。
- 导入前自动备份当前配置。

范围：

```text
internal/command/cfg.go
internal/command/cfg_test.go
```

命令：

```bash
sshc cfg import -f sshc-export.enc --key "sshc-v1:..."
sshc cfg import -f sshc-export.enc --key "sshc-v1:..." --overwrite
sshc cfg import -f sshc-export.enc --key "sshc-v1:..." --replace
```

实现：

1. `cfg` 注册 `import` 子命令。
2. `-f/--file` 必填。
3. `--key` 必填。
4. `--merge/--overwrite/--replace` 互斥，默认 merge。
5. 读取导出文件并解密。
6. 读取当前配置。
7. 根据策略合并。
8. 保存前备份当前配置文件。
9. 调用 `core.SaveConfig()` 写入合并后配置。
10. 输出备份路径和导入统计。

测试：

```text
TestCfgImportMergeAddsEntries
TestCfgImportMergeRejectsConflicts
TestCfgImportOverwriteUpdatesEntries
TestCfgImportReplaceConfig
TestCfgImportRequiresFileAndKey
TestCfgImportRejectsMultipleStrategies
TestCfgImportBacksUpExistingConfig
TestCfgImportReencryptsPasswordsWithLocalKey
```

验证：

```powershell
go test ./internal/command
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe cfg import --help | Out-String
git diff --check -- internal/command internal/core
```

提交：

```text
feat(cfg): import encrypted config
```

### P4: 文档和 TODO 收口

目标：

- README、中文 README、TODO、后续能力设计更新。

范围：

```text
README.md
README.zh-CN.md
docs/TODO.md
docs/2026-07-04-sshc-next-features-design.md
docs/plan/2026-07-05-sshc-config-export-import-plan.md
```

实现：

1. README 增加 export/import 用法。
2. 中文 README 同步。
3. TODO 标记导入导出完成。
4. 后续能力设计标记 P7 完成，并把下一步推荐调整到 P8。
5. 本计划更新各阶段状态。

验证：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe cfg --help | Out-String
.\tmp\sshc.exe cfg export --help | Out-String
.\tmp\sshc.exe cfg import --help | Out-String
git diff --check -- README.md README.zh-CN.md docs
```

提交：

```text
docs: document config export and import
```

## 完整验收

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe cfg --help | Out-String
.\tmp\sshc.exe cfg export --help | Out-String
.\tmp\sshc.exe cfg import --help | Out-String
git diff --check -- .
git status --short --branch
```

建议手工 smoke：

```powershell
$env:SSHC_CONFIG = "$PWD\tmp\export-src.json"
.\tmp\sshc.exe auth add dev-root -u root -p
.\tmp\sshc.exe host add --ip 10.0.0.8 --name devhost --auth dev-root
.\tmp\sshc.exe cfg export -o tmp\sshc-export.enc

$env:SSHC_CONFIG = "$PWD\tmp\export-dst.json"
.\tmp\sshc.exe cfg import -f tmp\sshc-export.enc --key "<printed-key>"
.\tmp\sshc.exe cfg show
.\tmp\sshc.exe cfg doctor
```

## 风险与处理

| 风险 | 处理 |
| --- | --- |
| export key 遗失导致导出包不可恢复 | 明确输出一次性 key，并在 README 提醒用户保存 |
| 导出包被误认为包含本机 key | 文档明确不导出 `~/.config/sshc/key`，导入时重新使用目标机器本地 key |
| merge 策略误覆盖当前配置 | 默认 merge 遇冲突拒绝，只有 `--overwrite` 或 `--replace` 才覆盖 |
| 导入失败破坏当前配置 | 导入前先备份，且先完成解密/合并校验再保存 |
| mask 后配置被导出 | export 必须使用 `LoadConfig()` 原始内存态，不使用 `MaskConfig()` |
| 导出包里包含明文 payload | 外层文件只写加密 payload；测试断言导出文件不含 host password 明文 |
| 导入后 password 没有重新加密 | 测试读取磁盘 raw config，断言只有 `password_enc`，无明文 `password` |
| 导出包格式未来不兼容 | 外层和 payload 都带 version，未知版本明确报错 |

## 后续扩展

- 用户输入 passphrase 模式。
- `--key-file` 从文件读取 export key。
- `--stdout` 输出导出包到 stdout。
- `--dry-run` 预览导入冲突和统计。
- 只导出指定 group、host 或 auth profile。
- 导入时自动重命名冲突 host。

这些不进入本轮实现。
