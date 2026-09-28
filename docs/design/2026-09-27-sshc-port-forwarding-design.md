<!-- template_id: design; template_version: 1.1.1 -->
# sshc 本地端口转发设计

> 状态：Draft 0.3 / 待人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=让本机客户端通过 sshc 访问远程 SSH 主机可达的 DB、Redis 等 TCP 服务；allowed_scope=CLI、本地端口转发核心、配置解析复用、tunnels 配置集合、cfg export/import 与 doctor 协同、测试和使用文档；non_goals=远程端口转发、SOCKS、Web 控制台、command_proxy 转发、守护进程管理、转发审计日志；expansion_policy=DEFER_OR_REQUEST；review budget=0.2 已消费一轮设计评审，本修订后只做一轮 changed-scope 复评；停止条件=命令契约、连接生命周期（含空闲存活与断开检测）、安全边界和验收证据明确后停止。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-09-27 | Codex | 初稿：确定本地 TCP 转发命令、连接复用方式、安全边界和实施分阶段方案 |
| 0.2 | 2026-09-27 | Codex | 根据用户确认增加命名保存配置、完整 `tunnel/tun` 命令组，并简化本地 endpoint 输入 |
| 0.3 | 2026-09-28 | Jcode | 按 `docs/review/2026-09-28-sshc-port-forwarding-design-review.md` 修订：修正转发 seam 事实、补空闲存活与断开检测、确定 tunnel 校验等级与 doctor 协同、把 tunnels 纳入 cfg export/import、补引用完整性规则、拆分 `target`/`address` 并支持持久化 `port`/`jump`、新增验收章节，并收敛待确认事项 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 背景与目标

用户希望通过一台已能 SSH 访问的远程机器，访问该机器或其网络可达范围内的 DB、Redis 等 TCP 服务，而不必为这些服务开放公网端口或单独配置每个服务主机：

```text
本地 DB 客户端 -> 127.0.0.1:15432 -> sshc -> SSH 主机 -> 127.0.0.1:5432
```

目标是提供一个 CLI-first、前台可观察、复用现有 host/auth/jump/known_hosts 解析的本地端口转发能力。目标可以是已保存的 host，也可以是用 `--auth` 直接连接的未登记地址。

本设计取代同仓库 `docs/2026-07-08-sshc-usability-enhancements-design.md` 的 P2 tunnel/port-forward 提案：字段由 `host` 拆分为 `target`/`address`，取消 `tunnel stop` 与后台/pid 语义，`-L` 不作为 OpenSSH 兼容承诺（见决策 5）。

## 名词

| 名称 | 含义 |
|---|---|
| SSH target | sshc 建立 SSH 会话的主机，复用 host、auth、jump 和 host key 配置 |
| host target | 保存在 `hosts` 中、按名称或 IP 解析的 SSH 目标 |
| address target | 未登记在 `hosts` 中的 SSH 地址（IP 或主机名），必须配合认证 profile 使用 |
| local endpoint | 本机监听地址和端口，供 DB、Redis 客户端连接 |
| remote endpoint | 从 SSH target 视角访问的服务地址和端口 |
| forward rule | 一个 `local endpoint -> remote endpoint` 映射 |
| tunnel profile | 命名保存的 SSH target（host 或 address）、认证覆盖、可选 port/jump 和一个或多个 forward rule |
| forward session | 一个 SSH client 和一个或多个本地监听器的前台生命周期 |
| forward dialer | 转发核心使用的拨号缝：对 SSH 会话发起 `direct-tcpip`，并负责会话关闭 |
| 存活监视 | 判断空闲 SSH 会话是否仍可用（keepalive 与连接关闭观察）的机制 |
| local forwarding | OpenSSH `-L` 语义；连接由本机进入，经 SSH 会话到远端服务 |

## 范围与非目标

### 范围

