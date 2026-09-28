<!-- template_id: plan; template_version: 1.2.0 -->
# sshc 本地端口转发 实施计划

> 状态：Draft 0.1 / 待人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=按已批准的 design Draft 0.3 实现 `sshc tunnel/tun` 的本地 TCP 端口转发 v1（host/address 双目标、多规则、前台会话、空闲存活监视、配置与迁移/doctor/引用一致性）；scope_freeze=internal/core、internal/command、internal/bootstrap、README 与 docs/TODO.md 及对应测试；non_goals=远程转发、SOCKS、后台 daemon、Web API、转发审计日志、command_proxy 转发；expansion_policy=DEFER_OR_REQUEST；review budget=一轮计划评审；停止条件=任务文件、动作、验证命令与完成标准可独立执行，且无未决设计歧义时停止。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-09-28 | Jcode | 初稿：按 design 0.3 拆出隧道配置与解析、持久化/doctor/export-import、转发核心与会话生命周期、CLI 命令组、引用完整性、文档与验收闭环共 7 个任务 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 目标与完成定义

完成定义（可观察）：

1. `sshc tunnel add/list/show/rm/forward` 与别名 `tun` 可用，`add` 支持 host（`--target`）与 address（`--address` + `--auth`）两种目标、重复 `--forward`、`--port`、`--jump`、`--force`。
2. `tunnel forward` 前台运行：多规则共用一个 SSH 会话，`local:0` 打印系统分配的实际端口，`--json` 在 stdout 只输出一行就绪对象。
3. 空闲存活：keepalive 超时或 SSH 连接关闭时，listeners/活动连接被关闭，`Wait()` 返回非零并给出原因；不自动重连。
4. `tunnels` 进入 `cfg export/import` 合并与统计；`cfg doctor` 对 tunnel 问题只产出 `warn`，不影响 host/group/web/import 写入。
5. `auth rm` 拒绝删除被 tunnel 引用的 profile；`host rm` 对被 tunnel `target` 引用的 host 给出提示。
6. design 0.3 验收表中 A1-A8 全部有自动化证据；A9（真实主机）作为人工步骤交接。
7. README 与 `docs/TODO.md` 更新。

不包含：后台/pid、`ssh -R`、SOCKS、Web console 转发 API、转发审计日志、command_proxy 转发。

## 范围、排除项与授权

- 实施范围（owner 粒度）：`internal/core`（隧道模型/校验/转发核心）、`internal/command`（tunnel 命令组与引用完整性）、`internal/bootstrap/init.go`（注册命令）、`README.md`、`README.zh-CN.md`、`docs/TODO.md` 及对应 `_test.go`。
- 排除项：`web/` 与 `internal/server` 不改动（design 非目标）；不新增第三方依赖；不写 run log schema；不做配置版本号升级（`tunnels` 缺省即空集合）。
- `host_or_non_offline_action=NOT_APPLICABLE`
- 授权边界：本计划批准只授权实施与本地验证（`go build`/`go test`/`gofmt`）。不授权 push、发布、部署、真实主机批量操作；A9 由用户在本机执行，不占用 Agent 的 host 动作授权。
- 可追溯性要求：任何无法映射到第 5 节任务与验证的改动不得进入本轮范围。

## 输入与批准证据

- 设计输入：`docs/design/2026-09-27-sshc-port-forwarding-design.md`，Draft 0.3，commit `716428c`，validator `--kind design` = `{"ok": true, "errors": []}`。
- 评审输入：`docs/review/2026-09-28-sshc-port-forwarding-design-review.md`（结论 BLOCKED 针对 candidate 0.2 `0308bf4`；处置记录给出 F1-F11 在 0.3 的落点）。
- 用户批准证据：用户消息「提交，批准进入实施计划」（2026-09-28）——批准内容为提交评审/设计修订并进入计划阶段。
- 关联历史文档（只读参考）：`docs/2026-07-08-sshc-usability-enhancements-design.md` 的 P2 tunnel 提案（已被 design 0.3 取代）、`docs/plan/2026-07-07-sshc-serve-v1-plan.md`（历史计划风格）。
- workspace baseline：Git root `D:\work\inhere\my-tools-dev\inhere-tools\sshc`；branch `main`；HEAD `716428c`；`git status` clean；无他人 dirty/untracked 归属冲突。基线验证：`go build ./...` 通过，`go test ./... -count=1` 在 `internal/bootstrap`、`internal/command`、`internal/core`、`internal/server` 全绿。工具链 `go1.25.10 windows/amd64`；CI 使用 `go test -cover ./...`（Go 1.25/stable）。
- expected paths / symbols（planning evidence，非封闭白名单）：

