<!-- template_id: review; template_version: 1.1.0 -->
# sshc 本地端口转发设计评审

## 评审目标与范围

- 目标: 判断 Draft 0.2 是否足以进入实施计划；重点核对命令契约、连接生命周期、安全边界、配置持久化与既有配置设施（doctor/export/import/引用）的协同，以及验收证据是否可执行。
- 修订版本: 0.2
- candidate: git root `D:\work\inhere\my-tools-dev\inhere-tools\sshc`；candidate commit `0308bf4bca9d26bc62cfb72b6b2cb379dd12fed0`；subject path `docs/design/2026-09-27-sshc-port-forwarding-design.md`，blob `f90b7c88e89ea7f773dcd1193b2d8316f822e8a0`；评审时工作树干净，subject 与 candidate 一致。
- 评审者: Jcode，对 subject 只读评审，未修改被评文档与任何源码。
- 日期: 2026-09-28
- 排除项: 未实现任何转发代码，未运行真实 SSH 端口转发或端到端 DB/Redis 往返；未验证 Web console、后台 daemon、`ssh -R`/SOCKS、command_proxy 转发（设计已列为非目标）；未执行第二轴独立评审轮，理由见"覆盖缺口与限制"。

## 输入与方法

- 完整读取 subject 289 行；确认其章节与 `design; template_version=1.1.1` 模板一致，并用发布版 validator 校验：
  `python .../inhere-doc-workflow/scripts/validate_document.py --kind design docs/design/2026-09-27-sshc-port-forwarding-design.md` → `{"ok": true, "errors": []}`。
- 逐条核对"已确认事实与规范"：`internal/core/ssh.go`、`internal/core/config_resolve.go`、`internal/core/store.go`、`internal/core/config_doctor.go`、`internal/core/config_export.go`、`internal/core/config_mask.go`、`internal/core/batch_run.go`、`internal/core/ssh_test.go`、`internal/command/{util,add,host,auth,group,cfg,check,run}.go`、`internal/server/{api_hosts,request,api_config}.go`、`internal/bootstrap/init.go`。
- 交叉核对仓库既有约定与历史：`docs/2026-07-08-sshc-usability-enhancements-design.md`（P2 tunnel/port-forward 提案）、`docs/TODO.md`、`README*`；提交 `0308bf4`（本文档）、`8e21d9a`（auth profiles for direct targets）、`f3aeb4e`/`26ee37e`（login 空闲 SSH 会话存活修复）。
- 治理输入：工作区 `standards.json` 绑定 release 0.22.1（`idev-std probe` → BOUND）；读取 design/review 合同、document-routing、gates、planning-core-rigorous，引用 SR1204、SR1403、SR1407、SR1408、SR1409、SR1211 判断评审力度与结论口径。
- 评审轴：Standards/Governance（模板合规、规则引用、Gate 与授权边界、事实一致性）与 Spec/Executability（命令契约、连接生命周期、配置/数据、安全、可执行验收）合并为一轮评审。暴露度按 SR1409 判定为低暴露度（本地 CLI、未投产转发路径、无生产数据、错误可由终止进程与删除新增文件廉价回滚、不产生部署/放量动作），因此按 SR1409 封顶为一轮合并轴评审，不建 review ledger、不做 candidate 回溯定位或两轴独立评审；该判定本身作为限制记录在下文。
- 代码图服务不可用（`codebase-memory-mcp` 连接即退出），本评审的调用关系证据全部来自直接读取源码与测试文件，与 subject 第 67 行声明的限制一致。

## 验收标准

- 设计合同 `design; template_version=1.1.1` 结构与内容齐全（validator 通过），修订记录、名词、范围/非目标、已确认事实、总体方案、架构、流程、安全/数据/运维/回滚、决策、待确认事项、人工计划 Gate 均可定位。
- 可执行性：`--forward` 语法可无歧义解析；连接生命周期覆盖建立、空闲存活、断开检测、有序关闭；监听安全边界与日志脱敏明确；配置持久化与既有 doctor 门禁、export/import、引用完整性一致；至少一条可执行验收路径可指向具体测试缝。
- 事实一致性：subject"已确认事实与规范"每条能在当前源码中直接验证。
- 授权边界：设计不授权实施、提交、发布或部署；人工计划 Gate 保留。

