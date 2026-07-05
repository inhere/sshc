# sshc 配置管理与凭证模型设计

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-04 | Codex | 初版，聚焦配置结构、凭证复用、敏感字段、安全边界和实施顺序 |
| v0.2 | 2026-07-05 | Codex | 对齐当前实现，cfg get/set/unset 已支持 defaults.* 白名单字段，export/import 转入下一阶段计划 |

## 背景

`sshc` 当前配置已经从 `hosts.json` 演进到固定路径：

```text
~/.config/sshc/sshc.config.json
```

现有模型以 host 为中心：

```json
{
  "logs_path": "logs",
  "hosts": [
    {
      "name": "devhost",
      "ip": "192.168.1.10",
      "user": "root",
      "password_enc": "v1:...",
      "key_path": "~/.ssh/id_ed25519",
      "port": 22
    }
  ]
}
```

这个模型适合少量主机，但继续增加 `cfg`、`host`、`auth`、`jump`、`export/import` 后会遇到几个问题：

- 多台主机复用同一账号或 key 时需要重复写 `user/password_enc/key_path`。
- 全局默认值缺少统一入口，`connect_timeout`、`run_timeout`、`host_key_check` 只能散落在代码默认值里。
- 配置展示、导出、doctor 校验缺少统一的敏感字段处理规则。
- 后续跳板机、批量执行、跨机器导入导出都需要稳定的 effective host 解析规则。

因此先稳定配置管理和凭证模型，再进入命令实现。

## 范围

本设计覆盖：

- `sshc.config.json` 目标结构。
- `defaults`、`hosts`、`auth_profiles` 的字段语义。
- host 与 auth profile 的合并优先级。
- 密码加密、mask、导出导入前置约束。
- `cfg`、`host`、`auth` 命令的职责边界。
- 从当前配置结构到新结构的兼容和迁移方式。

本文不开始实现代码。

## 非目标

- 不支持 `.ssh/password` 或其他明文密码文件。
- 初版不引入 OS keyring；继续使用当前本机 key file 加密模型。
- 初版不强制把所有 host 内联凭证迁移成 auth profile。
- 初版不实现完整 `cfg export/import`，但配置结构要为它预留稳定边界。
- 初版不做多用户权限系统、中心化堡垒机审计或远端同步服务。

## 总体思路

配置分三层：

```text
defaults       全局默认值
auth_profiles  可复用认证资料
hosts          目标主机和主机特有覆盖项
```

运行命令时，不直接把原始 host 当连接配置使用，而是解析成一个 effective host：

```text
命令行显式参数 > host 内联字段 > auth_ref 指向的 auth profile > defaults > 内置默认值
```

这样可以同时满足两类使用方式：

- 简单场景：`sshc add --ip ... -u root -p`，host 内联保存凭证即可。
- 复用场景：`sshc auth add dev-root ...`，多个 host 通过 `auth_ref` 复用凭证。

## 目标配置结构

