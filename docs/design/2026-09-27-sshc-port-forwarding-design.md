<!-- template_id: design; template_version: 1.1.1 -->
# sshc 本地端口转发设计

> 状态：Draft 0.2 / 待人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=让本机客户端通过 sshc 访问远程 SSH 主机可达的 DB、Redis 等 TCP 服务；allowed_scope=CLI、本地端口转发核心、配置解析复用、测试和使用文档；non_goals=远程端口转发、SOCKS、Web 控制台、command_proxy 转发、守护进程管理；expansion_policy=DEFER_OR_REQUEST；review budget=一轮设计确认，若出现 CORE_BLOCKING 再进入阻塞闭环；停止条件=命令契约、连接生命周期、安全边界和验收证据明确后停止。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-09-27 | Codex | 初稿：确定本地 TCP 转发命令、连接复用方式、安全边界和实施分阶段方案 |
| 0.2 | 2026-09-27 | Codex | 根据用户确认增加命名保存配置、完整 `tunnel/tun` 命令组，并简化本地 endpoint 输入 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 背景与目标

用户希望通过一台已能 SSH 访问的远程机器，访问该机器或其网络可达范围内的 DB、Redis 等 TCP 服务，而不必为这些服务开放公网端口或单独配置每个服务主机：

```text
本地 DB 客户端 -> 127.0.0.1:15432 -> sshc -> SSH 主机 -> 127.0.0.1:5432
```

目标是提供一个 CLI-first、前台可观察、复用现有 host/auth/jump/known_hosts 解析的本地端口转发能力。用户可以使用保存的 host，也可以使用已经支持的 `--auth` 连接未登记 IP。

## 名词

| 名称 | 含义 |
|---|---|
| SSH target | sshc 建立 SSH 会话的主机，复用 host、auth、jump 和 host key 配置 |
| local endpoint | 本机监听地址和端口，供 DB、Redis 客户端连接 |
| remote endpoint | 从 SSH target 视角访问的服务地址和端口 |
| forward rule | 一个 `local endpoint -> remote endpoint` 映射 |
| tunnel profile | 命名保存的 SSH target、认证覆盖和一个或多个 forward rule |
| forward session | 一个 SSH client 和一个或多个本地监听器的前台生命周期 |
| local forwarding | OpenSSH `-L` 语义；连接由本机进入，经 SSH 会话到远端服务 |

## 范围与非目标

### 范围

- 新增顶层 `sshc tunnel` 命令组，别名为 `tun`。
- 初版支持命名保存 tunnel profile，并提供 `tunnel forward`、`tunnel list`、`tunnel show`、`tunnel add`、`tunnel rm` 等完整管理和运行入口。
- 支持一个命令中重复配置多个 TCP forward rule。
- 默认只监听 loopback 地址，支持 DB/Redis 等普通 TCP 协议。
- 复用现有 host/auth/`--auth`/jump/known_hosts/连接超时配置。
- 前台运行，输出实际监听地址、连接建立/关闭和首个错误；收到 Ctrl-C 后有序关闭。
- 支持本地端口 `0`，由操作系统分配后打印实际端口，便于测试和脚本调用。

### 非目标

- 本阶段不做 `ssh -R` 远程转发或 `ssh -D` SOCKS 动态代理。
- 不支持远程转发、SOCKS 或后台 daemon；命名 tunnel profile 本身属于本地配置的一部分。
- 不做后台 daemon、PID 文件、跨进程 `forward list/stop`；后台生命周期可作为后续设计。
- 不支持 `command_proxy` 逻辑主机，因为它没有可复用的 SSH TCP 通道语义。
- 不在 Web console 暴露转发管理 API。
- 不承诺跨 SSH 会话的连接池；每个转发会话使用一个 SSH client，每条本地连接通过该 client 新建一个 SSH direct-tcpip channel。

## 已确认事实与规范

