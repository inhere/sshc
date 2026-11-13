<!-- template_id: plan; template_version: 1.3.1 -->
# sshc IPv6 目标与临时 SSH 路由实施计划

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-13 | Suga | 基于已批准 Draft 0.7，拆分 IPv6 解析、路由/身份、命令接线和验收波次 |

> 仅任务合同、范围、风险、验证、生命周期或其他执行语义变化递增版本；纯 identity/provenance/元数据纠正不升版本。

## 目标与完成定义

目标：在 sshc 中实现 Draft 0.7 规定的直接 IPv6/zone 连接优先路径，并在直连不可达时提供显式 `--via-local-port` 和 `--via-jump` 路由，同时保持目标 host key 身份与实际 TCP 拨号地址分离。

完成定义：

- `check/run/scp` 支持 `--port`、目标 `v6%zone`、`[v6%zone]:port`，且端口/zone 解析由同一 core parser 完成；`host add --ip` 可原样保存 zone。
- 无显式 via 且无保存 `Jump` 时直接连接；保存 `Jump` 仍沿用既有行为；via 选项冲突按 Draft 0.7 拒绝。
- `--via-local-port` 使用回环已有端口；`--via-jump` 仅接受自身无 `Jump` 的已保存 SSH host，并自动清理临时转发。
- `IdentityAddress` 用于 SSH 握手、known_hosts、算法选择和指纹；`DialAddress` 只用于 TCP 拨号。
- `check` 输出固定 `target/address/zone` JSON 字段和表格列，解析失败仍输出结果行；显式 trust 时串行确认，目标与跳板策略一致。
- 真实 CLI 分发测试覆盖参数、`--` 边界、额外位置参数退出码 2 和 handler 不执行；focused core/command 测试及包级测试通过。
- 真实 IPv6/跳板环境完成一次验证；无法提供相同环境时，完成报告明确写出 `UNVERIFIED_REAL_ENV`，不得用 seam 测试替代声明真实通过。

## 范围、排除项与授权

范围：`inhere-tools/sshc` 的 `internal/core` 目标解析、SSH/known_hosts/forward/check 核心，`internal/command` 的 check/run/upload(scp) 接线、CLI 分发测试、相关文档与测试。

排除项：`download` 命令、SOCKS/通用 ProxyCommand、未保存跳板地址、额外多级跳板、后台转发、远程转发、持久化 schema 迁移、Web API、发布/安装/部署和 push。

- `host_or_non_offline_action=REQUIRED`

授权边界：本计划只授权后续在批准计划和当前执行请求下修改上述代码/测试/文档并创建本地 atomic commit；不授权真实 Secret 写入仓库、push、release、部署或未经单独 Gate 的真实外部连接。真实环境验证只使用已由用户提供的目标和既有认证，不记录密码/私钥。

## 输入与批准证据