- 新增顶层 `sshc tunnel` 命令组，别名为 `tun`。
- 支持命名保存 tunnel profile，并提供 `tunnel forward`、`tunnel list`、`tunnel show`、`tunnel add`、`tunnel rm` 等管理和运行入口。
- 目标既可以是已保存 host（`--target`），也可以是未登记地址（`--address` + `--auth`）；profile 可保存可选 `port` 和 `jump`。
- 支持一个命令中重复配置多个 TCP forward rule。
- 默认只监听 loopback 地址，支持 DB/Redis 等普通 TCP 协议。
- 复用现有 host/auth/`--auth`/jump/known_hosts/连接超时配置。
- 前台运行，输出实际监听地址、连接建立/关闭和首个错误；收到 Ctrl-C 后有序关闭。
- 空闲会话存活监视：keepalive 与 SSH 连接关闭检测（见"存活与失效"）。
- 支持本地端口 `0`，由操作系统分配后打印实际端口，便于测试和脚本调用。
- `tunnels` 参与 `cfg export/import` 合并；`cfg doctor` 报告 tunnel 问题；`auth rm`、`host rm/rename` 与本集合保持引用一致性。

### 非目标

- 本阶段不做 `ssh -R` 远程转发或 `ssh -D` SOCKS 动态代理。
- 不做后台 daemon、PID 文件、跨进程 `forward list/stop`；后台生命周期可作为后续设计。
- 不支持 `command_proxy` 逻辑主机，因为它没有可复用的 SSH TCP 通道语义。
- 不在 Web console 暴露转发管理 API。
- 不承诺跨 SSH 会话的连接池；每个转发会话使用一个 SSH client，每条本地连接通过该 client 新建一个 SSH direct-tcpip channel。
- 首版不写转发 session 审计日志（不新增 run log schema），只在运行时日志中保留命令行参数。

## 已确认事实与规范

- `internal/core/config_resolve.go:66-113` 已统一处理 host、auth profile、group defaults、jump 和有效连接参数；`ResolveEffectiveHostWithAuth` 对未登记目标走 `validateRawBatchTarget`，因此"未登记地址 + `--auth`"是既有解析链已支持的形式，转发应调用同一解析链。
- `internal/core/ssh.go:63-95`：`RemoteClient` 接口只有 `Run`、`RunContext`、`NewSession`、`NewSftp`、`Close`；`Dial(network, addr)` 属于未导出的具体实现 `*remoteClient`，`newSSHClient`/`newSSHClientWithOptions`（`internal/core/ssh.go:732-749`）返回接口类型。转发核心需要自己的拨号缝，不能直接对接口调用 `Dial`。
- `internal/core/ssh.go:87-95` 的 `Dial` 在 jump 场景下指向 target channel（`internal/core/ssh.go:761-799` 的 jump 实现同样通过它建 target channel），因此拨号缝与 jump host 语义天然一致。
- `internal/core/ssh.go:240-267` 已提供 `startSessionKeepalive`（发送 `keepalive@openssh.com`，超时后关闭 client），但目前只接在 `loginWithClient`（`internal/core/ssh.go:186-190`）；超时常量 `defaultKeepaliveEvery=30s`、`defaultKeepaliveWait=10s`。转发会话需要复用同一形态。
- `internal/core/ssh.go:88-100` 提供 `remoteClientDialForTest` 测试缝，`internal/core/ssh_test.go:162-194` 已有替换 `Dial` 行为的先例，可直接用于转发单元测试。
- `internal/core/store.go:561-594` 的 `SaveConfig` 使用临时文件加 rename 的原子写入；`normalizeConfig`（`internal/core/store.go:604-621`）对缺失集合按空值处理，旧配置缺 `tunnels` 时天然兼容。
- `internal/core/config_doctor.go:20-33` 的 `CheckConfig` 被当作保存门禁使用：`internal/command/add.go:156`、`internal/command/host.go:891`、`internal/command/group.go:124,166`、`internal/server/request.go:33-39`（serve 写接口）、`internal/core/host_import.go:595`。新增检查的等级会影响这些无关操作。
- `internal/core/config_export.go:157-276` 的导入合并只覆盖 `logs_path`、`defaults`、`groups`、`auth_profiles`、`hosts`，`ImportResult` 只统计 hosts/groups/auth；`ImportReplace` 直接采用导入包（`internal/core/config_export.go:165-172`）。
- `internal/command/auth.go:191` 的 `auth rm` 只拒绝被 `hosts` 引用的 profile；`internal/command/host.go:619-645` 的 `host rm` 与 rename 没有引用检查。
- `sshc` 的现有命令按前台 CLI 运行，`run/login/scp/download/check` 已使用 host/auth 解析；新命令应保持 CLI-first 和现有错误风格。
- 适用规范：SR1204（范围确认）、SR1403（最小充分实现）、SR1407（至少一个可执行验证）、SR1211（功能点完成后精确本地原子提交；不授权 push）。
- 代码图服务本轮不可用（MCP 连接即失败），调用图和覆盖状态未作为证据；以上事实均通过当前源码和现有测试文件直接读取确认，并附行号便于复核。