## 发现

### F1 [HIGH] 已确认事实错误：`RemoteClient` 接口并不提供 `Dial`

- 严重级别: HIGH（disposition: CORE_CORRECTIVE）
- 证据: subject 第 63 行称"`internal/core/ssh.go` 的 `RemoteClient` 已提供 `Dial(network, addr)`"，第 274 行决策 7 再次以该 seam 为前提。实际 `RemoteClient` 接口只有 `Run/RunContext/NewSession/NewSftp/Close`（`internal/core/ssh.go:63-69`），`Dial` 属于未导出的具体类型 `*remoteClient`（`internal/core/ssh.go:87-95`），`newSSHClient`/`newSSHClientWithOptions` 返回的是接口（`internal/core/ssh.go:732-749`）。
- 影响: 计划必须额外决定 seam 形式：扩接口会波及 `fakeRemoteClient` 等测试替身（`internal/core/command_proxy_test.go:125`），不扩接口则要新增内部构造/适配函数。这直接改变"新实现只负责 listener、channel 和 copy"的最小改动假设，也改变新增测试的替身写法。
- 建议: 在设计中把 seam 写成可实施形式，例如 `func newForwardDialer(host Host) (forwardDialer, error)`（包内直接复用 `*remoteClient.Dial`），或明确"把 `Dial` 提升为 `RemoteClient` 方法并同步更新测试替身"；同时修正第 63、274 行的措辞为"具体实现 `*remoteClient` 提供 `Dial`"。

### F2 [HIGH] 连接生命周期缺少空闲存活与断开检测机制

- 严重级别: HIGH（disposition: CORE_BLOCKING）
- 证据: subject 第 242 行承诺"SSH 主连接意外断开：关闭 listeners，返回非零"，但"关键流程"（211-242 行）未给出任何检测点；前台阻塞点是 listener `Accept` 与双向 copy，空闲时两者都不会观察到 SSH 连接死亡。`startSessionKeepalive`（`internal/core/ssh.go:240-267`）目前只接在 `loginWithClient`（`internal/core/ssh.go:186-190`），`newSSHClient` 路径没有任何 keepalive。仓库已有同一缺陷的历史：`f3aeb4e fix(login): keep idle ssh sessions alive`、`26ee37e fix(login): detect stalled ssh sessions`。此外 subject 第 6 行把"连接生命周期明确"列为停止条件。
- 影响: 按现设计实现，空闲隧道会被 NAT/防火墙静默断开而 session 仍显示"运行中"：listener 继续 accept，每条客户端连接才报 `direct-tcpip` 失败，用户看到的是逐条 DB/Redis 连接错误而不是隧道已失效。
- 建议: 在设计中补"存活与失效"小节：复用 `startSessionKeepalive` 或等价 `keepalive@openssh.com` 请求；用 `client.Client.Conn.Wait()`（或等价的连接关闭观察点）在后台 goroutine 触发 session 关闭；声明 keepalive 间隔/失败阈值来源（沿用 `defaultKeepaliveEvery`/`defaultKeepaliveWait` 还是新增配置），并把它写进"退出与错误"分支，作为该停止条件的验收项。

### F3 [MEDIUM] tunnels 校验与配置医生写入门禁的耦合未处理

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: `core.CheckConfig` 被当作保存门禁使用：`internal/command/add.go:156`、`internal/command/host.go:891`、`internal/command/group.go:124,166`、`internal/server/request.go:33-39`（serve 的所有写接口）、`internal/core/host_import.go:595`。subject 第 164 行只说"沿用现有原子写入和校验链"。
- 影响: 若把 tunnel 校验以 `DoctorError` 加入 `CheckConfig`，一条过期 tunnel（例如目标 host 已删除、auth_ref 悬空）会连带阻塞 `host add`、`group` 管理、Web console 写接口和 host import 等无关操作。
- 建议: 在设计中明确等级策略：新增 tunnel 检查使用 `DoctorWarn`，`tunnel add` 自身做硬校验（保存时拒绝非法规则）；并把"哪些检查进入 CheckConfig、哪些只在 tunnel 命令内"写成决策，避免把新集合变成全局写入门禁。