| 文件 | 新增/修改 | 预计符号 |
|---|---|---|
| `internal/core/tunnel.go` | 新增 | `TunnelForward`、`TunnelProfile`、`ParseForwardRule`、`NormalizeTunnelProfile`、`ValidateTunnelProfile`、`FindTunnel`、`UpsertTunnel`、`RemoveTunnel`、`TunnelsUsingAuth`、`ResolveTunnelHost`、`ValidateTunnelTarget` |
| `internal/core/tunnel_test.go` | 新增 | A1/A7 相关表驱动测试 |
| `internal/core/forward.go` | 新增 | `ForwardRule`、`ForwardOptions`、`ForwardSession`、`forwardDialer`、`remoteDialer`、`newForwardDialer`、`StartLocalForward`、accept/copy/keepalive 内部函数 |
| `internal/core/forward_test.go` | 新增 | A2-A6 测试与假 dialer |
| `internal/core/store.go` | 修改 | `Config.Tunnels`、`normalizeConfig` 空集合处理 |
| `internal/core/config_doctor.go` | 修改 | `checkTunnels`（warn 级） |
| `internal/core/config_export.go` | 修改 | tunnel 合并分支、`ImportResult.TunnelsAdded/TunnelsUpdated` |
| `internal/core/config_test.go` / `config_export_test.go` | 修改/新增 | doctor warn、export/import 往返（A7/A8） |
| `internal/command/tunnel.go` | 新增 | `NewTunnelCmd` 及 `add/list/show/rm/forward` 子命令 |
| `internal/command/tunnel_test.go` | 新增 | A4/A7 CLI 级测试 |
| `internal/command/auth.go` | 修改 | `auth rm` 检查 tunnel 引用 |
| `internal/command/host.go` | 修改 | `host rm` tunnel 引用提示 |
| `internal/command/util.go` | 修改（如需） | tunnel 目标解析辅助（host/address + port/jump 覆盖） |
| `internal/bootstrap/init.go` | 修改 | 注册 `command.NewTunnelCmd()` |
| `README.md`、`README.zh-CN.md`、`docs/TODO.md` | 修改 | tunnel 用法与进度条目 |