## 总体方案

### 推荐命令契约

```bash
# host 目标：本地只写端口时默认绑定 127.0.0.1
sshc tunnel add dev-db --target devhost \
  --forward 15432=127.0.0.1:5432

# 同一命令保存多条规则，共享一次 SSH 认证
sshc tunnel add dev-stack --target devhost \
  --forward 15432=127.0.0.1:5432 \
  --forward 16379=127.0.0.1:6379

# address 目标：未登记地址必须给出认证 profile；可指定非默认端口与跳板
sshc tunnel add prod-redis --address 192.168.1.20 --auth dev-root --port 2222 \
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
| `tunnel add <name>` | 创建命名 tunnel profile；只保存配置，不启动转发；名称已存在时默认拒绝，`--force` 覆盖（决策 9） |
| `tunnel forward <name>` | 按保存配置启动前台转发；别名建议 `start` |
| `tunnel list` | 列出名称、目标模式、target/address、port、jump、forward 数量、默认本地端口和备注；支持 `--json` |
| `tunnel show <name>` | 展示单个配置；凭据字段只显示 profile 名称，不显示秘密；支持 `--json` |
| `tunnel rm <name>` | 删除保存配置；运行中的前台进程不受配置删除影响 |
| `tunnel edit <name>` | 初版不实现（决策 8）；用 `rm` + `add` 替代 |

`tunnel forward` 允许一次性覆盖，不保存修改：

```bash
# 提供任一 --forward 时整体替换保存的规则（不是追加）
sshc tunnel forward dev-db --forward 15433=127.0.0.1:5432

# 无 name 的临时运行模式：--target 或 --address 必填其一
sshc tunnel forward --target devhost --forward 15433=127.0.0.1:5432
sshc tunnel forward --address 192.168.1.20 --auth dev-root \
  --forward 16379=127.0.0.1:6379