### F4 [MEDIUM] `cfg export/import` 未覆盖 tunnels，存在静默数据丢失

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: `MergeImportedConfig`/`mergeImportedConfig` 只处理 `LogsPath`、`Defaults`、`Groups`、`AuthProfiles`、`Hosts`（`internal/core/config_export.go:157-276`），`ImportResult` 无 tunnel 字段；`ImportReplace` 直接返回导入包（`internal/core/config_export.go:165-172`）。subject 的"数据与运维"只提到"配置新增 tunnels 集合，需要沿用现有配置版本兼容和原子保存机制"。
- 影响: `sshc cfg export` 生成的迁移包不含 tunnel profile，换机迁移后隧道配置丢失；`cfg import --replace` 会清空本机 tunnels 且没有任何提示。
- 建议: 二选一并写进设计：把 tunnels 纳入 export/import 合并与 `ImportResult` 统计；或显式声明 v1 不迁移 tunnels，并要求 import replace 对该集合打印删除警告。

### F5 [MEDIUM] 引用完整性规则缺失（auth rm / host rename / host rm）

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: `auth rm` 只用 `hostsUsingAuth(config.Hosts, name)` 判断引用（`internal/command/auth.go:191`，辅助函数同在 auth.go:320）；`host rm` 无任何引用检查（`internal/command/host.go:619-645`）；host rename 直接改名。subject 只写了"删除 tunnel 不删除关联 host/auth"（第 164 行），没有反向规则。
- 影响: 删除被 tunnel 引用的 auth profile 会留下悬空 `auth_ref`，下次 `tunnel forward` 报 `auth profile "x" not found`；更隐蔽的是 target 被删除/改名后，只要 profile 带 `auth_ref`，`ResolveEffectiveHostWithAuth` 会把该名称当未登记目标继续解析（`internal/core/config_resolve.go:77-113`），隧道静默指向同名/同 IP 的另一个目标而不报错。
- 建议: 在设计中补引用规则：`auth rm` 检查 tunnels 引用；`host rm`/`host rename` 对被 tunnel 引用的 host 至少提示；明确"target 无法解析但存在 auth_ref 时按未登记目标继续"是否为期望语义，若否应在 forward 前校验并报错。

### F6 [MEDIUM] tunnel profile 字段不足以支撑文档主打的"`--auth` + 未登记 IP"场景

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: subject 第 79-80 行示例 `tunnel add prod-redis --target 192.168.1.20 --auth dev-root` 被描述为可保存场景，但配置结构只有 `name/target/auth_ref/forwards/remark`（第 133-148 行）；未登记目标的有效端口只能来自 `defaults.port` 或默认 22（`internal/core/config_resolve.go:181-183`），`--jump` 仅本次覆盖、不可持久化（第 119 行）。
- 影响: 目标 SSH 端口非 22 或需要固定跳板时，"直接用 `--auth` 保存未登记 IP"的动机无法成立，用户仍被迫先建 host 条目；设计与示例自相矛盾。
- 建议: profile 增加可选 `port`、`jump`（语义与 `Host` 对齐，仍不保存凭据），或在设计中把未登记 target 的适用范围限定为"默认端口、无跳板"，并改写示例。

### F7 [MEDIUM] 缺少验收证据章节（SR1407）

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: subject 第 6 行把"验收证据明确"写入停止条件、第 66 行声称适用 SR1407，但正文章节没有验证/验收内容；仓库已存在可用测试缝：`remoteClientDialForTest`（`internal/core/ssh.go:88-100`）已在 `internal/core/ssh_test.go:162-194` 用于替换 dial 行为。
- 影响: 计划阶段缺少可执行验收锚点，容易退化为人工验证；端口 `0` 打印、部分规则失败清理、Ctrl-C 回收等关键行为没有验收归属。
- 建议: 增加"验收"小节，至少列出：以假 dialer + 本地 echo 服务验证 `local:0` 实际端口打印与单条连接往返；多规则中一条 listener 失败时的全量回滚；Ctrl-C 后 listener/活动连接/goroutine 回收；条件允许时对本地 sshd（或仓库内 fakeserver）做一条 DB/Redis 风格 TCP 端到端。