- 设计：`docs/design/2026-11-13-sshc-ipv6-temporary-routing-design.md`，Draft 0.7。
- 设计批准证据：用户原话“批准 Draft 0.7，进入实施计划”。
- 用户反馈：IPv6 zone 解析、`run/scp/check --port`、`[v6%zone]:port`、本地转发、显式 host key trust 和 check 输出。
- 当前仓库基线：Git root `D:/work/inhere/my-tools-dev/inhere-tools/sshc`；branch `main`；HEAD `0689cab34a663864f9a80f79fb155833a76bc977`；基线工作树无未提交修改。
- 设计审核：Draft 0.7 的 Spec/Executability 与 Standards/Governance 阻断已闭合；本计划仍需独立计划审核和人工计划批准后才能实施。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | 统一目标/endpoint 的 IPv4、hostname、IPv6、zone、端口解析 | `net.SplitHostPort`、`net.JoinHostPort`、`net/netip`；`internal/core/batch_run.go:185-201`；`internal/core/forward.go:422-464,492-509`；`store.go:234-237` | stdlib 可复用拆分/规范化原语，现有校验只能部分复用 | 在 `internal/core` 增加统一 parser/normalizer，保留 host 保存校验和 forward 本地 loopback 规则 | OWNER_EXTENSION | 现有 raw/forward 校验拒绝 scoped IPv6 或把冒号当端口，无法满足统一 grammar | 不增加新依赖；所有入口调用同一 owner，避免 check/SSH/forward 分叉 |
| CAP-02 | 拨号地址与目标身份分离，含 via-local/via-jump、trust 传递和清理 | `newDirectSSHClient`、`newSSHClientForConnection`；`StartLocalForward`；`forwardDialer` seam；known_hosts callback | 复用现有 SSH auth、jump、forward session、keepalive、known_hosts | 在 `internal/core/ssh.go` 及 route/forward owner 中扩展 route options 和 callback 参数，不扩展通用代理 | OWNER_EXTENSION | 现有 `newSSHDial` 同时使用目标地址作为拨号/身份；forward remote endpoint 不支持 zone；jump trust 无统一传参 | 不新建代理模块；复用 existing session，集中关闭顺序和目标/jump身份 |
| CAP-03 | CLI 选项、check schema、真实 gcli 分发与 trust 交互 | `internal/command/{check,run,upload}.go`；`bootstrap.NewApp`；gcli v3.8.3；现有 trust callback | 复用各命令现有 options struct、`AddArg`、RunWithArgs、JSON/table 输出和交互 callback | 在各命令 owner 增加 options/路由校验；显式 `SetValidateNum(true)`；新增结果字段和 golden/dispatch 测试 | OWNER_EXTENSION | 当前命令缺少新 flags，check 解析失败直接返回，gcli 默认允许多余参数，run separator 可选 | 不增加 CLI framework；选项绑定命令局部 struct，保持入口薄 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 通用 ProxyCommand/新网络依赖 | CAP-02 | 删除后仍可用现有 SSH `Dial`、`StartLocalForward` 和 route adapter 满足本轮验收；引入会扩大协议和生命周期范围 |
| 新 CLI framework | CAP-03 | gcli 已完成命令注册、选项、位置参数和分发；新 framework 不增加当前用户结果 |

## 前置检查与 fail-closed 条件

1. 进入每个 mutation wave 前重新确认 Git root、branch、HEAD、status 和 owner 路径；发现他人 dirty work 或归属不明立即停止。
2. 设计 Draft 0.7 与本计划 revision 必须保持一致；发现需要新增代理协议、持久化字段、未保存跳板或安全策略时归类为 Semantic Amendment，返回 design/plan review，不在实现中自行扩围。
3. 所有地址 parser 失败必须 fail closed；不能 fallback 到按最后冒号切分，也不能把回环拨号地址写入目标 known_hosts。
4. `--trust-new-host` 只能处理未知 key；`knownhosts.KeyError.Want` 非空、非交互、用户拒绝均失败。
5. 真实网络验证前必须有目标、跳板和既有认证来源；不得为测试把 `host_key_check` 改为 insecure 或把凭据写入测试文件。
6. 基线 focused 测试先建立 characterization：当前 IPv6 raw target 被拒、当前 check 解析失败无结果行、当前 gcli 多余参数行为；新测试必须通过真实 app dispatch 而非只调用 handler。

## 波次与依赖

- Wave 1（T01）：统一目标与 endpoint parser、端口覆盖和 host 保存校验；无其他实现依赖。
- Wave 2（T02）：SSH route、identity/dial 分离、forward endpoint、trust 传递和清理；依赖 T01。
- Wave 3（T03）：check schema、run/scp/check CLI 接线与命令级路由校验；依赖 T01、T02。
- Wave 4（T04）：真实 CLI 分发、全量回归和真实环境验证；依赖 T03。

每个波次完成后只提交本波次 owner 文件，并在下一波前复核 status 与 focused 验证结果。

## 任务

### T01 统一 IPv6/zone/端口目标解析