```

临时运行模式与已有 `sshc run --auth` 一样不会保存地址或规则。

建议选项：

| 选项 | 语义 |
|---|---|
| `name` | 保存模式必填；临时运行模式可省略 |
| `--target` | host 目标：已登记 host 的名称或 IP；保存模式与 `--address` 互斥且必填其一 |
| `--address` | address 目标：未登记 IP 或主机名；必须与 `--auth` 同时使用 |
| `--forward` | 保存/覆盖规则，可重复；格式 `local[host:]port=remote[host:]port`；不接受 OpenSSH `-L` 冒号语法，不提供 `-L` 短名（决策 5） |
| `--auth` | 可选（host 目标）；必填（address 目标）。沿用现有 profile 解析 |
| `--port` | 可选；SSH 端口，服务 address 目标与非默认端口场景，host 目标下覆盖该 host 的端口 |
| `--jump` | 可选；跳板 host 名称，覆盖本次解析结果 |
| `--connect-timeout` | 可选；SSH 连接超时，覆盖 host/defaults 配置 |
| `--json` | 可选；`tunnel forward` 就绪后向 stdout 输出一行 JSON 就绪对象（含实际端口）；`tunnel list/show` 输出 JSON |
| `--verbose` | 可选；输出单条连接的建立/关闭与字节统计（默认静默，仅错误可见） |
| `--quiet` | 可选；抑制非错误诊断输出（就绪行仍按 `--json` 规则处理） |
| `--force` | 可选；`tunnel add` 允许覆盖同名 profile（默认拒绝） |
| `--yes` | 可选；`tunnel rm` 跳过交互确认（与其他 rm 命令一致） |
| `--allow-non-loopback` | 初版不实现；非 loopback 监听在后续安全设计中单独评审 |
| `--background` | 初版不实现；避免没有可观测的生命周期和 stop 语义 |

`--forward` 的解析规则：本地侧只写端口时默认规范化为 `127.0.0.1:port`；远端侧只写端口时默认规范化为 `127.0.0.1:port`；完整地址可写成 `192.168.1.10:15432=10.0.0.8:5432`。端口范围为 1..65535，允许本地端口为 `0`；不接受 Unix socket、端口范围和 UDP。IPv6 使用带方括号的标准形式，例如 `[::1]:15432=[::1]:5432`。

输出约定（本版固定，计划阶段不再重开）：默认就绪信息（含 `local -> remote` 与 `:0` 分配到的实际端口）写到 stderr 诊断流；`--json` 时同一信息以单行 JSON 写到 stdout，stdout 不接受其他内容；单条连接失败、连接关闭和字节统计只在 `--verbose`/debug 级别输出。

### 配置策略

新增独立的 `tunnels` 配置集合，不把转发规则塞入 `Host`。认证和连接参数仍从已有 host/auth/defaults/group/jump 配置解析。

建议 JSON 结构：

```json
{
  "tunnels": [
    {
      "name": "dev-db",
      "target": "devhost",
      "port": 22,
      "jump": "bastion",
      "auth_ref": "dev-root",
      "forwards": [
        {"local": "127.0.0.1:15432", "remote": "127.0.0.1:5432"}
      ],
      "remark": "开发环境数据库"
    },
    {
      "name": "prod-redis",
      "address": "192.168.1.20",
      "port": 2222,
      "auth_ref": "dev-root",
      "forwards": [
        {"local": "127.0.0.1:16379", "remote": "127.0.0.1:6379"}
      ]
    }
  ]
}
```

字段约束：

- `name` 在 `tunnels` 内唯一，使用与 host/auth 名称相同的安全字符约束。
- `target` 与 `address` 互斥，必须且只能提供一个：`target` 为已登记 host 的名称或 IP；`address` 为未登记 IP 或主机名。
- `address` 模式必须提供 `auth_ref`；`tunnel add --address` 未给 `--auth` 时直接报错。
- `port` 可选，1..65535，缺省沿用 host/defaults 的解析结果（address 模式缺省为 22）。
- `jump` 可选，值为已登记的 host 名称；与 `sshc run --jump` 语义一致，仅一级跳板。
- `auth_ref` 可选（host 目标）；保存时必须是已存在的 auth profile，命令行 `--auth` 可在启动时覆盖。
- `forwards` 至少一项；保存时统一存储规范化后的 `local` 和 `remote` 地址。
- `remark` 可选，不参与连接逻辑。

保存时校验（`tunnel add` 内的硬校验，不进入全局写入门禁）：

- `target` 模式必须解析到已存在 host，否则报错并提示改用 `--address`；这保证 host 被删除或改名后 forward 阶段明确报 `host not found`，而不会静默把名称当未登记地址使用。
- `address` 模式必须已有 `auth_ref`；`--port`/`--jump` 写入 profile。
- 全部 forward rule 必须能解析；本地端口重复、保留地址绑定（非 loopback）和非法端口在保存阶段即拒绝。

配置保存使用现有原子写入；`normalizeConfig` 对缺失 `tunnels` 按空集合处理，旧配置无需迁移。删除 tunnel 不删除关联 host/auth。

`tunnels` 参与 `cfg export/import`（决策 10）：合并策略与 host 一致（默认冲突即失败，overwrite 覆盖，replace 直接采用导入包），`ImportResult` 增加 `TunnelsAdded`/`TunnelsUpdated`；旧导出包无该集合时按空处理。

## 架构

```text
command.NewTunnelCmd
  -> tunnel add/list/show/rm/forward
  -> tunnel target resolution (host 模式或 address 模式)
       -> resolveCommandHostWithAuth / ResolveEffectiveHostWithAuth + HostOverrides{Port}
       -> --jump 覆盖为 profile 或本次运行值
  -> core.StartLocalForward(host, rules, options)
       -> newForwardDialer(host)       // 复用 newSSHClient 得到的 *remoteClient.Dial
       -> net.Listen(local endpoint) for each rule
       -> accept loop (临时错误退避)
       -> forwardDialer.Dial("tcp", remote endpoint)
       -> bidirectional copy with half-close and per-connection cleanup
       -> 存活监视: keepalive + SSH 连接关闭观察
       -> context/signal cancellation closes listeners, active conns, SSH client
