<!-- template_id: design; template_version: 1.2.0 -->
# sshc scan 局域网 SSH 目标发现设计

> 状态：Draft 0.1 / 已批准（2026-11-17，用户原话：批准 Draft 0.1，进入实施计划）/ 实施计划见 `docs/plan/2026-11-17-sshc-scan-plan.md`
>
> thinking_mode=RIGOROUS；core_objective=让用户一条命令发现直连小交换机上的 IPv4/IPv6 SSH 服务器，并把结果一键纳入 sshc 主机配置；allowed_scope=新增 `sshc scan`（别名 `discover`）命令、IPv4 子网枚举、IPv6 邻居缓存读取、私有网段守卫、TCP+SSH banner 探测、表格/JSON 输出、`--save` 入库及对应测试与帮助；non_goals=全端口扫描、漏洞或 OS 指纹识别、非私网/公网目标、自动写入凭据、IPv6 主动组播探测（二期）；expansion_policy=DEFER_OR_REQUEST；review budget=本机工具档一轮合并轴评审；停止条件=发现来源、地址/zone 规范、守卫边界、banner 判定、入库命名与冲突策略、退出码与验收均明确。暴露度：本机工具档；仅维护者本机运行、只做本链路探测与本地配置追加，错误可由删掉新增 host 回退，不迁移数据、不新增 Secret、不做出机器动作。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-17 | Suga | 初稿：命令名 `scan`/别名 `discover`、IPv4 枚举 + IPv6 邻居缓存、私网守卫、banner 识别、可选入库 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 背景与目标

用户直连一台小交换机时，手上有 link-local 的 `fe80::…%12` 这类地址，但不知道网段里还有哪些机器开了 SSH，只能逐个手工 `ping`/`ssh` 试。目标：

- `sshc scan` 一次列出本链路可发现的 SSH 服务器（地址、zone、端口、banner、延迟）；
- 能直接把这些结果落到 sshc 配置里，随后 `sshc run <name>` 即可使用；
- 默认只用本机已有信息（网段枚举 + 邻居缓存），不加特权、不发主动探测报文。

## 名词

- **发现来源**：本轮只有两种——`cidr`（按本机接口或 `--cidr` 枚举的 IPv4 地址）和 `neighbor`（系统 IPv6 邻居缓存条目）。
- **SSH 端口确认**：TCP 连接建立后读到以 `SSH-` 开头的首个协议行（RFC 4253 4.2），比"端口开着"更可信。
- **zone**：IPv6 link-local 地址的作用域标识（本机接口索引或接口名），`fe80::1%12` 中的 `12`。

## 范围与非目标

- 新增命令 `scan`，别名 `discover`；不修改 `run/scp/check/tunnel` 的既有行为。
- 发现来源固定为两类：IPv4 子网枚举（本机接口地址+掩码，或 `--cidr` 显式网段）；IPv6 系统邻居缓存。
- 探测对象固定为 `--port` 指定端口，默认仅 `22`；不做全端口扫描。
- IPv6 一期**不枚举** `/64` 网段、**不发** ICMPv6 组播或 NDP 主动探测；邻居缓存为空时如实报告"未发现"，并提示可用 `--cidr` 或先 `ping6 ff02::1%<if>` 预热缓存。
- `--save` 默认关闭；开启时只追加 host（名称+IP+端口+分组），不写入用户名、密码、密钥或 `--auth`。
- 非私网目标（IPv4 非 RFC1918/169.254、IPv6 非 `fe80::/10`/`fc00::/7`）默认拒绝，需显式 `--force`。
- 不做端口 0/广播/组播地址探测；不做跨网段路由发现。

## 已确认事实与规范

- 现有 CLI 组织：`internal/command/*.go` 建命令、`internal/core/*.go` 放领域逻辑，入口 `cmd/sshc/main.go` 保持薄；`internal/command/check.go:31-80` 是"主机集合 + 并发 + 表格/JSON"的现行参照。
- 现有探测原语：`internal/core/check.go:43-45` 的 `checkTCPDial`（可替换、可测）与 `internal/core/ssh_knownhosts.go:487-493` 的 `knownHostAddress`（`net.JoinHostPort`，保留 zone）可直接复用，不新建连接层。
- 现有配置写入：`internal/core/store.go:380-400,571-575` 的 `LoadStore/SaveStore`；`validateHost`（`store.go:206`）与 `NormalizeHostFields`（`store.go:280`）负责保存前校验与规范。
- 目标解析：`internal/core/endpoint.go:20-93` 已支持 `fe80::…%zone`、`[v6%zone]:port`，zone 原样保留，不新增语法。
- 适用 SR1403（不做→复用→stdlib→最小实现）、SR1405（验收外 deferred）、SR1410（默认最小充分方案）、SR1204（范围确认：本设计预计 5 个逻辑文件、>100 行，需批准）、SR2003（gcli 选项绑定与位置参数声明）、GR110（真实 CLI 分发测试）、GR104（单文件 ≤1000 行）、SR1301（不泄露凭据、入参校验）。

## 总体方案

