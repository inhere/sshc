<!-- template_id: design; template_version: 1.2.0 -->
# sshc IPv6 目标与临时 SSH 路由设计

> 状态：Draft 0.4 / 待独立复评与人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=让用户以 IPv6 zone 和临时端口安全直接连接 SSH 目标，并可在直连不可达时显式复用已有本地转发或由 sshc 经 SSH host 建立临时转发；allowed_scope=sshc 的 check/run/scp、host add、目标解析、连接与 known_hosts、测试及相关 CLI 帮助；non_goals=通用代理框架、后台转发、修改已保存 host 的临时覆盖结果、静默信任、额外多级跳板；expansion_policy=DEFER_OR_REQUEST；review budget=修正后一次独立双轴复评；停止条件=直接连接优先级、目标解析语法、路由矩阵、主机身份、zone 作用域、资源释放和用户可见验收均明确。暴露度：本机工具档；仅维护者本机构建/私有使用，错误可回退，修改不迁移持久数据、不新增 Secret 处理或出机器动作，不放宽认证拒绝门。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-13 | Suga | 初稿：IPv6 zone、临时端口、已有/内建本地转发与新主机密钥确认 |
| 0.2 | 2026-11-13 | Suga | 根据独立功能审核修正：直接 IPv6 连接为默认路径，补齐 CLI grammar、路由矩阵、zone 作用域、host key 身份和生命周期约束 |
| 0.3 | 2026-11-13 | Suga | 根据复评修正：把 scoped IPv6 远端 endpoint 校验和跳板 trust 策略传递纳入本轮 owner 范围 |
| 0.4 | 2026-11-13 | Suga | 根据安全轴复评修正：`--via-jump` 仅接受自身无 `Jump` 的直连 SSH host，拒绝隐式多级跳板 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 背景与目标

实测版本 `sshc 0.12.3-1-g8e21d9a` 的 link-local IPv6 zone 目标被误拒，`run/scp/check` 无临时 `--port`，连接本地转发和批量处理未知 host key 不便。目标是在不改保存的主机身份、不降低 host key 安全性的前提下，让一次命令可选端口和路由，并看见最终解析地址及 zone。

## 名词

- **目标身份**：应验证其 SSH host key 的目标 IP/域名与 SSH 端口；不是转发监听地址。
- **拨号地址**：本次 TCP 实际连接地址，可以是目标、已有的 `127.0.0.1:port` 或 sshc 临时分配的回环端口。
- **跳板**：配置中已有且可独立认证、校验密钥的 SSH 主机；内建转发通过它访问目标地址。

## 范围与非目标

- **默认路径是直接连接**：没有显式 `--via-local-port`/`--via-jump` 时，未设置保存的 `Host.Jump` 就直接连接目标；保存的 `Host.Jump` 仍按既有配置生效。直接 IPv6 可达时不需要任何转发选项。
- `check/run/scp` 增加 `--port` 临时覆盖；已保存 host、带 `--auth` 的临时地址均适用，端口限 1..65535。位置参数支持 `v6%zone`、`[v6%zone]:port`、IPv4/hostname 的 `host:port`；未加方括号的多冒号值只按 IPv6 地址解析，不推断端口；目标内联端口与 `--port` 同时出现时显式拒绝冲突。
- `host add --ip fe80::1%12 --port 22` 按原 IP 字段保留 zone；仍拒绝在 `--ip` 写 `[v6%zone]:port`，不增加持久化字段。转发远端 endpoint 同样接受 `v6%zone:port` 的结构化内部表示，并保留 loopback、本地端口及其他 endpoint 拒绝规则。
- `check` 的 `Addr`/JSON `address` 输出规范化 host:port，并增加 `zone` 字段；解析失败也返回包含原始 target、可解析地址（若有）和 zone（若有）的结果，不把 `%` 作为格式串。
- CLI grammar 固定为：`run [options] <target> -- <command>`、`scp [options] <target> ...`、`check [options] <target>`；`--port`、`--via-local-port`、`--via-jump`、`--trust-new-host` 绑定到各自命令，不依赖未声明的位置参数。
- `--via-local-port <port>` 使用已有的 `127.0.0.1:<port>`；`--via-jump <saved-host>` 只接受已保存 SSH host，且该 host 自身不得设置 `Jump`，由 sshc 为本次命令建立 `127.0.0.1:0 -> target` 临时转发。两者互斥，且均与显式 `--jump` 互斥；目标保存 `Jump` 也与显式 via 冲突。
- 路由矩阵固定：无显式 via 且目标无保存 `Jump`=直连；无显式 via 且目标有保存 `Jump`=沿用保存跳板；显式 via 与目标保存 `Jump` 同时存在=拒绝；显式 `--jump` 与保存 `Jump`=覆盖（既有语义），但再与任一 via 同时存在=拒绝。没有 `--no-jump`，避免新增隐式直连语义。
- `--trust-new-host` 仅允许未知 key 的交互指纹确认；已知 key 变化、非交互、拒绝确认仍失败。`check` 使用该选项时强制 `--parallel 1`；未指定时 check 不交互，且该策略必须同时传递给目标连接和 `--via-jump` 的跳板连接。run/scp 未指定时保留既有 known_hosts 行为，指定时显式启用同一确认回调。
- 不新增 `--jump-addr`/`--proxy-target`：本轮内建转发的上游由已保存 SSH host 确定；需要未保存跳板或非 SSH TCP 代理时另行设计。