```

建议新增的核心边界：

```go
type ForwardRule struct {
    LocalAddr  string
    RemoteAddr string
}

type ForwardOptions struct {
    ConnectTimeout time.Duration
    KeepaliveEvery time.Duration // 缺省 30s，沿用 login 语义
    KeepaliveWait  time.Duration // 缺省 10s
}

type ForwardSession interface {
    Endpoints() []string
    Wait() error
    Close() error
}

func StartLocalForward(host Host, rules []ForwardRule, opts ForwardOptions) (ForwardSession, error)

// 包内拨号缝：不扩展 RemoteClient 接口，避免波及既有测试替身。
type forwardDialer interface {
    Dial(network, addr string) (net.Conn, error)
    Close() error
}

func newForwardDialer(host Host) (forwardDialer, error)
```

实现约束：

1. 先解析并校验所有规则，再建立 SSH client，避免只启动部分 listener 后才发现另一条规则非法。
2. `newForwardDialer` 复用 `newSSHClient` 的建连与关闭语义（直接连接、jump、认证、host key）；它只暴露 `Dial`/`Close`，使 `RemoteClient` 接口保持不变。
3. listener 建立后返回实际端口；任一 listener 建立失败时关闭已建立 listener 和 SSH client。
4. accept loop 中每条本地连接调用 `forwardDialer.Dial("tcp", rule.RemoteAddr)`；临时 accept 错误按退避重试（首个 5ms，指数增长到上限 1s，持续失败时按 debug 记录），非临时错误终止 session。
5. 双向 copy 使用半关闭语义：任一方向读到 EOF 时对该方向的对端执行 `CloseWrite`（可用时），两个方向都结束或出错后关闭整条连接并清理 goroutine。
6. session `Close` 必须可重复调用；关闭顺序为 listeners、活动本地连接、SSH client。
7. `Wait` 只在前台命令中阻塞；命令层负责 `os.Interrupt`/`SIGTERM` 转为 session close，不把信号处理放进 core。
8. `command_proxy` 在 target 解析后立即返回明确错误；它只有远端命令代理，没有可直接承载 `direct-tcpip` 的 SSH client。该检查同时前移到 `tunnel add`/`tunnel forward` 与 doctor。

## 关键流程

### 启动

```text
1. 读取 name 或临时 `--target`/`--address`、`--auth`、`--jump`、`--port` 和重复 `--forward`。
2. 如果给出 name，读取 tunnel profile；命令行的 target/address/auth/jump/port/forward 覆盖对应保存值。
3. 校验至少一条规则，解析 local/remote endpoint，拒绝非 TCP、非法端口和非 loopback local host。
4. 按现有解析链得到有效 SSH host：host 模式解析已登记 host，address 模式以 `--auth` 解析未登记地址；profile 的 port/jump 作为覆盖项。
5. 建立 SSH client，并启动存活监视（keepalive + SSH 连接关闭观察）；jump host 由现有实现处理。
6. 为所有规则创建 listener；输出 `tunnel ready local -> remote`（含 `:0` 的实际端口）。
7. 等待连接、存活监视事件或终止信号。
```

### 单条连接

```text
local client connects to local listener
  -> accept
  -> forwardDialer.Dial("tcp", remote endpoint)
  -> copy local -> remote and remote -> local with half-close
  -> close both ends
  -> log duration and bytes at debug/verbose level only
