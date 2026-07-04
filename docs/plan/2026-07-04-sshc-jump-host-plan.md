# sshc jump host 跳板连接实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-04 | Codex | 初版，基于后续能力设计拆分 jump host 连接实现阶段、提交边界和验收项 |

## 关联文档

- 后续能力设计：`docs/2026-07-04-sshc-next-features-design.md`
- 配置与凭证模型设计：`docs/2026-07-04-sshc-config-auth-design.md`
- batch-run 计划：`docs/plan/2026-07-04-sshc-batch-run-plan.md`

## 背景

当前 `sshc` 已经有：

- versioned config。
- `Host.Jump string` 字段预留。
- effective host 解析。
- `known_hosts` 默认 host key 校验。
- `run/login/scp/download` 四类 SSH 连接能力。

还缺标准 jump host 连接能力。目标方向已经确认：

```text
local -> jump host -> target host
```

即 `jump` 字段配置在最终目标 host 上，表示访问该 target 时通过哪个跳板。

示例：

```json
{
  "hosts": [
    {
      "name": "bastion",
      "ip": "1.2.3.4",
      "auth_ref": "ops"
    },
    {
      "name": "inner-db",
      "ip": "10.0.0.8",
      "auth_ref": "ops",
      "jump": "bastion"
    }
  ]
}
```

运行：

```bash
sshc login inner-db
sshc run inner-db -- hostname
```

等价于：

```bash
ssh -J bastion inner-db hostname
```

## 目标

- 支持 host 配置字段 `jump`。
- 支持命令行覆盖 `--jump`。
- 覆盖以下命令：
  - `run`
  - `login/connect`
  - `scp/upload`
  - `download/dl`
- jump host 和 target host 都走 effective host 解析。
- jump host 和 target host 都执行 host key 校验。
- 初版只支持一级 jump。
- 显式拒绝循环 jump。
- 不影响无 jump 的现有连接路径。
- 分阶段实现、验证和提交。

## 非目标

- 不支持多级跳板链，例如 `local -> b1 -> b2 -> target`。
- 不支持 OpenSSH `ProxyCommand`。
- 不支持 PVE/LXC/vhost 专用执行器。
- 不支持经 jump host 进行端口转发或 socks 代理。
- 不实现 jump host 的交互选择。
- 不实现 jump host 连接复用池。

## 命令面

### 配置默认 jump

```json
{
  "name": "inner-db",
  "ip": "10.0.0.8",
  "auth_ref": "ops",
  "jump": "bastion"
}
```

使用：

```bash
sshc login inner-db
sshc run inner-db -- hostname
sshc scp -l app.jar -r /tmp/app.jar inner-db
sshc download -r /var/log/app.log -l tmp/logs inner-db
```

### 命令行覆盖 jump

```bash
sshc login inner-db --jump bastion
sshc run inner-db --jump bastion -- hostname
sshc scp -l app.jar -r /tmp/app.jar inner-db --jump bastion
sshc download -r /var/log/app.log -l tmp/logs inner-db --jump bastion
```

规则：

- `--jump` 非空时覆盖 host 配置里的 `jump`。
- `--jump none` 或空字符串禁用 jump 暂不支持，避免语义复杂；如需要禁用，后续可设计 `--no-jump`。
- `--jump` target 使用 host name/IP/唯一部分匹配解析。
- jump host 本身不能再有 `jump`，初版直接报错。

## 配置与解析规则

### 解析顺序

目标 host：

```text
target argument -> target effective host -> jump override/config -> jump effective host
```

命令行覆盖顺序：

```text
--jump > target host.jump > empty
```

### 循环检测

必须拒绝：

```json
{"name":"a","jump":"a"}
{"name":"a","jump":"b"}
{"name":"b","jump":"a"}
```

初版只支持一级 jump，因此如果 jump host 自己带 jump，也报错：

```text
jump host "bastion" also has jump "outer"; multi-level jump is not supported
```

### known_hosts

两段连接都要校验：

1. local -> jump host
2. jump host -> target host

默认仍使用：

```text
known_hosts_path = ~/.ssh/known_hosts
host_key_check = known_hosts
```

说明：

- target 的 host key 校验发生在本地进程中，不是在 jump host 上读取 known_hosts。
- target 校验使用 target 的 IP/host 和 port。
- 如果 target 使用内网 IP，例如 `10.0.0.8`，本机 `~/.ssh/known_hosts` 需要有该地址或对应 host name 的记录。
- 如要跳过校验，必须显式配置 target 或 defaults 的 `host_key_check=insecure`。

## 代码设计

当前连接入口：

```go
func newSSHClient(host Host) (*goph.Client, error)
```

需要演进为可以基于 jump dial target。

建议新增结构：

```go
type SSHConnectOptions struct {
    Jump *Host
}
```

或更直接：

```go
func newSSHClient(host Host) (*goph.Client, error)
func newSSHClientViaJump(host Host, jump Host) (*goph.Client, error)
```

初版建议第二种，改动更小。

### 无 jump 路径

保持现有：

```text
ssh.Dial("tcp", targetAddr, targetClientConfig)
```