- 文件: `internal/core/endpoint.go`（新增于现有 core package）；`internal/core/store.go`；`internal/core/batch_run.go`；`internal/core/config_resolve.go`；`internal/command/add.go` 如需要不直接改命令逻辑；`internal/core/resolve_test.go`；`internal/core/batch_run_test.go`；`internal/core/auth_test.go`
- 动作: 实现单一 target/endpoint parser：接受无内联端口的 `v6%zone` 和带端口的 `[v6%zone]:port`，hostname/IPv4 的明确 `host:port`，拒绝 `v6%zone:port`、空/非法 zone、非法端口和 `--port` 冲突；统一用 `net.JoinHostPort` 生成规范地址。让 `ResolveEffectiveHostWithAuth` 和保存 host 校验调用 parser，临时 `HostOverrides.Port` 只改本次 effective host；保留 `batch_run` 既有语义，若共享 helper 会改变 batch grammar，先拆成 target parser 与 batch policy 两层。
- 验证: 运行目录 `.`；`go test ./internal/core -run 'Test(Parse|ResolveEffectiveHost|ValidateRaw|Host)'`；新增/调整表驱动用例覆盖 `fe80::1%12`、`[fe80::1%12]:22`、`fe80::1%12:22`、IPv4/hostname、冲突端口、zone 原样保存和未持久化。
- 完成标准: 所有三类入口得到相同 `Host.IP/Port`；保存 host 不含端口；非法输入在 resolver 前 fail closed；T01 focused tests 通过。
- 依赖: 设计 Draft 0.7；无前置代码波次。

### T02 实现 route、拨号/身份分离和临时转发

- 文件: `internal/core/ssh.go`；`internal/core/ssh_knownhosts.go`；`internal/core/forward.go`；`internal/core/config_resolve.go`；`internal/core/check.go`；`internal/core/ssh_test.go`；`internal/core/auth_test.go`；`internal/core/forward_test.go`；`internal/core/jump_test.go`；`internal/core/tunnel_test.go`
- 动作: 增加 route/connection options，使 `DialAddress` 与 `IdentityAddress` 独立；直连默认二者相同，via-local 使用 `127.0.0.1:port` 拨号但目标地址参与握手、算法、known_hosts 和 fingerprint；via-jump 只接受已保存且自身无 `Jump` 的 bastion，通过现有 forward session 建立 `127.0.0.1:0` 临时 listener。将 trust/no-prompt 选项同时传给 target 和 bastion；保留变更 key 拒绝。扩展 remote endpoint normalizer 接受规范 `[v6%zone]:port`，不放宽本地 listener 必须 loopback、端口和非法 endpoint 规则。为认证、握手、listener、取消、业务错误定义逆序清理并保留首个错误。
- 验证: 运行目录 `.`；`go test ./internal/core -run 'Test(SSH|Known|Forward|Jump|Tunnel|Check)'`；新增 seam 断言拨号地址/identity 地址、目标与跳板独立 key callback、nested Jump 拒绝、scoped remote endpoint、未知/变更 key、listener/SSH 资源释放。
- 完成标准: 所有 route 矩阵在 core 层可观察；回环地址不会进入目标 known_hosts；所有失败路径无 listener/client/channel 泄漏；T02 focused tests 通过。
- 依赖: T01 parser；不得新增通用 proxy module。

### T03 接线 check/run/scp 及固定输出/交互契约

- 文件: `internal/command/check.go`；`internal/command/run.go`；`internal/command/upload.go`；`internal/command/util.go`；`internal/core/check.go`；`internal/command/check_test.go`；`internal/command/run_test.go`；`internal/command/upload_dl_test.go`；必要时 `internal/command/command_test.go`
- 动作: 给 check/run/scp 的 command-local options 增加 `--port`、`--via-local-port`、`--via-jump`、`--trust-new-host`，先做 via/Jump 冲突校验，再调用共享 route resolver；不修改保存 host。check 结果增加固定 `target/address/zone` JSON 字段，表格列固定为 `Name, Target, Addr, Zone, TCP, SSH, Auth, HostKey, Latency, Error`；解析失败保留一行且不把输入作为 printf format。trust 模式 check 强制 `--parallel 1`，并把 no-prompt/trust 传给 jump/target。run 非 script 模式强制 command 首项为 `--`，保留 separator 后以 `--` 开头的命令和 script 例外；scp 复用 route，不把 download 纳入本波次。
- 验证: 运行目录 `.`；`go test ./internal/command -run 'Test(Check|Run|Upload|Command)'`；新增 JSON/table golden、解析失败行、zone、via 冲突、run separator、script 例外、保存 Jump 冲突和 trust 并发拒绝用例。
- 完成标准: 三个命令都能接受 Draft 0.7 flags 并把 route 传到 core；成功/失败输出字段稳定；既有 command_proxy 和显式 `--jump` 回归通过；T03 focused tests 通过。
- 依赖: T01、T02；不得在命令 handler 中复制 parser 或 SSH host key 逻辑。