```

### 存活与失效

- 存活监视由两部分组成：keepalive 定时发送 `keepalive@openssh.com`（间隔与等待沿用 `defaultKeepaliveEvery=30s`、`defaultKeepaliveWait=10s`，可通过 `ForwardOptions` 覆盖，首版不新增配置文件项）；SSH 连接关闭观察。
- 连接关闭观察：后台 goroutine 等待 SSH 连接结束（`*remoteClient` 上提升的 `Wait()`：`goph.Client` 内嵌 `*ssh.Client`，`ssh.Client` 内嵌 `ssh.Conn`，`Conn.Wait()` 在连接关闭时返回），返回即视为会话失效。
- keepalive 超时或连接关闭观察返回时：关闭 listeners、关闭活动本地连接、`Wait()` 返回非零并输出会话失效原因。
- 首版不自动重连，避免隐藏网络或认证问题；失效后由用户重新执行 `tunnel forward`。
- 该机制是本设计"连接生命周期明确"停止条件的验收对象（见验收章节 A3/A4）。

### 退出与错误

- SSH 认证、host key、jump 或首个 remote channel 建立失败：返回非零，并关闭所有资源。
- 单条本地连接失败：记录目标和错误，继续接受后续连接；不因一个 DB 客户端断开终止整个 session。
- listener 被占用：返回包含 local endpoint 的错误，不自动换端口；只有用户显式使用 `:0` 才由系统分配。
- Ctrl-C：停止接收新连接，关闭活动连接和 SSH client，等待 goroutine 回收后返回 0。
- 空闲期间 SSH 主连接失效：由存活监视触发关闭 listeners，返回非零并说明原因。

## 验收证据与验证计划

首版验收以"可执行检查"为准，全部可在本机完成，不依赖外部网络：

| 编号 | 验证点 | 方式 |
|---|---|---|
| A1 | `--forward` 解析与规范化 | 表驱动单元测试：`15432=127.0.0.1:5432`、`15432=5432`、`[::1]:15432=[::1]:5432`、`192.168.1.10:15432=10.0.0.8:5432`；非法端口、`0.0.0.0` 本地绑定、重复本地端口报错 |
| A2 | 单条连接转发 | 使用 `remoteClientDialForTest` 等价的拨号缝（`internal/core/ssh.go:88-100` 同形态）接入本地 echo 服务，验证双向数据与半关闭 |
| A3 | 空闲存活与失效 | 假拨号缝下停止响应 keepalive / 直接关闭假连接，断言 listeners 关闭、`Wait()` 返回非零、无 goroutine 泄漏 |
| A4 | `local:0` 实际端口与就绪输出 | 断言 ready 行包含系统分配的端口；`--json` 时 stdout 恰有一行 JSON 就绪对象 |
| A5 | 多规则回滚 | 占用其中一个本地端口，断言启动失败时其他 listener 与 SSH client 均已释放 |
| A6 | Ctrl-C 有序关闭 | 发送 interrupt，断言活动连接被关闭、listener 释放、进程返回 0 |
| A7 | 配置与引用 | `tunnel add` 非法输入被拒；`--address` 缺 `--auth` 被拒；`--target` 指向不存在 host 被拒；`auth rm` 拒绝被 tunnel 引用的 profile；`cfg doctor` 对过期 tunnel 只产出 warn |
| A8 | 迁移链路 | `cfg export`/`cfg import --merge|--overwrite|--replace` 往返后 tunnels 与 `TunnelsAdded`/`TunnelsUpdated` 统计正确 |
| A9 | 真实主机（可选，人工） | 对已配置 host 执行 `tunnel forward`，用本地 `psql`/`redis-cli` 或 `nc` 完成一次读写，再验证 Ctrl-C 后端口释放 |

A1-A8 是计划必须给出的自动化验收；A9 记为人工验证步骤，不阻塞计划批准，但需在计划中保留为发布前检查。

## 安全、数据、运维与回滚

### 安全

- 默认绑定 `127.0.0.1`，不把数据库端口暴露给局域网。
- 初版拒绝 `0.0.0.0`、非 loopback IPv4/IPv6 和 wildcard bind；允许公网/局域网监听必须另开设计和人工确认。
- 不把密码、私钥、passphrase 写入隧道输出；凭据通过既有 profile 解析链获取，不复制、不变换。
- 日志边界（统一规则）：前台诊断输出可以记录 target（host 名称或地址）、port、local endpoint 和 remote endpoint，因为它们是用户自己保存的服务连接元数据；凭据与流量内容永不记录；不写转发 session 审计日志；`--quiet` 抑制非错误诊断输出。
- 继续使用已有 known_hosts；不因为端口转发而默认降低 host key 检查。
- 不支持通过命令字符串拼接转发；只接受结构化 TCP 地址，避免 shell 注入。

### 数据与运维

- 配置新增 `tunnels` 集合，沿用现有原子保存；旧配置缺少该字段时按空集合处理。
- `tunnels` 纳入 `cfg export/import` 合并与统计，避免迁移时静默丢失（决策 10）。
- tunnel 检查进入 `cfg doctor` 时只使用 `DoctorWarn`，且写入路径的最小校验在 `tunnel add` 内完成；`CheckConfig` 不因 tunnel 问题阻塞 host/group/web/import 等无关写入（决策 11）。
- 引用完整性：`auth rm` 拒绝删除被 tunnel profile 引用的 profile（与现有 host 引用策略一致）；`host rm` 对被 tunnel `target` 引用的 host 给出提示；host 改名后 host 模式 tunnel 在 forward 阶段明确报 `host not found`，不静默漂移。
- 前台进程是生命周期真源；终止进程即停止转发。
- `--forward ...:0=...` 的实际端口按输出约定打印；首版不写 run log，若需要审计后续增加专用 forward session log schema。

### 回滚

- 代码回滚只涉及新增 tunnel command/core/session/test/docs 文件，以及 `tunnels` 在 config/export/doctor 中的附加分支；删除 `tunnels` 字段不会影响现有 Host/Auth 数据。
- 运行时回滚为终止 `sshc tunnel forward` 进程；不会残留远端进程或 listener。

## 决策

1. **采用 `tunnel/tun` 命令组。** 转发配置具有命名、查看、列出、删除和启动生命周期，不能继续作为无状态顶层命令。
2. **命名配置独立于 Host。** tunnel 是"SSH target + 服务映射"，不应污染 SSH 主机身份模型。
3. **一个 SSH session 支持多个 forward rule。** DB、Redis 等服务可以共用一次 SSH 认证和 jump 连接，减少连接和配置重复。
4. **本地 endpoint 默认只写端口。** `15432` 等价于 `127.0.0.1:15432`；完整地址仅在用户需要指定地址时使用，但 v1 仍拒绝非 loopback 监听。
5. **规则使用 `local=remote` 结构化格式，不提供 `-L` 短名。** 等号区分两侧，端口简写降低输入成本，带方括号的 IPv6 保持可解析；`-L` 会暗示 OpenSSH `port:host:hostport` 语法，本版不承诺该兼容。
6. **默认 loopback 且前台运行。** 先保证暴露面和生命周期可观察，再评估后台管理。
7. **转发复用 `*remoteClient.Dial`，通过包内 `forwardDialer` 缝接入，不扩展 `RemoteClient` 接口。** 现有 jump、认证、host key 和 close 语义集中在 `newSSHClient`，新实现只负责 listener、channel、copy 与存活监视；扩接口会波及 `fakeRemoteClient`（`internal/core/command_proxy_test.go:125`）等替身，收益不足。
8. **`target`/`address` 双模式，`tunnel edit` 延后。** 已登记 host 用 `--target`，未登记地址用 `--address` + `--auth`，避免 host 名称消失后被静默当作未登记地址；交互编辑留待后续。
9. **`tunnel add` 默认拒绝重名，`--force` 覆盖。** 避免误改共享配置。
10. **tunnels 参与 `cfg export/import`。** 迁移包必须携带隧道配置，避免换机静默丢失；冲突策略与 host 一致。
11. **tunnel 校验分级：`tunnel add` 硬校验，`cfg doctor` 只 warn。** 不把新集合变成 host/group/web/import 的全局写入门禁。
12. **存活与失效由 keepalive 加连接关闭观察实现，不自动重连。** 空闲会话可被网络静默断开，必须前台可见并有序退出。

## 待确认事项

本版已收敛 0.2 的 6 项询问，结论记录如下，剩余项为后续设计边界：

| 0.2 问题 | 本版结论 |
|---|---|
| `tunnel edit` 是否初版实现 | 不实现（决策 8） |
| `tunnel add` 重名覆盖策略 | 默认拒绝，`--force` 覆盖（决策 9） |
| `tunnel forward --json` 实际端口 | 支持；`--json` 就绪对象写 stdout，见输出约定 |
| remote endpoint 是否支持 Unix socket | 不支持，仅 TCP |
| Windows 控制事件与 stdin EOF | 只做跨平台 interrupt（Ctrl-C/SIGTERM）；stdin EOF 不作为退出条件 |
| 是否转发到 command_proxy 目标 | 不支持；需要先为 proxy backend 设计 TCP stream 语义 |

剩余待确认：

1. keepalive 间隔/阈值是否需要在配置中可调（首版沿用 login 常量，不新增配置项）。
2. `address` 模式是否允许非 IP 的 DNS 名称，还是限定为 IP 字面量；若允许，`tunnel add` 是否做一次解析探针。
3. 未登记 `address` 的 target 是否需要在 `tunnel list/show` 中显示解析结果（例如反查出的主机名），以便运维辨识。

## 结论与人工计划 Gate

本设计以 `sshc tunnel/tun` 作为 v1 命令组，支持 `tunnel add/list/show/rm/forward`，将命名 tunnel profile 保存到独立的 `tunnels` 配置集合，并让该集合与 `cfg doctor`、`cfg export/import`、`auth rm`/`host rm` 保持一致的校验和引用语义。`tunnel forward` 复用现有 SSH/auth/jump 连接，经包内 `forwardDialer`（`*remoteClient.Dial`）建立 direct-tcpip channel；本地只写端口时默认绑定 `127.0.0.1`，默认前台运行并拒绝非 loopback 监听；空闲会话通过 keepalive 与连接关闭观察保证失效可见。它覆盖"本地连接远程 DB/Redis"的核心结果，同时把远程转发、SOCKS、后台管理、审计日志和 command_proxy 转发留在后续边界。

当前为 `Draft 0.3`。需要用户确认命令组、持久化字段（`target`/`address`/`port`/`jump`）、export/import 覆盖与覆盖规则后，才能进入实施计划；设计批准本身不授权代码实施、提交、发布或部署。