- `internal/core/config_resolve.go` 已统一处理 host、auth profile、group defaults、jump 和有效连接参数；端口转发应调用同一解析链，避免复制凭据或跳板逻辑。
- `internal/core/ssh.go` 的 `RemoteClient` 已提供 `Dial(network, addr)`，jump host 实现也通过该能力建立 target channel；这为转发复用提供了直接 seam。
- 现有 `newSSHClient(host)` 会处理直接 SSH、jump host、认证、host key 校验和连接关闭；forward 核心只需在其上管理本地 listener 和 TCP copy。
- `sshc` 的现有命令按前台 CLI 运行，`run/login/scp/download` 已使用 host/auth 解析；新命令应保持 CLI-first 和现有错误风格。
- 适用规范：SR1204（范围确认）、SR1403（最小充分实现）、SR1407（至少一个可执行验证）、SR1211（功能点完成后精确本地原子提交；不授权 push）。
- 代码图服务本轮 Transport closed，调用图和覆盖状态未作为证据；以上事实已通过当前源码和现有测试文件直接读取确认。

## 总体方案

### 推荐命令契约

```bash
# 保存一个命名转发配置；本地只写端口时默认绑定 127.0.0.1
sshc tunnel add dev-db --target devhost \
  --forward 15432=127.0.0.1:5432

# 复用 auth 连接未登记 SSH IP 并保存
sshc tunnel add prod-redis --target 192.168.1.20 --auth dev-root \
  --forward 16379=127.0.0.1:6379

# 查看、启动、删除
sshc tunnel list
sshc tunnel show dev-db
sshc tunnel forward dev-db
sshc tun forward prod-redis
sshc tunnel rm dev-db --yes
```

命令组建议：

| 命令 | 语义 |
|---|---|
| `tunnel add <name>` | 创建或更新命名 tunnel profile；只保存配置，不启动转发 |
| `tunnel forward <name>` | 按保存配置启动前台转发；别名建议 `start` |
| `tunnel list` | 列出名称、target、forward 数量、默认本地端口和备注；支持 `--json` |
| `tunnel show <name>` | 展示单个配置；凭据字段只显示 profile 名称，不显示秘密 |
| `tunnel rm <name>` | 删除保存配置；运行中的前台进程不受配置删除影响 |
| `tunnel edit <name>` | 可选的交互编辑入口；若 CLI 编辑器约束不稳定，首版可延后 |

`tunnel forward` 还允许一次性覆盖，不保存修改：

```bash
sshc tunnel forward dev-db --forward 15433=127.0.0.1:5432
sshc tunnel forward --target 192.168.1.20 --auth dev-root \
  --forward 16379=127.0.0.1:6379
```

后一个形式不带 name，只运行临时规则；它与已有 `sshc run --auth` 一样不会保存 IP。

建议选项：

| 选项 | 语义 |
|---|---|
| `name` | 保存模式必填；临时运行模式可省略 |
| `--target` | 临时运行模式必填；保存模式写入 profile |
| `--forward`, `-L` | 保存/覆盖规则，可重复；格式 `local[host:]port=remote[host:]port` |
| `--auth` | 可选；沿用现有 profile 解析，支持未登记 SSH IP |
| `--jump` | 可选；覆盖本次 SSH target 的 jump host |
| `--connect-timeout` | 可选；连接 SSH 或 remote channel 的超时 |
| `--ready-timeout` | 可选；等待本地 listener 建立，默认只用于启动错误返回 |
| `--allow-non-loopback` | 初版不实现；非 loopback 监听在后续安全设计中单独评审 |
| `--background` | 初版不实现；避免没有可观测的生命周期和 stop 语义 |

`--forward` 的解析规则：本地侧只写端口时默认规范化为 `127.0.0.1:port`，所以用户不必重复写前面的 `127.0.0.1`；远端侧只写端口时默认规范化为 `127.0.0.1:port`；完整地址仍可写成 `192.168.1.10:15432=10.0.0.8:5432`。端口范围为 1..65535，允许本地端口为 `0`；不接受 Unix socket、端口范围和 UDP。IPv6 使用带方括号的标准形式，例如 `[::1]:15432=[::1]:5432`。

### 配置策略

初版新增独立的 `tunnels` 配置集合，不把转发规则塞入 `Host`。认证和连接参数仍从已有 host/auth/defaults/group/jump 配置解析。

建议 JSON 结构：

```json
{
  "tunnels": [
    {
      "name": "dev-db",
      "target": "devhost",
      "auth_ref": "dev-root",
      "forwards": [
        {"local": "15432", "remote": "127.0.0.1:5432"},
        {"local": "16379", "remote": "127.0.0.1:6379"}
      ],
      "remark": "开发环境数据库和 Redis"
    }
  ]
}
```

字段约束：