1. **目标集合构建**（`core/scan.go`）
   - `--cidr` 可重复；未提供时取本机各 IPv4 接口地址与掩码，构造同网段主机地址（跳过网络号、广播号与本机地址）。
   - 枚举上限保护：单次最多 `--max-hosts`（默认 4096）个地址，超出直接报错并要求用 `--cidr` 收窄。
2. **IPv6 来源**（`core/scan_neighbors.go`，平台分支）
   - Windows：解析 `netsh interface ipv6 show neighbors`；Linux：解析 `ip -6 neigh show`；其它平台返回"不支持"而不是失败。
   - 只接受 `fe80::/10`、`fc00::/7` 与全局单播；跳过 `::1`、组播、`incomplete`/`failed` 状态；zone 取系统给出的接口索引/名称。
   - 读取失败（命令不存在/输出格式变化）不影响 IPv4 结果，只在结果摘要里报告来源不可用。
3. **探测**（`core/scan_probe.go`）
   - 并发 worker（默认 `--parallel 128`），每个目标 `DialTimeout` 使用 `--timeout`（默认 400ms）。
   - 连接成功后在同一个 deadline 内读首行；`SSH-` 前缀记为 SSH（带 banner），其它记为 `open`，连接失败/超时不输出。
   - 结果字段固定：`address`（`host:port` 规范串，IPv6 带 zone）、`family`、`zone`、`interface`、`banner`、`latency_ms`、`source`。
4. **输出与退出码**
   - 默认表格列固定：`Addr, Zone, Port, Banner, Latency, Source`；`--json` 输出数组。
   - 有发现、无发现都退出 0；参数错误、守卫拒绝、`--max-hosts` 溢出退出 2（gcli `ERR`）；Ctrl-C 取消时输出已发现结果后退出 130。
5. **入库**（`--save`）
   - 名称生成：IPv4 → `lan-192-168-1-20`；IPv6 → `lan-<后 4 个 hextet，冒号换连字符>`；重名追加 `-2`、`-3`。
   - 写入字段仅 `Name/IP/Port/Group`（`--group`，默认 `lan`）；已存在同名且 IP+端口相同的 host 跳过并计数，不覆盖用户已有条目。
   - 保存前统一走 `validateHost` + `NormalizeHostFields`，只调用一次 `SaveStore`。

## 架构

```text
internal/command/scan.go        选项绑定、guard 校验、表格/JSON 输出、--save 编排
    -> internal/core/scan.go          目标集合：--cidr / 本机 IPv4 接口枚举 + 上限保护
    -> internal/core/scan_neighbors.go IPv6 邻居缓存（windows/linux/其它）
    -> internal/core/scan_probe.go    并发 DialTimeout + SSH banner 判定
    -> internal/core/store.go         --save 追加 host（复用 validate/normalize/SaveStore）
```

代码组织维持 `internal/command`（CLI）+ `internal/core`（领域）分工，不新建包或框架；平台差异只在邻居缓存读取处按 `_windows.go`/`_linux.go` 文件区分，其余共用。

## 关键流程

1. `sshc scan` → 构建目标集合（本机 IPv4 网段 + 邻居缓存）→ 私网守卫过滤 → 并发探测 → 表格输出。
2. `sshc scan --cidr 192.168.1.0/24 --json` → 只用给定网段，输出 JSON 供脚本处理。
3. `sshc scan --save --group sw1` → 探测后在表格之后输出 `saved: N, skipped: M`，随后 `sshc run lan-192-168-1-20 -- hostname` 可用。
4. `sshc scan --cidr 10.0.0.0/8` → 超过 `--max-hosts`，报错并提示收窄网段，不发起任何探测。

## 安全、数据、运维与回滚

- 只探测本链路私网地址；公网网段需 `--force`，避免误扫外部网络。
- 并发与超时可配置但设上限（`--parallel` ≤ 1024，`--timeout` ≤ 10s），避免把小交换机打满。
- `--save` 只追加不含凭据的 host 条目；不写密码/密钥/`--auth`。
- banner 只作识别信息展示，不当作设备身份或信任依据；不改变 known_hosts 行为。
- 回滚：`sshc host remove <name>`（现有命令）删除自动添加的条目即可；扫描本身不改变任何远程状态。

## 决策

- D1：命令名 `scan`，别名 `discover`（用户确认）。
- D2：IPv6 一期只读邻居缓存，主动组播探测列二期（用户确认）。
- D3：`--save` 默认关闭，需显式开启（用户确认）。
- D4：保留 `--cidr` 手动指定网段作为兜底（用户确认）。
- D5：只认 `SSH-` 首行 banner 作为 SSH 判据；不引入版本/漏洞识别。
- D6：邻居缓存读取用系统命令输出解析，不引入第三方依赖。

## 待确认事项

- 二期（`ff02::1` 主动探测、Windows `arp -a` 加速、mDNS）不在本设计范围内，需另行设计。
- 是否需要 `--family ipv4|ipv6` 显式开关：默认两者都做；若 IPv6 来源不可用则自动降级。计划阶段可决定是否暴露该选项。

## 结论与人工计划 Gate

本设计确定 `sshc scan` 的发现来源、守卫边界、探测判定、输出契约与入库策略，实施范围约 5 个逻辑文件。

**需要你批准 Draft 0.1 后才能进入实施计划**（批准进入规划，不等于授权实施）。