- 依赖与能力：`golang.org/x/crypto v0.54.0`（ssh: `Conn.SendRequest`、`Conn.Wait`）、`github.com/melbahja/goph v1.5.2`（`Client.Dial`）、`github.com/gookit/gcli/v3`、`github.com/gookit/cliui`、标准库 `net`/`io`/`os/signal`。无新增依赖。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | 经 SSH 会话建立 remote TCP channel（direct-tcpip） | `github.com/melbahja/goph` `(*Client).Dial`、`internal/core/ssh.go` 既有 `*remoteClient.Dial`、`ssh.Client.Dial` | `*remoteClient.Dial` 已覆盖直接连接与 jump 场景 | `internal/core/forward.go` 内 `remoteDialer` 薄适配 4 个方法（Dial/SendKeepalive/Wait/Close） | THIN_ADAPTER | `RemoteClient` 接口不含 `Dial`，不能直接对接口拨号（`ssh.go:63-95`） | 不复制建连/认证/jump；适配层与 `newSSHClient` 同生命周期，无第二实现 |
| CAP-02 | 本地 TCP 监听与双向字节转发 | 标准库 `net.Listen`、`io.Copy`、`net.TCPConn.CloseWrite`；第三方 tunnel 库（见 Rejected new tools） | 标准库已足够，无需库 | `internal/core/forward.go` 的 listener/accept/copy 编排 | DIRECT_REUSE | 无 | 不引入转发框架；关闭顺序与 goroutine 回收由单一 session 拥有 |
| CAP-03 | 空闲会话存活与失效检测 | `startSessionKeepalive`（`ssh.go:240-267`，当前仅 login 使用）、`ssh.Conn.SendRequest`、`ssh.Conn.Wait` | 复用 keepalive 常量与请求形态 | 在 `forward.go` 内按同一形态接入会话关闭观察 | OWNER_EXTENSION | `newSSHClient` 路径无任何 keepalive/失效观察 | 不修改 login 行为；keepalive 逻辑仍单点定义（常量复用，不复制实现） |
| CAP-04 | CLI 命令组、选项与信号处理 | `gcli/v3` 命令组（`internal/command/auth.go`、`group.go` 模式）、`os/signal` | 现有命令组模式直接复用 | `internal/command/tunnel.go` 新命令组文件 | DIRECT_REUSE | 无 | 不新建 CLI 框架；与 `host/auth/cfg` 同级注册 |
| CAP-05 | 转发规则与 endpoint 解析 | 标准库 `net.SplitHostPort`、`net.JoinHostPort`、`strconv`；OpenSSH 冒号语法 | 标准库可用于 `local=remote` 自定义语法 | `ParseForwardRule` 统一规范化两侧 | DIRECT_REUSE | 无（`-L` 冒号语法已在 design 决策 5 中排除） | 解析规则单点实现，命令与 core 不各自解析 |
| CAP-06 | 配置持久化、doctor 与 export/import | `SaveConfig`/`normalizeConfig`（`store.go:561-621`）、`CheckConfig`（`config_doctor.go:20-33`）、`MergeImportedConfig`（`config_export.go:157-276`） | 原子写入与合并框架直接复用 | 既有 owner 扩展：新增 `Tunnels` 字段、`checkTunnels`、tunnel 合并分支与统计字段 | OWNER_EXTENSION | 现有合并与统计不覆盖新集合（F4） | 不新建配置存储；避免把 tunnel 校验升级为全局写入门禁（doctor 仅 warn） |
| CAP-07 | 状态/JSON 输出与表格展示 | `gookit/cliui/show/table`（`check`、`list` 已用）、`encoding/json` | 现有表格与 JSON 输出模式复用 | `tunnel list/show` 复用表格 helper；就绪 JSON 用 `encoding/json` | DIRECT_REUSE | 无 | 不新建 renderer；stdout/stderr 约定在命令层固定一次 |
| CAP-08 | 测试替身与拨号注入 | `remoteClientDialForTest`（`ssh.go:88-100`）、`setSSHClientFactoriesForTest`（`ssh_test.go:328`）、`withTempConfig`/`newTestApp`（`command_test.go:705/684`） | 测试配置与命令入口 helper 直接复用 | core 内新增 `newForwardDialerForTest` 伴生 seam | OWNER_EXTENSION | 现有 dial 替身不可直接注入 `StartLocalForward` 的会话缝 | 与 `remoteClientDialForTest` 同风格，测试专用变量不进入生产路径 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 外部 SSH 隧道库（如 sshtunnel 类库） | CAP-01 | 删除测试：`*remoteClient.Dial` + `forwardDialer` 薄适配已满足验收；引入库会复制认证/jump/known_hosts 语义，并与 `newSSHClient` 生命周期分裂，故拒绝 |
| 以子进程调用 `ssh -L` | CAP-01 | 删除测试：子进程方案无法复用 sshc 的 auth/known_hosts/日志与 `--json` 就绪语义，还需要平台相关信号与参数转义，故拒绝 |
| 后台 daemon/pid 管理库 | CAP-04 | 删除测试：v1 明确前台运行、无 stop/pid 语义（design 非目标），引入即扩大 lifecycle 与安全面，故拒绝 |

## 前置检查与 fail-closed 条件

### Workspace baseline

- Git root：`D:\work\inhere\my-tools-dev\inhere-tools\sshc`（独立仓库；父仓库 `my-tools-dev` 将其视为未跟踪目录，本计划不触碰父仓库）。分支 `main`，HEAD `716428c`，工作树 clean。
- 基线验证已完成：`go build ./...` OK；`go test ./... -count=1` 全绿（4 个包）。Go `1.25.10`。
- 变更基线：本轮只按第 5 节任务改动 expected paths；出现其他路径改动按 "错误路径即停止" 处理。