- `name` 在 `tunnels` 内唯一，使用与 host/auth 名称相同的安全字符约束。
- `target` 保存 SSH host 名称、IP 或可解析目标；不保存解析后的密码和私钥。
- `auth_ref` 是可选的本次 tunnel profile 凭据引用；命令行 `--auth` 可在启动时覆盖。
- `forwards` 至少一项；保存时统一存储规范化后的 `local` 和 `remote` 地址。
- `remark` 可选，不参与连接逻辑。

这样可以：

- 不把数据库密码、私钥或解析后的认证秘密写进 sshc 配置；服务地址和本地端口属于用户明确保存的 tunnel profile 数据。
- 保持 `--auth` 的“本次命令选择凭据，不保存 IP”语义。
- 让 `tunnel list/show/rm` 能围绕命名配置形成完整命令组。

配置保存使用现有原子写入和校验链；删除 tunnel 不删除关联 host/auth。

## 架构

```text
command.NewTunnelCmd
  -> tunnel add/list/show/rm/forward
  -> resolveCommandHostWithAuth / existing host+jump resolution
  -> core.StartLocalForward(host, rules, options)
       -> newSSHClient(host)
       -> net.Listen(local endpoint) for each rule
       -> accept loop
       -> remoteClient.Dial("tcp", remote endpoint)
       -> bidirectional copy with per-connection cleanup
       -> context/signal cancellation closes listeners and SSH client
```

建议新增的核心边界：

```go
type ForwardRule struct {
    LocalAddr  string
    RemoteAddr string
}

type ForwardOptions struct {
    ConnectTimeout time.Duration
}

type ForwardSession interface {
    Endpoints() []string
    Wait() error
    Close() error
}

func StartLocalForward(host Host, rules []ForwardRule, opts ForwardOptions) (ForwardSession, error)
```

实现约束：

1. 先解析并校验所有规则，再建立 SSH client，避免只启动部分 listener 后才发现另一条规则非法。
2. listener 建立后返回实际端口；任一 listener 建立失败时关闭已建立 listener 和 SSH client。
3. accept loop 中每条本地连接调用 `RemoteClient.Dial("tcp", rule.RemoteAddr)`，然后启动两个方向的 copy；任一方向结束都关闭该连接的另一端。
4. session `Close` 必须可重复调用；关闭顺序为 listeners、活动本地连接、SSH client。
5. `Wait` 只在前台命令中阻塞；命令层负责 `os.Interrupt`/`SIGTERM` 转为 session close，不把信号处理放进 core。
6. `command_proxy` 在命令解析后立即返回明确错误；它只有远端命令代理，没有可直接承载 `direct-tcpip` 的 SSH client。

## 关键流程

### 启动

```text
1. 读取 name 或临时 `--target`、`--auth`、`--jump` 和重复 `--forward`。
2. 如果给出 name，读取 tunnel profile；命令行的 target/auth/jump/forward 覆盖对应保存值。
3. 校验至少一条规则，解析 local/remote endpoint，拒绝非 TCP、非法端口和非 loopback local host。
4. 按现有解析链得到有效 SSH host；显式 `--auth` 覆盖 profile 中的 `auth_ref`。
5. 建立 SSH client；jump host 由现有实现处理。
6. 为所有规则创建 listener；输出 `tunnel ready local -> remote`。
7. 等待连接或终止信号。
```

### 单条连接

```text
local client connects to local listener
  -> accept
  -> SSH client.Dial("tcp", remote endpoint)
  -> copy local -> remote and remote -> local
  -> close both ends
  -> log duration and bytes at debug/verbose level only
```

### 退出与错误

- SSH 认证、host key、jump 或 remote channel 建立失败：返回非零，并关闭所有资源。
- 单条本地连接失败：记录目标和错误，继续接受后续连接；不因一个 DB 客户端断开终止整个 session。
- listener 被占用：返回包含 local endpoint 的错误，不自动换端口；只有用户显式使用 `:0` 才由系统分配。
- Ctrl-C：停止接收新连接，关闭活动连接和 SSH client，等待 goroutine 回收后返回 0。
- SSH 主连接意外断开：关闭 listeners，返回非零；不自动重连，避免隐藏网络或认证问题。

## 安全、数据、运维与回滚

### 安全