### F8 [LOW] `-L` 短选项与 OpenSSH 语法及仓库既有提案冲突

- 严重级别: LOW（disposition: DEFERRED_ENHANCEMENT）
- 证据: subject 第 117 行给 `--forward` 配 `-L` 短名，格式为 `local[host:]port=remote[host:]port`；同仓库 `docs/2026-07-08-sshc-usability-enhancements-design.md:660-665` 对同一功能给出 OpenSSH 风格 `-L 3307:127.0.0.1:3306`、`tunnel start/stop` 与 `host` 字段名，本文档改为 `target` 字段并去掉 stop/daemon，但未声明取代关系。
- 影响: 用户按 OpenSSH 习惯输入会解析失败；两份设计对同一命令面给出不同契约，实施与后续文档容易歧义。
- 建议: 或接受 OpenSSH 冒号语法作为 `-L` 兼容形式，或去掉 `-L` 别名只保留 `--forward`；并在设计中写明对 2026-07-08 提案的取代点（`host`→`target`、取消 `stop`/后台语义）。

### F9 [LOW] `--ready-timeout` 语义空转

- 严重级别: LOW（disposition: DEFERRED_ENHANCEMENT）
- 证据: subject 第 121 行把 `--ready-timeout` 定义为"等待本地 listener 建立"，但 listener 由同步 `net.Listen` 建立（第 174 行架构），启动即成功或立即失败。
- 影响: 选项没有实际作用，给用户与实现留下歧义。
- 建议: 删除该选项，或重定义为"等待 SSH channel 就绪/首次连通校验"的超时。

### F10 [LOW] accept 循环健壮性与半关闭语义未定义

- 严重级别: LOW（disposition: DEFERRED_ENHANCEMENT）
- 证据: subject 第 206 行规定"任一方向结束都关闭该连接的另一端"，未提临时 accept 错误处理。
- 影响: 临时错误（如 EMFILE）可能导致 accept 忙循环；单向 EOF 立即关闭双向连接对半关闭协议不友好。
- 建议: 设计中说明临时错误的退避/重试策略，并明确是否对单方向 EOF 使用 `CloseWrite` 半关闭。

### F11 [LOW] 日志边界表述自相矛盾

- 严重级别: LOW（disposition: DEFERRED_ENHANCEMENT）
- 证据: subject 第 250 行同时写"不把……完整 remote endpoint……写入日志"和"日志只记录 target、local endpoint、remote endpoint 的必要元数据"；第 239 行又要求单条连接失败"记录目标和错误"。
- 影响: 同一份设计给出互相冲突的日志规则，实现时无法判定 service 地址是否属于敏感信息。
- 建议: 明确"remote endpoint 是否算敏感"的判定，并统一为一条可执行规则（例如：默认记录 local/remote endpoint，`--quiet` 或配置可关闭）。

## 覆盖缺口与限制

- 评审强度：按 SR1409 判定为低暴露度，采用一轮合并轴评审，未建立 review ledger，未做 candidate 回溯或两轴独立评审；若治理认为本候选触及"安全边界"而必须高暴露处理，则还需一轮独立轴评审后才能消费本结论。
- 评审者隔离：subject 作者标注为 Codex（agent），本次评审由另一会话的 agent 只读完成，隔离是流程性的，不等价于人工独立评审。
- 视角限制：未编译或运行任何新代码，全部结论来自源码、测试、文档与 Git 历史的静态读取；未验证真实 SSH 服务器上的 `direct-tcpip` 行为、Windows 控制事件、以及 `ssh -R`/SOCKS/command_proxy 等非目标项。
- 代码图服务不可用（MCP 连接失败），调用关系与覆盖状态未作为证据，与 subject 第 67 行声明一致；因此"未发现问题"在调用图层面不构成穷尽性证明。
- 未读取 `web/` 前端产物与发布流水线细节；设计已把 Web console 暴露列为非目标，故不影响本轮结论。
- 本评审不授权实施、计划编写、提交、推送或部署；发现只作为决策证据。