### 环境与依赖

- 无需网络与真实 SSH 主机即可完成 A1-A8（全部使用假 dialer/本地 echo 服务）。
- 统一验证命令：`go build ./...`、`go test ./... -count=1`、`gofmt -l internal cmd`（期望无输出）。CI 等价命令为 `go test -cover ./...`。

### fail-closed 条件

1. `go build ./...` 或 `go test ./...` 在任一任务后失败，且不能在 owner 文件内修正时停止。
2. 需要新增 Module、第三方依赖、改变 CLI/配置 schema、改变安全边界或数据语义时，停止并回到 design/plan 评审与人工计划 Gate（Semantic Amendment）。
3. 发现与 design 0.3 冲突（例如需要 `--allow-non-loopback`、需要 daemon）时停止，不在计划内自行解决。
4. 工作树出现非本计划来源的改动（Ownership Conflict）时停止，不用 staging 绕过。
5. 任一任务无法给出第 5 节的验证命令输出时不得标记完成。

## 波次与依赖

```text
W1 P0 配置与迁移   T1 (隧道模型/解析) -> T2 (持久化/doctor/export-import)
W2 P0 转发核心     T3 (StartLocalForward/生命周期)   [可与 T1/T2 并行，但需 T1 的 TunnelForward 字段名]
W3 P1 CLI          T4 (tunnel 命令组/输出/信号)      [依赖 T1,T2,T3]
W4 P1 引用与文档   T5 (auth rm/host rm 引用), T6 (README/TODO)  [T5 依赖 T1,T2；T6 依赖 T4]
W5 验收闭环        T7 (A1-A8 证据、A9 交接、完成 Gate)  [依赖 T1-T6]
```

建议顺序执行以减少冲突；T3 若与 T1/T2 并行，需先冻结 `TunnelForward{Local,Remote}` 与 `ForwardRule{LocalAddr,RemoteAddr}` 字段名。

## 任务

### T1 隧道配置模型、转发规则解析与保存级校验

- 文件: `internal/core/tunnel.go`（新增）、`internal/core/tunnel_test.go`（新增）
- 动作:
  1. 定义 `TunnelForward{Local, Remote string}` 与 `TunnelProfile{Name, Target, Address string; Port int; Jump, AuthRef string; Forwards []TunnelForward; Remark string}`，JSON tag 与 design 0.3 配置示例一致（`target`/`address`/`port`/`jump`/`auth_ref`/`forwards`/`remark`）。
  2. 实现 `ParseForwardRule(value string) (TunnelForward, error)`：`local[host:]port=remote[host:]port`；只写端口时补 `127.0.0.1`；IPv6 用 `net.SplitHostPort` 处理方括号形式；端口 1..65535，本地允许 `0`；拒绝 Unix socket、端口范围、UDP 表述与 OpenSSH 冒号三段式。
  3. 实现 `NormalizeTunnelProfile`（TrimSpace、规则规范化、名称安全字符校验）与 `ValidateTunnelProfile(cfg Config, p TunnelProfile) error`：`target`/`address` 互斥且必填其一；`address` 必须有 `auth_ref` 且 `auth_ref` 必须存在；`port` 范围；`jump` 必须是已登记 host；`forwards` 至少一项、本地端口不重复、本地 host 必须是 loopback。
  4. 实现 `FindTunnel`、`UpsertTunnel(profiles, p, force)`（重名且非 force 时报错）、`RemoveTunnel`、`TunnelsUsingAuth(profiles, name) []string`。
  5. 实现 `ResolveTunnelHost(cfg Config, p TunnelProfile) (Host, error)`：host 模式走 `ResolveEffectiveHostWithAuth(target, authRef)`，解析失败直接报错不降级；address 模式走 `ResolveEffectiveHostWithAuth(address, authRef)` 且要求 `auth_ref` 非空；最后应用 `HostOverrides{Port: p.Port}` 与 `Jump`。
  6. 测试 `tunnel_test.go`：A1 全表（含拒绝用例）；`target` 指向不存在 host、`address` 缺 `auth_ref`、非 loopback 本地绑定、重复本地端口、重名 add 非 force 等失败路径；`ResolveTunnelHost` 的 host/address 两模式与 port/jump 覆盖。