### jump 路径

连接流程：

```text
1. newSSHClient(jump)
2. jumpClient.Dial("tcp", net.JoinHostPort(target.IP, target.Port))
3. ssh.NewClientConn(conn, targetAddr, targetClientConfig)
4. ssh.NewClient(clientConn, chans, reqs)
5. 包装为 *goph.Client 或替代本地 client wrapper
```

需要确认 `goph.Client` 可直接构造：

```go
type Client struct {
    *ssh.Client
    Config *Config
}
```

如果可行：

```go
return &goph.Client{
    Client: ssh.NewClient(conn, chans, reqs),
    Config: &goph.Config{...},
}, nil
```

如果 goph 内部限制较多，则新增本地轻量接口：

```go
type SSHClient interface {
    Run(string) ([]byte, error)
    RunContext(context.Context, string) ([]byte, error)
    NewSession() (*ssh.Session, error)
    NewSftp(...sftp.ClientOption) (*sftp.Client, error)
    Close() error
}
```

但初版应优先保持 `*goph.Client`，避免扩大改动。

### Close 顺序

jump 路径需要关闭两个连接：

```text
target client close
jump client close
```

如果只返回 target client，jump client 可能泄漏。建议设计 wrapper：

```go
type SSHSessionClient struct {
    *goph.Client
    closer func() error
}
```

但现有代码签名直接用 `*goph.Client`。更保守方案：

- `newSSHClient` 返回本地接口或 wrapper。
- P2.1 先抽象现有连接使用点，P2.2 再接 jump。

推荐方案：

```go
type RemoteClient interface {
    Run(string) ([]byte, error)
    RunContext(context.Context, string) ([]byte, error)
    NewSession() (*ssh.Session, error)
    NewSftp(...sftp.ClientOption) (*sftp.Client, error)
    Close() error
}
```

然后：

```go
type remoteClient struct {
    *goph.Client
    closeAll func() error
}
```

`Close()` 调用 `closeAll`。

这样 `run/login/scp/download` 只依赖接口，jump 连接可以安全关闭两段连接。

## 命令接入

### run

新增 flag：

```bash
sshc run devhost --jump bastion -- hostname
```

实现：

- `runFlagOptions` 增加 `Jump string`。
- resolve target host 后应用 `Jump` override。
- 执行路径使用 resolved target host，host 内携带 `Jump` 字段即可。

### login

新增 flag：

```bash
sshc login inner-db --jump bastion
```

### scp/upload

新增 flag：

```bash
sshc scp -l app.jar -r /tmp/app.jar inner-db --jump bastion
```

### download/dl

新增 flag：

```bash
sshc download -r /var/log/app.log -l tmp/logs inner-db --jump bastion
```

## 阶段计划

### P2.1: 抽象 SSH client 关闭模型

目标：

- 为 jump 双连接 close 做准备。
- 不改变用户行为。

范围：

```text
internal/core/ssh.go
internal/core/core_test.go
```

实现：

1. 新增 `RemoteClient` 接口。
2. 调整内部 helper 接收 `RemoteClient` 而不是强依赖 `*goph.Client`：
   - `executeRemoteScript`
   - `uploadRemoteScript`
   - `uploadPreparedJob`
   - 其他只需要 `Run/NewSftp/NewSession/Close` 的函数。
3. `newSSHClient(host)` 仍返回无 jump client，但返回类型可以改为 `RemoteClient`。
4. 保证无 jump 路径行为不变。

测试：

```text
TestNewSSHClientUsesKnownHostsByDefault
TestNewSSHClientUsesInsecureWhenConfigured
TestRemoteClientCloseCallsUnderlyingClient
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
refactor(ssh): abstract remote client connection
```

### P2.2: 解析 jump host

目标：

- 从 target host 和 `--jump` 解析出 target+jump。
- 暂不真正通过 jump 连接。

范围：

```text
internal/core/config_resolve.go
internal/core/core_test.go
internal/command/host_resolve.go
```

建议结构：

```go
type ResolveHostOptions struct {
    Jump string
}

type ResolvedConnection struct {
    Target Host
    Jump   *Host
}
```

实现：

1. 新增 `ResolveConnectionWithSSHConfig(target string, opts ResolveHostOptions)`。
2. target 使用 effective host 解析。
3. jump name 来源：
   - opts.Jump
   - target.Jump
4. jump 非空时解析 jump host effective。
5. 拒绝 target 自引用。
6. 拒绝 jump host 自己再配置 jump。
7. 保持现有 `ResolveHostWithSSHConfig` 兼容。

测试：

```text
TestResolveConnectionWithoutJump
TestResolveConnectionFromHostJump
TestResolveConnectionWithJumpOverride
TestResolveConnectionRejectsSelfJump
TestResolveConnectionRejectsNestedJump
TestResolveConnectionMissingJumpHost
```

验证：

```powershell
go test ./internal/core
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- internal/core internal/command
```

提交：

```text
feat(ssh): resolve jump host settings
```

### P2.3: 实现 jump SSH 连接

目标：

- 真正支持 local -> jump -> target。

范围：

```text
internal/core/ssh.go
internal/core/core_test.go
```