## 结论

BLOCKED。存在 1 项开放 `CORE_BLOCKING`（F2：连接生命周期缺少空闲存活与断开检测机制），另有 6 项 `CORE_CORRECTIVE`（F1 事实错误、F3 doctor 门禁耦合、F4 export/import 数据丢失、F5 引用完整性、F6 profile 字段不足、F7 缺少验收证据）需要在同一轮修订中一并处理，设计才具备进入实施计划的条件。其余 4 项为 `DEFERRED_ENHANCEMENT`，可记录不阻断。

命令契约、安全边界（loopback 默认、拒绝 wildcard、日志脱敏原则）与"复用既有 host/auth/jump 解析链"的总体方向经源码核对成立，因此本结论是"补全后即可进入计划"，而非方向性否决。

## 剩余风险与后续

- 下一修订建议升为 0.3（语义变化）：补 F1 seam 形式、F2 存活/失效小节、F3 校验等级决策、F4/F5 引用与迁移规则、F6 字段决策、F7 验收小节；同时对 subject"待确认事项"6 项给出结论（至少固定 `tunnel add` 覆盖策略、`tunnel forward --json`、信号退出边界）。
- 修订后只需一轮针对性复评（changed/dependent scope），无需重建全部论证。
- 若最终选择"tunnels 不参与 export/import、未登记 target 仅限默认端口"等收缩方案，必须在设计中显式写明，避免实施阶段把缺失当作实现发现处理。
- 可追溯性风险：现有"已确认事实"章节未标注可溯源位置（无文件与行号），建议修订时补行号级证据，便于下轮评审与计划复用。

## 处置记录（追加于 2026-09-28，针对修订版 0.3）

本节只记录处置，不修改上文的发现、severity 与结论；上文结论仍指向 candidate 0.2 的 `0308bf4`。用户于 2026-09-28 直接要求"修订"，据此完成 subject 的 Draft 0.3 修订：

| 发现 | disposition 结果 | 0.3 落点 |
|---|---|---|
| F1 | 已修正 | "已确认事实"改为 `RemoteClient` 接口不含 `Dial`（附 `ssh.go:63-95`、`732-749`）；决策 7 改为包内 `forwardDialer` 缝，不扩接口 |
| F2 | 已修正（阻断项消除） | 新增"存活与失效"小节（keepalive + `Wait()` 观察）、决策 12、启动流程第 5 步、退出与错误分支 |
| F3 | 已修正 | 决策 11 与"数据与运维"：`tunnel add` 硬校验、`cfg doctor` 仅 warn |
| F4 | 已修正 | 决策 10 与配置策略：tunnels 纳入 export/import 与 `ImportResult` 统计 |
| F5 | 已修正 | `target`/`address` 双模式加保存时解析硬校验（消除静默漂移）；`auth rm` 拒绝被引用 profile、`host rm` 提示 |
| F6 | 已修正 | profile 新增可选 `port`/`jump`，新增 `--address`/`--port` 选项，示例改写并补充"取代 2026-07-08 P2 提案"说明 |
| F7 | 已修正 | 新增"验收证据与验证计划"章节 A1-A9，复用 `remoteClientDialForTest` 测试缝 |
| F8 | 已采纳 | 决策 5（不提供 `-L` 短名）与背景章的取代说明 |
| F9 | 已采纳 | 删除 `--ready-timeout`；新增 `--json`/`--verbose`/`--quiet`/`--force`/`--yes` 选项行 |
| F10 | 已采纳 | 实现约束 4（accept 退避）与 5（半关闭），并有 A5 验收 |
| F11 | 已采纳 | 安全章统一日志边界，配套固定输出约定（ready 走 stderr，`--json` 走 stdout） |

结构化校验：`validate_document.py --kind design docs/design/2026-09-27-sshc-port-forwarding-design.md` 在 0.3 上返回 `{"ok": true, "errors": []}`。

结论口径：0.3 的 changed-scope 复评尚未执行，因此本报告不为 0.3 给出 PASS；0.3 仍需一轮针对性复评（changed/dependent scope）后才能进入实施计划。