- 验证: `go test ./internal/core/ -run 'TestParseForwardRule|TestValidateTunnelProfile|TestResolveTunnelHost|TestUpsertTunnel' -count=1 -v`
- 完成标准: 上述测试通过；A1 用例全部覆盖 design 0.3 验收表 A1 列出的输入；非法输入返回可读错误且不写配置。
- 依赖: 无（需 design 0.3 决策 5/8/9）

### T2 tunnels 持久化、doctor warn 与 cfg export/import 合并

- 文件: `internal/core/store.go`、`internal/core/config_doctor.go`、`internal/core/config_export.go`（修改）、`internal/core/config_test.go`、`internal/core/config_export_test.go`（修改/新增）
- 动作:
  1. `Config` 增加 `Tunnels []TunnelProfile \`json:"tunnels"\``；`normalizeConfig` 置空切片并按 `NormalizeTunnelProfile` 规范化每项（含 `normalizeConfigForSave` 路径）。
  2. `config_doctor.go` 增加 `checkTunnels(config)`：tunnel 名称重复、`target`/`address` 缺失或互斥冲突、`auth_ref` 不存在、`jump` 不存在、forward 规则不可解析、`target` 不再解析到 host 等，全部使用 `DoctorWarn`；`CheckConfig` 只追加 warn，不产生 `DoctorError`。
  3. `config_export.go`：`ImportResult` 增加 `TunnelsAdded`/`TunnelsUpdated`；`mergeImportedConfig` 为 tunnels 增加与 host 相同语义的分支（默认冲突即报错、overwrite 覆盖）；`ImportReplace` 直接采用导入包（天然覆盖 tunnels）；`MaskConfig` 无需改动（无秘密字段），但需在测试中断言 tunnel 字段完整保留。
  4. 测试：doctor 对过期 tunnel（host 已删）只返回 warn；`cfg` 层往返 `EncryptConfigExport`/`DecryptConfigExport`（tunnels 保留）与 merge/overwrite/replace 的统计与冲突行为；旧配置缺 `tunnels` 时加载为空集合且保存后字段存在但不改变其他集合。
- 验证: `go test ./internal/core/ -run 'TestCheckConfig|TestTunnel|TestMergeImported|TestConfigExport' -count=1 -v`，并 `go test ./internal/server/ -count=1`（确认 serve 写入路径仍通过 `CheckConfig` 门禁）
- 完成标准: A7/A8 的 core 级证据通过；`CheckConfig` 不因 tunnel 问题返回 error 级；export/import 往返后 tunnels 与统计正确。
- 依赖: T1

### T3 转发核心：会话建立、监听、转发与生命周期