### T04 真实 CLI 分发、包级回归与真实环境验收

- 文件: `internal/bootstrap/init.go`（仅必要的 gcli command validation wiring）；`internal/bootstrap/run.go`（仅退出码链路需要时）；`cmd/sshc/main.go`（默认不改，仅验证薄入口）；`internal/bootstrap/*_test.go`；`internal/command/*_test.go`；必要时 `docs/README` 使用说明
- 动作: 对新增命令配置显式启用 gcli v3.8.3 `SetValidateNum(true)`（或等价 API）；通过 `bootstrap.NewApp().Run(args)` 或等价真实 app dispatch 覆盖新 flags、target 位置、run `--`、缺失 separator、separator 后命令以 `--` 开头、script 例外；附加未声明位置参数必须退出码 2 且 handler 不执行。运行 core/command 包级和全量测试。若有用户提供的真实 SSH/IPv6 环境，在不写入 Secret 的前提下验证直连 zone、via-local、via-jump、目标/跳板 host key 与 Ctrl-C 清理；否则记录 `UNVERIFIED_REAL_ENV`。
- 验证: 运行目录 `.`；`go test ./internal/bootstrap ./internal/command ./internal/core`；随后 `go test ./...`；真实环境命令按用户提供目标单独记录，不把凭据写入命令/仓库。
- 完成标准: 真实 app dispatch 证明参数边界和退出码；包级/全量测试通过；真实环境验证结果或 `UNVERIFIED_REAL_ENV` 已记录；`cmd/sshc/main.go` 仍无业务逻辑。
- 依赖: T01–T03；真实环境验证需人工提供目标和认证来源。

## 回滚与恢复

- 每个波次使用精确 owner 文件创建本地 atomic commit；失败时保留失败证据，不使用共享工作区 stash/reset/checkout 绕过 dirty work。
- 代码回滚按波次反向 revert；不改配置 schema，不需要数据迁移。已由用户确认写入 known_hosts 的条目不自动删除。
- 若实现发现新 CLI、代理协议、持久化字段、nested Jump 或安全边界需求，停止当前波次，分类为 Semantic Amendment 并回到 design/plan review。
- 若真实环境不可用，保留 seam/CLI 测试结果并披露 `UNVERIFIED_REAL_ENV`，不把模拟结果升级为真实网络通过。

## 人工 Gate

1. 本计划需通过一次独立计划审核，确认任务 owner、能力复用、依赖、验证和完成标准闭合。
2. 本计划批准后，仍需单独的当前执行请求才能修改代码；计划批准不授权实施、push、release、部署或外部消息。
3. 真实 SSH/IPv6/跳板验证前需要用户提供可用目标和认证来源；不得要求或记录 Secret。
4. push、release、部署、安装和任何外部动作另行 Gate。

## 可追溯性

| 设计/需求 | 实施任务 | 验证 |
|---|---|---|
| 直接 IPv6、zone、`[v6%zone]:port`、`--port`、host add 保存 | T01 | core parser/resolve/保存 focused tests |
| 直连优先、saved Jump、via-local/via-jump 冲突矩阵 | T02、T03 | route seam、jump/forward、command conflict tests |
| target identity 与 dial address 分离、known_hosts/trust | T02 | callback、fingerprint、unknown/changed key、cleanup tests |
| check target/address/zone 输出和失败行 | T03 | JSON/table golden 与 command dispatch tests |
| run/scp/check 新 flags、run `--`、gcli 参数错误 | T03、T04 | bootstrap app dispatch、exit code 2、handler-not-called tests |
| 真实边界和回滚 | T04 | package/full suite、real env 或 `UNVERIFIED_REAL_ENV` |

## 完成 Gate 与剩余工作

完成前必须满足：T01–T04 完成并各有 focused 证据；`go test ./...` 通过；计划范围内每个 owner 文件已精确提交；真实环境验证已完成或按要求披露 `UNVERIFIED_REAL_ENV`；无未分类 Semantic Amendment/Ownership Conflict；最终 status 与提交路径可核对。

计划完成不代表发布、安装或 push。任何后续发布行为需另行获得明确授权。