## changed-scope 复评与关闭确认（追加于 2026-09-28）

本节只追加证据与处置，不修改上文对 0.2 的发现、severity 与结论。承接上文承诺的"一轮针对性复评"。

### 复评轮（独立子代理 `session_piglet_1790615176421_1998b6dc7c087b10`，候选 `19597c1`，工作树 clean）

- 逐项确认：F1 CONFIRMED；F2（keepalive + `Wait` 观察）CONFIRMED；F3（doctor warn 分级）CONFIRMED；F4（tunnels 进 export/import）CONFIRMED；F5（引用完整性）PARTIALLY_CONFIRMED；F6（target/address 与持久化字段）PARTIALLY_CONFIRMED；F7（A1-A9 可失败且各有 owner）CONFIRMED；schema 与缺失字段兼容 CONFIRMED；回归项（`--ready-timeout`/`-L` 已彻底移除）CONFIRMED。
- 新发现与结论：
  - **D-N1 [MEDIUM] CORE_CORRECTIVE**：0.3 第 207/353 行承诺"host 删除或改名后 forward 报错"，但 `internal/core/config_resolve.go:87-91` 在 `auth_ref` 非空且未命中 host 时回落为未登记目标，且 `Store.ResolveHost`（`store.go:135-153`）有模糊匹配回退；承诺缺机制与验收锚点。
  - **D-N2 [LOW] DEFERRED_ENHANCEMENT**：0.4 补入 `SendKeepalive`/`Wait` 后，名词表 "forward dialer" 未同步。
  - **D-N3 [LOW] DEFERRED_ENHANCEMENT**：`--forward` 解析段（157 行）与 A1 仍以 `192.168.1.10:15432=…`（非 loopback 本地侧）为可解析示例，与安全章的"拒绝非 loopback 绑定"并存。
  - 结论原文：**BLOCKED** —— F5 的"host 消失后不静默漂移"缺机制与验收锚点（D-N1），需一次小修订并同步计划 T1.5。

### 处置（设计 0.5/0.6、计划 0.6，提交 `088865c`）

| 发现 | 处置 | 落点 |
|---|---|---|
| D-N1 | 已修正 | 设计 0.5：`target` 模式改为"保存时解析并固化规范 host 名；forward 只用精确匹配（`Store.Find` 语义）；禁止模糊回退与 `ResolveEffectiveHostWithAuth` 未登记降级"；A7 增加"重命名/删除后精确匹配报错且不落到未登记目标"验收；计划 0.6 的 T1.5/T1.6/T1.8 同步 |
| D-N2 | 已修正 | 设计 0.5 名词表 "forward dialer" 补入 keepalive 与连接关闭观察 |
| D-N3 | 已修正 | 设计 0.6：解析段与 A1 的示例本地侧改为 loopback，并补非 loopback 非 wildcard 拒绝用例 |

### 关闭确认（同一独立子代理，候选 `088865c`）

- D-N1 **CLOSED**（设计 209/333/355 行：保存固化规范名、forward 仅精确匹配、禁模糊回退与未登记降级，A7 有验收锚点）；D-N2 **CLOSED**（45 行含 keepalive/关闭观察）；D-N3 **CLOSED**（159/327 行本地侧改 loopback 并补拒绝用例）；计划 T1.5/T1.6/T1.8 与规则一致。
- 结论原文：**PASS**（唯一残留为计划中若干 "design 0.5" 版本引用滞后，纯 provenance）。

后续 provenance 修正：计划中指向当前设计的引用统一为 design 0.6（设计输入、T1.2/T1 完成标准/依赖、T4 完成标准与选项表引用、可追溯性行），修订记录 0.6 行改为 "design 0.5/0.6"；该修正为元数据级别，按合同不递增计划版本。

最终口径：设计 Draft 0.6 与计划 Draft 0.6 的 changed/dependent scope 已完成复评并关闭，两侧均无开放 `CORE_BLOCKING`/`CORE_CORRECTIVE`；等待人工计划批准。本报告与确认均不授权实施。