- 文件: `internal/core/forward.go`（新增）、`internal/core/forward_test.go`（新增）
- 动作:
  1. 定义 `ForwardRule{LocalAddr, RemoteAddr string}`、`ForwardOptions{ConnectTimeout, KeepaliveEvery, KeepaliveWait time.Duration; Logf func(string, ...any)}`（Keepalive 缺省 30s/10s）、`ForwardSession{Endpoints() []string; Wait() error; Close() error}`。
  2. 定义会话缝 `forwardDialer{Dial(network, addr string) (net.Conn, error); SendKeepalive() error; Wait() error; Close() error}`，实现 `remoteDialer{client *remoteClient}`（`Dial` 直转、`SendKeepalive` 发 `keepalive@openssh.com` 并要求回复、`Wait` 等待 SSH 连接结束、`Close` 复用 `closeAll`），并提供 `newForwardDialer(host Host) (forwardDialer, error)` 与测试伴生变量 `newForwardDialerForTest`。不修改 `RemoteClient` 接口。
  3. `StartLocalForward(host, rules, opts)`：先全部校验规则再建会话；逐条 `net.Listen`（本地 host 必须 loopback，端口 `0` 允许）；任一 listener 失败即回滚已建 listener 与会话；返回 session 携带 `Endpoints()` 实际地址。
  4. accept 循环：临时错误（`net.Error` 且 `Temporary()` 或等价判断）按 5ms 起步、上限 1s 的指数退避重试并在 `Logf` 记录；非临时错误结束会话。
  5. 每连接 `Dial("tcp", remote)` + 双向 copy：单方向 EOF 时对写端 `CloseWrite`（`*net.TCPConn` 可用时），两方向结束或出错后关闭整条连接；统计字节与时长只在 `Logf` 输出。
  6. 存活监视：keepalive 定时器（失败即视为失效）与会话 `Wait()` 观察 goroutine；失效时关闭 listeners、活动连接与会话，`Wait()` 返回带原因的非零错误。
  7. `Close` 幂等，顺序为 listeners → 活动连接 → 会话；`Wait` 阻塞直到会话结束且 goroutine 已回收。
  8. 测试：A2（本地 echo 服务 + 假 dialer 双向数据与半关闭）、A3（keepalive 失败/会话关闭触发 listeners 关闭且 `Wait()` 非零、无 goroutine 泄漏）、A4 的 core 级部分（`:0` 分配的实际端口出现在 `Endpoints()`）、A5（占用其中一个本地端口启动失败后其他 listener 与会话已释放）、A6 的 core 级部分（`Close` 后活动连接被断开、监听的端口可立即重新绑定）。
- 验证: `go test ./internal/core/ -run 'TestStartLocalForward|TestForwardLiveness|TestForwardRollback|TestForwardClose' -count=1 -v`
- 完成标准: A2/A3/A5 与 A4/A6 的 core 级断言通过；无 `RemoteClient` 接口改动；goroutine 泄漏以「关闭后可立即重新绑定同一端口 + `runtime.NumGoroutine` 回到测试前基线」断言（不新增依赖）。
- 依赖: 无（字段名需与 T1 冻结一致）

### T4 `tunnel/tun` CLI 命令组、目标解析、输出约定与信号退出

- 文件: `internal/command/tunnel.go`（新增）、`internal/command/tunnel_test.go`（新增）、`internal/command/util.go`（如需）、`internal/bootstrap/init.go`（修改）
- 动作:
  1. `NewTunnelCmd()` 注册 `add`、`list`、`show`、`rm`、`forward`（别名 `tun`），`Category` 与 `host/auth` 一致；选项按 design 0.3 表格：`--target`、`--address`、`--forward`（可重复）、`--auth`、`--port`、`--jump`、`--connect-timeout`、`--json`、`--verbose`、`--quiet`、`--force`、`--yes`。
  2. `add`：加载 `core.LoadConfig()`，构造 profile（`--address` 时强制 `--auth`），调用 `core.ValidateTunnelProfile` + `core.UpsertTunnel`，保存前运行 `core.CheckConfig` 且只把 error 级问题视为阻断，最后 `core.SaveConfig`；成功输出保存摘要。
  3. `forward`：支持 `name` 与临时模式（`--target` 或 `--address`）；命令行 `--forward` 出现即整体替换保存规则；解析目标得到 `core.Host`（复用 `resolveCommandHostWithAuth` 与 `ResolveTunnelHost` 的覆盖语义）；`command_proxy` 目标直接报错。
  4. 就绪输出：默认把 `tunnel ready local -> remote`（含 `:0` 实际端口）写 stderr；`--json` 时同一信息以单行 JSON 写 stdout（结构 `{"name","target","address","listeners":[{"local","remote"}]}`），stdout 无其他内容；`--quiet` 抑制非错误诊断；`--verbose` 打开连接级日志。
  5. 信号：`signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`，收到信号调用 `session.Close()` 并返回 0；SSH 失效或启动失败返回非零。
  6. `list`/`show`：表格输出（名称、模式、target/address、port、jump、forward 数、备注）与 `--json`；`show` 不打印任何凭据内容。`rm`：`--yes` 或交互确认后 `core.RemoveTunnel` + `SaveConfig`。
  7. 在 `internal/bootstrap/init.go` 注册命令。
  8. 测试（`tunnel_test.go`，复用 `withTempConfig`/`newTestApp`/`readTestStore`）：A4（`:0` 端口与 `--json` 单行 stdout、ready 走 stderr）、A7 CLI 级（`--address` 缺 `--auth`、`--target` 不存在、重名 add、`--force`、`rm --yes`、`list/show --json`）。