## 已确认事实与规范

- 当前源码 `internal/core/batch_run.go:185-201` 遇到 `:` 即拒绝原始目标，错误本身使用 `%q`，并非直接将输入作为格式串；用户版本与当前源码须分别验证。`internal/core/store.go:234-237` 已用 `net.SplitHostPort` 防止保存的 IP 包含端口。
- `internal/core/config_resolve.go:77-113` 是临时目标与 `--auth` 解析入口；`internal/core/check.go:50-57,170-175` 已用 `net.JoinHostPort` 输出 `Address`。
- `internal/core/ssh.go:751-809` 的跳板与直连各有连接入口；`internal/core/ssh_knownhosts.go:64-73,296-348,487-493` 按目标地址选择算法并仅在未知密钥时走指纹确认。
- `internal/core/forward.go:117-167,205-211` 已实现 `StartLocalForward`、回环端口 0 的监听与资源归属；现有边界见 `docs/design/2026-09-27-sshc-port-forwarding-design.md`，不复制其转发底层设计。
- 适用 GR110（真实 CLI 分发测试）、SR1204（范围确认）、SR1107（设计/计划/实施 Gate）和 SR1301（认证、校验与密钥不泄漏）。

## 总体方案

1. 目标解析先做完整地址识别，再做仅在语法明确时的端口拆分：`[host]:port` 解析方括号内容；单冒号 IPv4/hostname 使用 `net.SplitHostPort`；无方括号且含多个冒号时，只有完整 IPv6（含非空 zone）才接受为无内联端口目标，否则报格式错误。端口必须为 1..65535；`--port` 只能覆盖未带内联端口的目标。解析结果统一为 `Host.IP`（保留 zone）和 `Host.Port`，地址统一由 `net.JoinHostPort` 构造。供 `StartLocalForward` 使用的 remote endpoint 复用同一 scoped IPv6 parser/normalizer，不调用只接受 `net.ParseIP` 的旧校验路径；本地 listener 的 loopback 和端口限制保持不变。
2. 直连是默认传输路径。路由选择在命令层一次完成并生成不可变 `ConnectionRoute`：`DialAddress`（实际 TCP 地址）、`IdentityAddress`（目标 host:port）、可选 `ForwardSession` 和关闭函数；不能在 core SSH client 内再次依据字符串猜测路由。
3. 连接建立严格区分拨号地址和目标身份：直连时二者相同；`--via-local-port` 时 TCP 拨号使用回环地址，但 SSH 握手的 server name、known_hosts 查找、算法选择和指纹提示使用 `IdentityAddress`；`--via-jump` 先用跳板 host 的自身身份建立 forward session，再让目标 client 通过临时回环 listener 拨号，目标身份仍为目标地址。跳板和目标分别回调、分别关闭。
4. zone 作用域随拨号位置确定：直连和 `--via-local-port` 中 zone 属于本机；`--via-jump` 中远端 endpoint 由跳板发起，因此 zone 必须是跳板机上的接口 zone。命令帮助和错误提示明确这一点，不自动把本机 zone 改写给跳板。
5. `--trust-new-host` 只包裹未知 key 分支；回调必须拒绝 `knownhosts.KeyError.Want` 非空的变更 key。check 的交互路径强制串行；`TrustNewHost` 作为连接选项沿目标 route 传给目标 client 和 jump forward dialer，未设置时 jump 与 target 都使用 no-prompt callback。所有临时 listener、forward session、目标 TCP/SSH client 在成功、认证失败、握手失败、命令失败和取消路径均按逆序清理，并保留首个业务错误。

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