- 默认绑定 `127.0.0.1`，不把数据库端口暴露给局域网。
- 初版拒绝 `0.0.0.0`、非 loopback IPv4/IPv6 和 wildcard bind；允许公网/局域网监听必须另开设计和人工确认。
- 不把密码、私钥、完整 remote endpoint 或服务流量写入日志；日志只记录 target、local endpoint、remote endpoint 的必要元数据和状态，敏感值按现有脱敏规则处理。
- 继续使用已有 known_hosts；不因为端口转发而默认降低 host key 检查。
- 不支持通过命令字符串拼接转发；只接受结构化 TCP 地址，避免 shell 注入。

### 数据与运维

- 配置新增 `tunnels` 集合，需要沿用现有配置版本兼容和原子保存机制；旧配置缺少该字段时按空集合处理。
- 前台进程是生命周期真源；终止进程即停止转发。
- `--forward ...:0=...` 的实际端口必须打印到 stdout，便于测试脚本读取；固定端口启动信息打印到 stderr 或现有命令输出流，格式需要在计划阶段固定。
- 首版不写 run log，因为转发不是一次远程命令；若需要审计，后续增加专用 forward session log schema。

### 回滚

- 代码回滚只涉及新增 tunnel command/core/session/test/docs 文件；删除 `tunnels` 字段不会影响现有 Host/Auth 数据。
- 运行时回滚为终止 `sshc tunnel forward` 进程；不会残留远端进程或 listener。

## 决策

1. **采用 `tunnel/tun` 命令组。** 转发配置具有命名、查看、列出、删除和启动生命周期，不能继续作为无状态顶层命令。
2. **命名配置独立于 Host。** tunnel 是“SSH target + 服务映射”，不应污染 SSH 主机身份模型。
3. **一个 SSH session 支持多个 forward rule。** DB、Redis 等服务可以共用一次 SSH 认证和 jump 连接，减少连接和配置重复。
4. **本地 endpoint 默认只写端口。** `15432` 等价于 `127.0.0.1:15432`；完整地址仅在用户需要指定地址时使用，但 v1 仍拒绝非 loopback 监听。
5. **规则使用 `local=remote` 结构化格式。** 等号区分两侧，端口简写降低输入成本，带方括号的 IPv6 保持可解析。
6. **默认 loopback 且前台运行。** 先保证暴露面和生命周期可观察，再评估后台管理。
7. **复用 `RemoteClient.Dial`，不复制 SSH 建连。** 现有 jump、认证、host key 和 close 语义集中在 `newSSHClient`，新实现只负责 listener、channel 和 copy。

## 待确认事项

1. `tunnel edit` 是否在初版实现；本设计建议命令组保留入口，若编辑器交互不稳定可先用 `tunnel rm` + `tunnel add` 替代。
2. `tunnel add <name>` 对已存在 name 是覆盖还是要求 `--force`；建议默认拒绝覆盖，显式 `--force` 才更新，避免误改共享配置。
3. 是否需要 `tunnel forward <name> --json` 输出实际 listener 端口；本设计建议初版支持，以便端口为 `0` 时脚本读取。
4. `remote endpoint` 是否只允许 SSH target 视角的 IP/hostname，还是需要额外支持 Unix socket（本设计建议暂不支持）。
5. Ctrl-C 之外是否需要 Windows 控制事件和 stdin EOF 退出；本设计建议先实现跨平台 interrupt，stdin EOF 不作为默认退出条件。
6. 后续是否需要转发到 `command_proxy` 目标；这需要为 proxy backend 设计 TCP stream 语义，不能直接复用当前命令模板。

## 结论与人工计划 Gate

本设计建议以 `sshc tunnel/tun` 作为 v1 命令组，支持 `tunnel add/list/show/rm/forward`，将命名 tunnel profile 保存到独立的 `tunnels` 配置集合。`tunnel forward` 复用现有 SSH/auth/jump 连接，通过 `RemoteClient.Dial` 建立 direct-tcpip channel；本地只写端口时默认绑定 `127.0.0.1`，默认前台运行并拒绝非 loopback 监听。它能覆盖“本地连接远程 DB/Redis”的核心结果，同时把远程转发、SOCKS、后台管理和 command_proxy 留在后续边界。

当前仍是 `Draft 0.2`。需要用户确认命令组、持久化字段和覆盖规则后，才能进入实施计划；设计批准本身不授权代码实施、提交、发布或部署。