建议 `sshc.config.json` 目标结构如下：

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
    },
    {
      "name": "deploy-key",
      "user": "deploy",
      "key_path": "~/.ssh/id_ed25519"
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

### 顶层字段

`version`

- 配置结构版本号。
- 新写入的配置固定写 `1`。
- 读取旧配置时，如果缺少 `version`，按 v0 兼容读取，不要求用户手动迁移。

`logs_path`

- run log 存储目录。
- 空值使用 `~/.config/sshc/logs`。
- 相对路径基于 `~/.config/sshc` 解析。
- 绝对路径按原样使用。

`defaults`

- 保存全局默认值。
- 只放真正跨 host 共用的默认连接和运行参数。
- 不放 host 列表，也不放批量执行状态。

`auth_profiles`

- 保存可复用凭证。
- 通过 `name` 被 host 的 `auth_ref` 引用。
- 敏感字段仍使用本机 key file 加密。

`hosts`

- 保存目标主机。
- host 可以内联凭证，也可以通过 `auth_ref` 引用凭证。
- host 内联字段用于覆盖 auth profile 和 defaults。

## 字段模型

### Defaults

建议结构：

```go
type Defaults struct {
    User            string `json:"user,omitempty"`
    Port            int    `json:"port,omitempty"`
    ConnectTimeout  string `json:"connect_timeout,omitempty"`
    RunTimeout      string `json:"run_timeout,omitempty"`
    RemoteScriptDir string `json:"remote_script_dir,omitempty"`
    HostKeyCheck    string `json:"host_key_check,omitempty"`
    KnownHostsPath  string `json:"known_hosts_path,omitempty"`
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `user` | 默认 SSH 用户；host 和 auth profile 都未设置 user 时使用 |
| `port` | 默认 SSH 端口；内置默认值是 `22` |
| `connect_timeout` | 建立 SSH 连接超时；内置默认值建议沿用当前 `20s` |
| `run_timeout` | `run` 默认命令超时；空值表示不主动注入远端 `timeout` |
| `remote_script_dir` | `run --script` 默认远端临时目录；内置默认值 `/tmp` |
| `host_key_check` | host key 校验模式，允许 `known_hosts` 或 `insecure` |
| `known_hosts_path` | known hosts 文件路径，默认 `~/.ssh/known_hosts` |

### AuthProfile

建议结构：

```go
type AuthProfile struct {
    Name        string `json:"name"`
    User        string `json:"user,omitempty"`
    Password    string `json:"password,omitempty"`
    PasswordEnc string `json:"password_enc,omitempty"`
    KeyPath     string `json:"key_path,omitempty"`
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `name` | profile 名称，配置内唯一 |
| `user` | 该凭证对应的 SSH 用户 |
| `password` | 只为兼容读取和内存态存在，新写入配置不落明文 |
| `password_enc` | AES-256-GCM 加密后的密码 |
| `key_path` | SSH private key 路径 |

认证类型允许：

- password only
- key only
- key + password

如果同时配置 key 和 password，保持当前行为：优先尝试 key，再尝试 password。

### Host

建议在当前 `Host` 上增加字段：

```go
type Host struct {
    Name        string `json:"name"`
    IP          string `json:"ip"`
    AuthRef     string `json:"auth_ref,omitempty"`
    User        string `json:"user,omitempty"`
    Password    string `json:"password,omitempty"`
    PasswordEnc string `json:"password_enc,omitempty"`
    KeyPath     string `json:"key_path,omitempty"`
    Remark      string `json:"remark,omitempty"`
    Group       string `json:"group,omitempty"`
    Port        int    `json:"port,omitempty"`
    Jump        string `json:"jump,omitempty"`
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `name` | 主机别名；为空时可以继续用 `ip` 作为名称 |
| `ip` | SSH 目标地址，不包含端口 |
| `auth_ref` | 引用 `auth_profiles[].name` |
| `user/password/password_enc/key_path` | host 内联认证字段，优先级高于 `auth_ref` |
| `remark` | 备注 |
| `group` | 分组；空值展示为 `default` |
| `port` | SSH 端口；空值或 0 走默认值 |
| `jump` | 后续跳板机字段，含义是 `local -> jump -> current host` |

`jump` 在本阶段只稳定字段名和方向，不要求立即实现连接逻辑。

## Effective Host 合并规则

运行时新增解析结果，避免把磁盘配置结构直接传入 SSH client：

```go
type EffectiveHost struct {
    Name            string
    IP              string
    User            string
    Password        string
    PasswordEnc     string
    KeyPath         string
    Remark          string
    Group           string
    Port            int
    Jump            string
    ConnectTimeout  time.Duration
    RunTimeout      time.Duration
    RemoteScriptDir string
    HostKeyCheck    string
    KnownHostsPath  string
}
```

解析顺序：

1. 从内置默认值开始：
   - `port=22`
   - `connect_timeout=20s`
   - `remote_script_dir=/tmp`
   - `host_key_check=known_hosts`
   - `known_hosts_path=~/.ssh/known_hosts`
2. 应用 `defaults` 中非空字段。
3. 如果 host 设置了 `auth_ref`，找到对应 auth profile，并应用 profile 字段。
4. 应用 host 内联字段。
5. 应用命令行显式参数。
6. 校验最终结果。

关键规则：

- host 内联 `user/key_path/password_enc/password` 覆盖 auth profile。
- host `port` 为 0 时不覆盖默认值。
- `group` 为空只影响展示，effective 值使用 `default`。
- 解析后连接层只消费 effective host，不再关心字段来自哪里。

## 校验规则

### 配置级校验

`cfg doctor` 和保存配置前应覆盖：

- `version` 是支持的版本。
- `logs_path` 可解析，目录可创建或可写。
- host `name` 非空或 `ip` 非空。
- host `ip` 不包含端口；端口必须写入 `port`。
- host name 在配置内唯一。
- host ip 在配置内建议唯一；考虑到当前 `Upsert` 按 name 或 IP 更新，初版继续保持 name/IP 都不重复。
- auth profile name 唯一。
- host `auth_ref` 非空时必须指向存在的 auth profile。
- `port` 范围为 `1..65535`。
- `host_key_check` 只能是 `known_hosts` 或 `insecure`。
- `known_hosts_path` 只在 `host_key_check=known_hosts` 时有意义。

### Effective Host 校验

解析为 effective host 后必须满足：

- `ip` 非空。
- `user` 非空。
- 至少存在一种认证方式：`password`、`password_enc` 或 `key_path`。
- `port` 合法。
- `connect_timeout` 合法。
- 如果 `host_key_check=known_hosts`，known hosts 文件路径可解析。

## 敏感字段处理

敏感字段：

- `password`
- `password_enc`
- 后续可能新增的 `key_passphrase`
- 后续可能新增的 `key_passphrase_enc`
- export/import 过程中短暂出现的明文配置内容

默认展示策略：

```json
{
  "password_enc": "***"
}
```

命令要求：

- `cfg show` 默认 mask，`cfg show --raw` 才输出原始内容。
- `host show` 默认 mask，`host show --raw` 才输出原始内容。
- `auth show` 默认 mask，`auth show --raw` 才输出原始内容。
- `list` 和 `auth list` 不展示密文，只展示认证类型，例如 `password`、`key`、`key+password`。
- 日志不记录 password、password_enc。

`--raw` 是排障能力，不建议放在 README 快速示例里重点展示。

## 加密模型

继续沿用当前模型：

```text
~/.config/sshc/key
```

- 首次保存带密码的配置时创建本机随机 key。
- 密码使用 AES-256-GCM 加密。
- 密文格式继续使用 `v1:<base64(nonce+ciphertext)>`。
- 读取旧明文 `password` 仍兼容。
- 新写入时不落明文 `password`。

保存规则：

1. 保存前深拷贝 store，不能为了落盘加密修改调用方内存对象。
2. host 内联 `password` 非空时，加密到 `password_enc`，并清空落盘副本的 `password`。
3. auth profile `password` 非空时，同样加密到 `password_enc`。
4. 仅使用 key auth 时不创建 password key file。

读取规则：

1. 如果 `password` 非空，直接作为内存态密码使用。
2. 如果 `password` 为空且 `password_enc` 非空，用本机 key 解密。
3. 解密缺少 key 或密文损坏时显式报错。
4. 解密不应自动创建新 key。

安全边界：

- 该模型避免默认明文写配置，但不是 OS keyring 级安全。
- 攻击者同时拿到 `sshc.config.json` 和 `key` 时可以解密。
- key 丢失后既有 `password_enc` 无法恢复。
- 后续如要增强，可独立设计 `password_ref` + OS keyring，不和本阶段耦合。

## 命令职责

### cfg

`cfg` 管理整个配置文件和全局配置：

```bash
sshc cfg path
sshc cfg show
sshc cfg get logs_path
sshc cfg set logs_path ./logs
sshc cfg set defaults.user root
sshc cfg set defaults.port 2222
sshc cfg set defaults.host_key_check known_hosts
sshc cfg unset logs_path
sshc cfg edit
sshc cfg doctor
sshc cfg export
sshc cfg import
```

当前实现：

- `cfg get/set/unset` 支持 `logs_path` 和 `defaults.*` 白名单字段。
- `defaults.*` 不实现为通用 JSON path，只允许明确字段，避免任意写入破坏配置结构。
- `cfg export/import` 保留在设计中，转入独立实施计划。

### host

`host` 管理主机资源：

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host list
sshc host show devhost
sshc host rm devhost
sshc host rename old-name new-name
sshc host edit devhost
```

兼容快捷入口继续保留：

```bash
sshc add ...
sshc list
sshc ls
```

这些快捷入口复用 `host add/list` handler。

### auth

`auth` 管理可复用凭证：

```bash
sshc auth add dev-root -u root -p
sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519
sshc auth list
sshc auth show dev-root
sshc auth rm dev-root
```

规则：

- `auth add -p` 使用隐藏输入读取密码。
- 不鼓励 `--password VALUE`，避免 shell history 泄漏。
- 如确需脚本能力，后续可以设计 `--password-stdin`，不在初版做。

## Host Key 策略

项目尚未发布，可以接受破坏性安全默认值。

默认：

```json
{
  "defaults": {
    "host_key_check": "known_hosts",
    "known_hosts_path": "~/.ssh/known_hosts"
  }
}
```

含义：

- `sshc` 连接目标 host 时，目标 host key 必须能在 known hosts 文件中匹配。
- 如果目标不在 known hosts 中，连接失败并提示用户先用 OpenSSH 或 `ssh-keyscan` 建立信任。
- 临时环境可以显式降级：

```json
{
  "defaults": {
    "host_key_check": "insecure"
  }
}
```

`insecure` 仅作为显式选择，不作为默认值。

## 迁移策略

### 读取兼容

当前结构：

```json
{
  "logs_path": "logs",
  "hosts": []
}
```

新结构会增加 `version`、`defaults`、`auth_profiles`，但旧配置仍可直接反序列化：

- 缺少 `version` 时按 v0 处理。
- 缺少 `defaults` 时使用内置默认值。
- 缺少 `auth_profiles` 时视为空列表。
- host 内联 `user/password_enc/key_path` 继续可用。
- legacy `hosts.json` 读取兼容继续保留。

### 写入新结构

一旦执行会保存配置的命令，写入新结构：

```json
{
  "version": 1,
  "logs_path": "...",
  "hosts": [...]
}
```

注意：

- 不强制把 host 内联凭证抽成 auth profile。
- 不自动删除 host 内联 `user/key_path/password_enc`。
- `auth_profiles` 为空时可以写 `[]`，也可以 `omitempty`；建议写 `[]`，让结构更直观。

### Upsert 行为

当前 `Upsert` 命中相同 name 或 IP 就更新。为减少行为变化，初版继续保持：

```text
same name OR same ip => update
```

但 `cfg doctor` 应明确报告重复 name/IP，因为重复会让 `run/list/rename` 行为变得不可预测。

## 模块划分建议

为避免 `store.go` 继续膨胀，建议后续实现时拆分：

```text
internal/core/config.go          Config/Defaults/AuthProfile/Host 结构
internal/core/config_store.go    LoadConfig/SaveConfig/路径/legacy 读取
internal/core/config_resolve.go  EffectiveHost 解析和校验
internal/core/config_mask.go     敏感字段 mask/raw 输出
internal/core/password_crypto.go 密码加解密，扩展 auth profile 支持
```

兼容层：

- 短期可以保留 `LoadStore/SaveStore`，内部转调新 `LoadConfig/SaveConfig`。
- 命令逐步改到新模型后，再决定是否删除旧命名。

## 测试计划

必须覆盖：

- 旧结构配置读取成功。
- 新结构配置读取成功。
- 空配置返回默认结构。
- `SSHC_CONFIG` 覆盖路径仍有效。
- legacy `hosts.json` 兼容读取仍有效。
- `SaveConfig` 写入 `version: 1`。
- host 内联 password 保存为 `password_enc`。
- auth profile password 保存为 `password_enc`。
- 保存加密不修改调用方内存对象。
- 缺少 key 解密 `password_enc` 时显式报错。
- effective host 合并优先级正确。
- host `auth_ref` 缺失时报错。
- `cfg show/host show/auth show` 默认 mask。
- `host_key_check` 默认是 `known_hosts`。
- `insecure` 只能显式配置。

## 实施顺序

建议按小阶段提交，不要等全部完成后一次提交。

### P0: gcli/v3 迁移

目的：

- 支持多级命令。
- 支持管理命令 category。
- 为 `cfg/host/auth` 建立命令树。

提交建议：

```text
refactor: migrate cli to gcli v3
```

### P1: 配置模型与兼容读取

范围：

- 新增 `Config`、`Defaults`、`AuthProfile`。
- `LoadConfig/SaveConfig`。
- legacy `Store` 兼容。
- 新配置写入 `version: 1`。

提交建议：

```text
feat(config): add versioned config model
```

### P2: Effective Host 解析

范围：

- 新增 `ResolveEffectiveHost`。
- 实现 defaults/auth_ref/host/flags 合并。
- 连接层逐步改用 effective host。

提交建议：

```text
feat(config): resolve effective host settings
```

### P3: 敏感字段与加密扩展

范围：

- auth profile password 加密保存。
- mask helper。
- 保证保存不修改调用方对象。

提交建议：

```text
feat(auth): encrypt and mask credential profiles
```

### P4: cfg 基础命令

范围：

- `cfg path/show/get/set/unset/edit/doctor`。
- `show` 默认 mask。
- `doctor` 做本地静态检查，不连接远端。

提交建议：

```text
feat(config): add cfg management commands
```

### P5: host/auth 命令

范围：

- `host add/list/show/rm/rename`。
- `auth add/list/show/rm`。
- 顶层 `add/list` 兼容入口复用 handler。

提交建议：

```text
feat(auth): add reusable credential commands
```

## 待确认事项

1. `cfg export/import` 是否默认自动生成一次性 key。建议初版自动生成，减少用户自行选择弱口令的风险。
2. import 默认合并策略是否拒绝冲突。建议默认 `--merge` 且冲突拒绝，显式 `--overwrite` 才覆盖。
3. 导入前配置备份放置位置。建议放到配置目录 `backups/` 下，文件名包含时间戳。

## 结论

配置管理和凭证模型已经完成，下一步推荐推进 `cfg export/import`。

核心落点是：

- `sshc.config.json` 升级为 versioned config。
- `defaults/auth_profiles/hosts` 三层稳定。
- 运行时统一解析 effective host。
- 密码继续使用本机 key file 加密，扩展覆盖 auth profile。
- `cfg/host/auth` 命令只消费同一套配置模型。

这套模型完成后，后续 `batch-run`、`jump`、`cfg export/import` 都可以复用同一套解析和校验逻辑，避免反复迁移配置结构。