- 验证: `go test ./internal/command/ -run 'TestTunnel' -count=1 -v`；`go build ./...`
- 完成标准: 命令组可按 design 0.3 示例执行（本地假 dialer 路径下）；输出约定与信号退出有测试断言；无 stdout 污染。
- 依赖: T1、T2、T3

### T5 引用完整性：`auth rm` 拒绝、`host rm` 提示

- 文件: `internal/command/auth.go`、`internal/command/host.go`（修改）、`internal/command/command_test.go`（扩展或新增测试）
- 动作:
  1. `auth rm`：在现有 host 引用检查后追加 `core.TunnelsUsingAuth(config.Tunnels, name)`，非空时返回错误并列出隧道名。
  2. `host rm`：删除前检查 `config.Tunnels` 中以该 host 为 `target` 或 `jump` 的 profile；存在时输出提示（stderr）说明 forward 阶段会报 `host not found`，`--yes` 时继续删除，无 `--yes` 时在交互确认文本中体现该影响。
  3. 测试：删除被隧道引用的 auth profile 被拒并包含隧道名；删除被引用 host 时出现提示但仍可 `--yes` 删除。
- 验证: `go test ./internal/command/ -run 'TestAuthRemove|TestHostRemove' -count=1 -v`
- 完成标准: A7 的引用完整性断言通过；既有 `auth rm`/`host rm` 行为（无隧道引用时）不回归。
- 依赖: T1、T2

### T6 文档与进度更新

- 文件: `README.md`、`README.zh-CN.md`、`docs/TODO.md`（修改）
- 动作:
  1. README（英文）与 README.zh-CN 增加 `tunnel` 小节：host/address 示例、多规则、`--json`、输出约定、keepalive/失效行为、非目标说明。
  2. `docs/TODO.md` 增加 tunnel 进度条目（已实现范围与非目标）。
  3. 文档中的命令与 design 0.3 示例保持一致（含 `--address`/`--port` 写法），不写未实现选项。
- 验证: `go run ./cmd/sshc tunnel --help`（或 `go build -o tmp/sshc ./cmd/sshc && tmp/sshc tunnel --help`）人工核对帮助文本与 README 一致；`git diff --stat` 只含预期文件。
- 完成标准: README 示例与 `--help` 一致；TODO 反映实际完成范围。
- 依赖: T4

### T7 验收闭环与完成 Gate

- 文件: 无新增（只写进度记录；如需可更新 `docs/TODO.md` 勾选状态）
- 动作:
  1. 运行全量验证：`gofmt -l internal cmd`（无输出）、`go build ./...`、`go test ./... -count=1`。
  2. 逐条记录 A1-A8 的验证命令与输出（A1→T1、A2/A3/A5→T3、A4→T3/T4、A6→T3、A7→T1/T2/T4/T5、A8→T2）。
  3. 核对未引入新 Module/依赖（`git status`、`go.mod` 未变更）、未触碰 `web/` 与 `internal/server`。
  4. 生成 A9 人工验证交接说明（真实主机 + 本地客户端一次读写 + Ctrl-C 后端口释放），交给用户执行。
  5. 按任务粒度提交本地原子提交（每任务或每组 owner 一个提交），提交信息形如 `feat(sshc): add local port forwarding core`。
- 验证: 上述命令输出留档；`git log --oneline` 显示按任务提交的记录。
- 完成标准: A1-A8 证据齐全且全量测试绿；工作树无未提交残留（除 A9 交接说明）；完成 Gate 条件全部满足。
- 依赖: T1-T6

## 回滚与恢复

