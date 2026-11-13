<!-- template_id: design; template_version: 1.2.0 -->
# sshc IPv6 目标与临时 SSH 路由设计

> 状态：Draft 0.1 / 待人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=让用户以 IPv6 zone 和临时端口安全连接 SSH 目标，并可复用已有本地转发或由 sshc 临时建立转发；allowed_scope=sshc 的 check/run/scp、host add、目标解析、连接与 known_hosts、测试及相关 CLI 帮助；non_goals=通用代理框架、后台转发、修改已保存 host 的临时覆盖结果、静默信任、额外多级跳板；expansion_policy=DEFER_OR_REQUEST；review budget=首轮设计安全与正确性审核一次，只有核心阻断才复评；停止条件=目标解析、实际拨号地址与 SSH 身份、资源关闭及用户可见验收均明确。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-13 | Suga | 初稿：IPv6 zone、临时端口、已有/内建本地转发与新主机密钥确认 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 背景与目标

实测版本 `sshc 0.12.3-1-g8e21d9a` 的 link-local IPv6 zone 目标被误拒，`run/scp/check` 无临时 `--port`，连接本地转发和批量处理未知 host key 不便。目标是在不改保存的主机身份、不降低 host key 安全性的前提下，让一次命令可选端口和路由，并看见最终解析地址及 zone。

## 名词

- **目标身份**：应验证其 SSH host key 的目标 IP/域名与 SSH 端口；不是转发监听地址。
- **拨号地址**：本次 TCP 实际连接地址，可以是目标、已有的 `127.0.0.1:port` 或 sshc 临时分配的回环端口。
- **跳板**：配置中已有且可独立认证、校验密钥的 SSH 主机；内建转发通过它访问目标地址。

## 范围与非目标

- `check/run/scp` 增加 `--port` 临时覆盖；已保存 host、带 `--auth` 的临时地址均适用，端口限 1..65535。位置参数支持 `v6%zone`、`[v6%zone]:port`，保留已有主机名/IPv4 行为；若目标中与 `--port` 同时指定端口，显式拒绝冲突。
- `host add --ip fe80::1%12 --port 22` 按原 IP 字段保留 zone；仍拒绝在 `--ip` 写 `[v6%zone]:port`，不增加持久化字段。
- `check` 的 `Addr`/JSON `address` 已有 host:port；增加明确的 zone 显示（没有 zone 时留空），并保证失败时也输出解析结果。
- 两种转发：`--via-local-port <1..65535>` 使用已经运行的本地回环转发；`--via-jump <已保存跳板名>` 由 sshc 创建仅绑定 `127.0.0.1:0` 的临时转发，命令结束即关闭。`--jump`、`--via-local-port`、`--via-jump` 不同时使用；已保存目标若带 jump 也须显式覆盖或报冲突，避免静默换路由。
- `--trust-new-host` 可在交互终端对未知密钥显示指纹并逐台确认；已知密钥变化、非交互、拒绝确认均失败。`check` 多目标时按顺序询问，不能让并发检查交错提示；不自动信任。
- 不新增 `--jump-addr`/`--proxy-target`：本轮内建转发的上游与远端目标由已保存跳板和已解析目标确定；如需要不属于已保存目标的另一远端地址，另行明确身份映射再设计。

## 已确认事实与规范

- 当前源码 `internal/core/batch_run.go:185-201` 遇到 `:` 即拒绝原始目标，错误本身使用 `%q`，并非直接将输入作为格式串；用户版本与当前源码须分别验证。`internal/core/store.go:234-237` 已用 `net.SplitHostPort` 防止保存的 IP 包含端口。
- `internal/core/config_resolve.go:77-113` 是临时目标与 `--auth` 解析入口；`internal/core/check.go:50-57,170-175` 已用 `net.JoinHostPort` 输出 `Address`。
- `internal/core/ssh.go:751-809` 的跳板与直连各有连接入口；`internal/core/ssh_knownhosts.go:64-73,296-348,487-493` 按目标地址选择算法并仅在未知密钥时走指纹确认。
- `internal/core/forward.go:117-167,205-211` 已实现 `StartLocalForward`、回环端口 0 的监听与资源归属；现有边界见 `docs/design/2026-09-27-sshc-port-forwarding-design.md`，不复制其转发底层设计。
- 适用 GR110（真实 CLI 分发测试）、SR1204（范围确认）、SR1107（设计/计划/实施 Gate）和 SR1301（认证、校验与密钥不泄漏）。

## 总体方案