- `sshc run --auth ops --port 2222 'fe80::3b3c:ca9b:837c:e55a%12' -- hostname` → 无 via 时直接从本机拨号；地址为 `[fe80::3b3c:ca9b:837c:e55a%12]:2222`，zone `12` 是本机接口。
- `sshc run --auth ops '[fe80::1%12]:2222' -- id` → 方括号明确提供内联端口；再传 `--port` 时报冲突。
- `sshc run --via-local-port 10022 host -- id` → TCP 连接 `127.0.0.1:10022`，SSH 主机身份仍为保存的 `host.IP:host.Port`；若 `host` 有保存 `Jump`，直接拒绝该组合。
- `sshc run --via-jump bastion host -- id` → `bastion` 必须是已保存且自身无 `Jump` 的 SSH host；先验证 bastion，再启动 `127.0.0.1:0 -> host` 临时转发，通过本地 listener 建立目标 SSH 握手；目标 zone 必须在 bastion 上有效。若 bastion 自身配置了 `Jump`，命令在建立连接前拒绝，避免隐式多级跳板。操作结束时按“目标 client、forward listener、bastion client”顺序关闭全部资源。`scp/check` 共用相同路由矩阵。
- `sshc check --trust-new-host --hosts a,b --parallel 1` → 依次显示目标地址、zone 和指纹并确认；`--parallel >1` 直接报错，默认不交互。

## 安全、数据、运维与回滚

- 只绑定回环，不将用户本地端口开放给局域网；不将路由地址写入 known_hosts 作为目标身份，跳板和目标密钥分开校验。非交互不可借信任选项静默写入；密码、私钥、连接内容不进输出。
- 不新增持久化 schema；`Host.IP` 原样存储 zone，CLI 覆盖只在本次命令生效。用户原有 known_hosts 中未知密钥经确认后新增条目是唯一持久副作用。
- 默认直连、保存的 `Jump` 与三个显式路由选项的矩阵必须由真实 CLI 分发测试覆盖；不能只测试 resolver 函数。
- `IdentityAddress` 与 `DialAddress` 必须在测试 seam 中分别断言；本地转发下 callback、known_hosts 查询和 fingerprint 输出不得出现回环地址。`StartLocalForward` 的远端 endpoint 需要覆盖带 zone 的 IPv6，并反向验证普通非法 endpoint、本地非 loopback 和端口错误仍失败。
- IPv6 zone 的接口作用域必须分别覆盖本机直连和跳板远端拨号；无法提供真实双机环境时，至少使用目标地址和 callback seam 验证，完成报告披露 `UNVERIFIED_REAL_ENV`。
- `--via-jump` 的验收必须覆盖：直连 bastion 成功、bastion 自身带 `Jump` 时在拨号前拒绝、bastion 未知/变更 key、目标未知/变更 key，以及所有失败路径的资源释放；不得通过测试多级跳板来扩大本轮范围。
- 回滚只需回退代码；已确认写入的 known_hosts 条目不自动删除，避免误删其他可信记录。

## 决策

1. **直接 IPv6 连接是默认路径。** 只要本机有到目标的路由，用户无需指定 via；zone 使用本机接口。
2. **显式路由才改变拨号位置。** `--via-local-port` 和 `--via-jump` 互斥，且不与目标保存 `Jump` 或 `--jump` 混用；拒绝优先于静默覆盖。
3. **`--via-jump` 跟已保存且直连的 SSH host。** 它不是普通 TCP 代理，也不接受未登记跳板地址；via host 自身带 `Jump` 时拒绝，避免本轮外的隐式多级跳板；跳板的认证和 host key 独立解析。
4. **目标身份和拨号地址必须分离。** 任何路由不能改变目标 known_hosts 身份；zone 的有效接口由实际发起目标 TCP 连接的一端决定。
5. **信任选项只为交互确认新密钥。** 绝不作为 `insecure` 或 `--force` 的别名。

## 待确认事项

本轮不再保留核心行为待确认项：用户已确认“IPv6 直连优先，转发作为不可达时的显式能力”，并同意同时支持已有本地端口和 sshc 内建转发。以下属于实施期验证而非设计选择：目标 IPv6 zone 在实际运行环境中是否与本机/跳板接口索引一致；若不一致，命令必须按设计失败并提示实际拨号位置。

## 结论与人工计划 Gate

Draft 0.4 已补入安全轴发现的多级路由边界：`--via-jump` 的 saved host 必须自身无 `Jump`，否则在连接前拒绝。请再次进行独立 changed-scope 复评；复评无 `CORE_BLOCKING` 后，才能据此起草实施计划。设计批准不授权代码实施。