- 代码回滚：新增文件（`internal/core/{tunnel,forward}*.go`、`internal/command/tunnel*.go`）可整体删除；`store.go`/`config_doctor.go`/`config_export.go`/`auth.go`/`host.go`/`bootstrap/init.go` 的改动是附加分支，可用 `git revert <commit>` 精确回退。
- 数据回滚：`tunnels` 是新增集合，删除该字段即恢复旧配置；不涉及 host/auth 迁移，无不可逆数据动作。
- 运行时回滚：终止 `sshc tunnel forward` 进程即停止转发，不残留远端进程；本地端口随进程释放。
- 恢复点：每个任务一次提交，HEAD 前移即恢复点；出现 fail-closed 条件时回到上一任务提交再处置。

## 人工 Gate

1. 人工计划批准（本文档）：批准后才可进入实施；当前状态为待批准。
2. 当前执行请求：实施需要单独的当前请求（"开始实施 T1" 之类），计划批准本身不授权 mutation。
3. 外部动作 Gate：push、release、deploy、真实主机批量操作、外部消息均未请求；如需执行逐项单独批准。
4. A9 真实主机验证：由用户执行，不属于 Agent 授权范围；结果回填后计划方可标记完成定义全部达成。

## 可追溯性

| 需求/决策来源 | 任务 | 验证 |
|---|---|---|
| design 0.3 §范围（命令组、双目标、多规则、`:0`） | T1,T3,T4 | A1、A4；`go test ./internal/command/ -run TestTunnel` |
| design 决策 5（`local=remote`、无 `-L`） | T1 | A1 表驱动（含拒绝 OpenSSH 冒号式） |
| design 决策 7（`forwardDialer` 缝、不扩接口） | T3 | `go test ./internal/core/ -run TestStartLocalForward`；`git diff` 不含 `RemoteClient` 接口改动 |
| design 决策 8/9（双模式、重名 `--force`） | T1,T4 | A7；`TestValidateTunnelProfile`/`TestTunnel` |
| design 决策 10（tunnels 参与 export/import） | T2 | A8；`TestMergeImported`/`TestConfigExport` |
| design 决策 11（doctor 仅 warn） | T2 | A7；`TestCheckConfig`；`go test ./internal/server/` |
| design 决策 12（keepalive + `Wait` 观察、不重连） | T3,T4 | A3；`TestForwardLiveness` |
| design 验收 A2/A5/A6（转发、回滚、Ctrl-C） | T3,T4 | `TestStartLocalForward`、`TestForwardRollback`、`TestForwardClose`、`TestTunnel` |
| review F1（seam 事实） | T3 | 代码中 `RemoteClient` 接口未变、`remoteDialer` 存在 |
| review F2（空闲存活缺失） | T3 | A3 证据 |
| review F3（doctor 门禁耦合） | T2 | tunnel 检查只产出 warn；server 写入测试绿 |
| review F4（export/import 丢失） | T2 | A8 证据 |
| review F5（引用完整性） | T1,T5 | A7 引用断言 |
| review F6（profile 字段不足） | T1,T4 | A1/A7；`--address` + `--port` 用例 |
| review F7（验收缺失） | T3,T4,T7 | A1-A8 证据表 |
| design 非目标（无 daemon/SOCKS/Web/审计日志） | 全部 | `git diff --stat` 不含 `web/`、`internal/server`；`go.mod` 未变更 |

## 完成 Gate 与剩余工作

完成 Gate（全部满足才可声明本计划完成）：

1. T1-T7 全部完成，每个任务的验证命令在前一次提交后的 HEAD 上执行通过。
2. `gofmt -l internal cmd` 无输出；`go build ./...` 与 `go test ./... -count=1` 全绿。
3. A1-A8 证据逐条记录在进度中；A9 已交接给用户并注明未执行。
4. 无新增 Module/依赖；`web/`、`internal/server`、host/auth 既有行为无回归。
5. 每个任务有独立本地提交；未执行 push/发布/部署。

剩余工作（明确排除在本计划外，需新设计）：

- 后台 daemon 与 `forward list/stop` 跨进程管理。
- `ssh -R` 远程转发、SOCKS 动态代理。
- command_proxy 目标的 TCP stream 语义与转发支持。
- 转发 session 审计日志 schema 与 Web console 转发管理。
- keepalive 间隔/阈值的配置化（design 0.3 剩余待确认 1-3）。