实现：

1. 新增 `newSSHClientForConnection(conn ResolvedConnection)` 或等价函数。
2. 无 jump 时走现有逻辑。
3. 有 jump 时：
   - 建立 jump client。
   - 通过 jump client Dial target TCP。
   - 在该 conn 上创建 target ssh client。
   - close target 时同时 close jump。
4. target 和 jump 分别使用自己的 auth、port、connect_timeout、host_key_check、known_hosts_path。
5. 错误提示包含阶段：
   - `connect jump host bastion: ...`
   - `connect target host inner-db via jump bastion: ...`

测试建议：

- 单元测试可通过注入 dialer/factory，避免依赖真实 SSH 服务。
- 如果不想大改结构，可先测试构造路径和错误包装。
- 可选后续补 integration test，用本地 test SSH server 模拟。

测试：

```text
TestNewSSHClientWithoutJumpUsesDirectDial
TestNewSSHClientWithJumpDialsTargetThroughJump
TestNewSSHClientWithJumpClosesBothClients
TestNewSSHClientWithJumpWrapsJumpConnectError
TestNewSSHClientWithJumpWrapsTargetConnectError
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
feat(ssh): connect through jump host
```

### P2.4: 命令接入 `--jump`

目标：

- run/login/scp/download 都支持 `--jump`。

范围：

```text
internal/command/run.go
internal/command/login.go
internal/command/scp.go
internal/command/download.go
internal/command/host_resolve.go
internal/command/commands_test.go
```

实现：

1. 给四个命令增加 `--jump`。
2. 调整 `resolveCommandHost` 为返回 resolved connection 或把 jump 写入 target host。
3. `runRemote/loginRemote/scpUpload/downloadRemote` 测试 hook 如果仍接收 `Host`，需要能断言 `host.Jump` 或新增 connection 类型。
4. 不破坏无 jump 现有测试。

测试：

```text
TestRunPassesJumpOption
TestLoginPassesJumpOption
TestSCPPassesJumpOption
TestDownloadPassesJumpOption
TestCommandUsesConfiguredJump
TestCommandJumpOptionOverridesConfiguredJump
```

验证：

```powershell
go test ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe run --help | Out-String
.\tmp\sshc.exe login --help | Out-String
.\tmp\sshc.exe scp --help | Out-String
.\tmp\sshc.exe download --help | Out-String
git diff --check -- internal/command
```

提交：

```text
feat(ssh): add jump option to ssh commands
```

### P2.5: 文档和 help 收口

目标：

- README、中文 README、TODO 和 LongHelp 更新。

范围：

```text
README.md
README.zh-CN.md
docs/TODO.md
internal/command/*.go
```

实现：

1. README 增加 jump host 配置示例。
2. 中文 README 同步。
3. TODO 标记标准 jump host 完成，PVE/LXC/vhost 继续保留为暂缓项。
4. LongHelp 只保留常用示例，不重复 option 描述。

验证：

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe run --help | Out-String
.\tmp\sshc.exe login --help | Out-String
git diff --check -- README.md README.zh-CN.md docs internal/command
```

提交：

```text
docs: document jump host usage
```

## 完整验收

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
.\tmp\sshc.exe --help | Out-String
.\tmp\sshc.exe run --help | Out-String
.\tmp\sshc.exe login --help | Out-String
.\tmp\sshc.exe scp --help | Out-String
.\tmp\sshc.exe download --help | Out-String
git diff --check -- .
git status --short --branch
```

手工 smoke 建议：

```bash
sshc run inner-db --jump bastion -- hostname
sshc login inner-db --jump bastion
sshc scp -l ./app.jar -r /tmp/app.jar inner-db --jump bastion
sshc download -r /var/log/app.log -l tmp/logs inner-db --jump bastion
```

如果没有真实 jump 环境，至少通过单元测试覆盖：

- direct path
- jump path
- jump 连接失败
- target 连接失败
- close 两段连接

## 风险与处理

| 风险 | 处理 |
| --- | --- |
| 双 SSH client close 泄漏 | P2.1 先抽象 RemoteClient，close 时统一关闭 target+jump |
| known_hosts 对内网 IP 不存在导致连接失败 | 文档说明先用 OpenSSH 或 ssh-keyscan 建立本机 known_hosts 信任 |
| 多级 jump 需求出现 | 初版明确拒绝 nested jump，后续单独设计 |
| goph.Client 构造受限 | 必要时引入本地 RemoteClient wrapper，不扩大到命令层 |
| run/login/scp/download 接入不一致 | P2.4 用同一 resolve helper 和命令测试覆盖四类命令 |
| `--jump` 参数位置和 gcli 解析差异 | 测试覆盖 `sshc run --jump bastion inner-db -- cmd` 和 `sshc scp ... inner-db --jump bastion` |

## 后续扩展

- 多级 jump：`jump: ["b1", "b2"]`
- `--no-jump` 禁用配置中的 jump
- OpenSSH config `ProxyJump` 读取
- ProxyCommand backend
- PVE/LXC/vhost proxy command
- jump connection pooling

这些不进入本轮 P2。