1. 只对明确的 `host:port` 使用 `net.SplitHostPort`：方括号 IPv6 和单冒号主机名/IPv4；先用 `net/netip.ParseAddr` 识别完整的 IPv6（含 zone），禁止把末段当端口。保留原始 zone 字面量，校验端口和不完整括号；连接地址统一由 `net.JoinHostPort(host, port)` 构造。临时覆盖应用在 EffectiveHost 后，不写回 Store。
2. 连接建立区分拨号地址和目标身份：无转发则两者相同；经本地端口转发时 TCP 拨号使用回环地址，SSH 握手主机名、known_hosts 查找和算法选择始终使用目标地址。内建转发先独立校验跳板，再由跳板 `Dial` 到目标地址；目标 SSH 再独立校验其密钥。失败时保留原始错误并关闭跳板、listener、TCP 与 SSH 连接。
3. 交互信任只用于 `known_hosts` 未知项；`check` 默认仍不提示，显式 `--trust-new-host` 才允许交互；`run/scp` 沿用现有未知密钥确认行为，新增显式选项不放宽边界。

## 架构

```text
internal/command/check.go, run.go, upload.go, add.go: 选项与 CLI 分发
    -> internal/core: 原始目标 / EffectiveHost / 临时端口
    -> core SSH client: target identity + dial address 分离
       -> 直连 | 已有回环端口 | StartLocalForward(已保存跳板, 127.0.0.1:0 -> 目标)
       -> host key 验证（跳板与目标分别验证）、业务会话、defer 释放
```

代码组织维持现有 `cmd/sshc` 薄入口及 `internal/command`、`internal/core` 分工；沿用同仓库 `tunnel forward`（`internal/command/tunnel.go:248-300`）的监听与清理做法，不新建框架。与同工作区 `inhere-tools/gofer/cmd/gofer/main.go` 的薄入口组织一致。

## 关键流程

- `sshc check --auth ops --port 2222 'fe80::3b3c:ca9b:837c:e55a%12'` → 地址 `[fe80::3b3c:ca9b:837c:e55a%12]:2222`、zone `12`；TCP 和 SSH 使用该地址，错误中不得出现 `%!(MISSING)`。
- `sshc run --auth ops '[fe80::1%12]:2222' -- id` → 方括号仅作端口语法，认证和 known_hosts 保留目标 zone；`--port` 冲突时报错。
- `sshc run --via-local-port 10022 host -- id` → TCP 连接 `127.0.0.1:10022`，SSH 主机身份仍为保存的 `host.IP:host.Port`。
- `sshc run --via-jump bastion host -- id` → 先验证 bastion；临时监听 `127.0.0.1:0`，通过 bastion SSH channel 连到目标 IP:port，再按目标身份 SSH 握手；操作结束时关闭全部资源。`scp/check` 共用相同路由语义。
- `sshc check --trust-new-host --hosts a,b --parallel 1` → 两台分别显示指纹并确认；错误密钥不提示替换。默认并发时显式要求 `--parallel 1`，避免交互输入竞争。

## 安全、数据、运维与回滚

- 只绑定回环，不将用户本地端口开放给局域网；不将路由地址写入 known_hosts 作为目标身份，跳板和目标密钥分开校验。非交互不可借信任选项静默写入；密码、私钥、连接内容不进输出。
- 不新增持久化 schema；`Host.IP` 原样存储 zone，CLI 覆盖只在本次命令生效。用户原有 known_hosts 中未知密钥经确认后新增条目是唯一持久副作用。
- 测试覆盖：IPv4/IPv6/zone/方括号及非法端口、冲突选项；真实 CLI 分发；存储往返；拨号地址与 known_hosts 身份不同；跳板与目标独立密钥校验、拒绝/变更/非交互；转发失败与关闭释放。可运行本地 SSH 测试端验证握手；若真实转发环境不可用，报告 `UNVERIFIED_REAL_ENV`。
- 回滚只需回退代码；已确认写入的 known_hosts 条目不自动删除，避免误删其他可信记录。

## 决策

1. 两种本地转发都提供，内建方式使用已有保存的跳板及 OS 分配的回环端口；不新增代理协议。
2. zone 留在 `Host.IP`，不更改配置格式；内联端口仅用于临时目标。
3. 目标身份和拨号地址必须分离；任何连接路由不能隐式改变信任对象。
4. 信任选项只为交互确认新密钥，绝不作为 `insecure` 或 `--force` 的别名。

## 待确认事项

- **请用户确认接口命名与冲突策略**：内建方式暂命名 `--via-jump`，仅接受已保存的跳板；若已有目标配置了 jump，同时再指定路由参数时明确拒绝。若希望改为 `--jump-addr`、支持未登记跳板或沿用原 jump，需先明确凭据与 host key 身份。

## 结论与人工计划 Gate

本设计为 Draft 0.1；先请求批准“已保存跳板 + `--via-jump` 内建临时转发”及上述冲突/信任语义。设计批准仅允许进入独立审核与实施计划，不能视为允许实施；实施须经后续计划 Gate。用户此前已确认两种转发都支持，尚未确认具体 CLI 命名及设计版本。
