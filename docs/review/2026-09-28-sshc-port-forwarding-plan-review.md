<!-- template_id: review; template_version: 1.1.0 -->
# sshc 本地端口转发实施计划评审

## 评审目标与范围

- 目标: 判断计划 Draft 0.3 是否可独立执行（任务文件、动作、验证命令、完成标准是否闭合），并复核其与 design 0.3 的一致性、验收覆盖和测试可达性。
- 修订版本: 计划 0.3（design 输入 0.3）
- candidate: git root `D:\work\inhere\my-tools-dev\inhere-tools\sshc`；subject path `docs/plans/2026-09-28-sshc-port-forwarding-plan.md`；**该轮评审时 subject 尚未提交**（工作树中为未提交修订），因此本轮按合同记为 **advisory review**，不作候选身份锁定。评审时点 HEAD = `4e00830`，`go build ./...` 与 `go test ./... -count=1` 全绿。
- 评审者: 独立 headless 子代理会话 `session_retriever_1790613611856_849d8344c3230835`（只读指令：禁止编辑、禁止 git 写操作），由主 Agent（Jcode）汇总并复核其发现。评审者与 subject 作者不同会话，但同属 agent 执行环境；隔离是流程性的，不等价于人工独立评审。
- 日期: 2026-09-28
- 排除项: 未实施任何转发代码；未在真实 SSH 主机上验证；未评审 Web console、后台 daemon、`ssh -R`/SOCKS、command_proxy 转发（design 非目标）；未对 design 0.4 做逐段复评（仅核对 P5 相关 seam 段）。

## 输入与方法

- 评审者完整读取计划、design 0.3、design 评审报告，并直接核对 `internal/core/{ssh,store,config_doctor,config_export,config_mask,config_resolve,password_crypto}.go`、`internal/command/{cfg,host,auth}.go`、`internal/bootstrap/init.go`。
- 评审者执行的只读验证：`go build ./...`（OK）、`go test ./... -count=1`（4 包全绿）、`gofmt -l internal cmd`（复现 2 个既有偏差）、`go test -run <不存在的测试>`（复现 PASS + `no tests to run`）、`go run ./cmd/sshc cred list --help`（确认命令组别名路由）、计划符号名全仓 findstr 冲突扫描（0 命中）、`goph v1.5.2` 与 `x/crypto v0.54.0` 源码核对（`*ssh.Client` 内嵌 `Conn`，`SendRequest`/`Wait` 可达）。
- 主 Agent 独立复核的关键事实：`internal/command/hooks_test.go:5-44` 为命令层 seam 集合（`setRunRemoteForTest`/`setLoginRemoteForTest` 等），`internal/command/{run.go:14,login.go:17,serve.go:98}` 为生产侧 seam 与信号用法；`internal/core/config_test.go:404-460` 的 doctor 测试实际名为 `TestDoctorReports*`/`TestDoctorAccepts*`；`internal/core/password_crypto.go:30-77` 只遍历 AuthProfiles/Hosts；`ImportResult` 字段为 `Hosts/Groups/Auth*Added|Updated`（`config_export.go:57-65`）。
- 评审轴：Standards/Governance（模板合规、Gate 与授权边界、provenance）与 Spec/Executability（任务可执行性、测试可达性、design↔plan 一致性、验收覆盖）合并为一轮；按 SR1409 低暴露度封顶，未建 ledger。

## 验收标准

- 计划合同 `plan; template_version=1.2.0` 结构齐全（validator 通过），Capability Discovery 三表与 `host_or_non_offline_action` 声明有效。
- 每个任务的任务文件、动作、验证命令、完成标准可独立执行，且验证命令能真正失败（非空洞通过）。
- design 0.3 验收 A1-A9 每项都有具名 owner 与可运行检查。
- 计划与 design 的接口/配置描述一致，无未声明偏离。
- workspace baseline（Git root/branch/HEAD/status）与评审时点事实一致。

## 发现

### P1 [HIGH] 命令层没有可注入的转发会话缝，T4/A4 的 CLI 级验证不可执行

- 严重级别: HIGH（disposition: CORE_BLOCKING）
- 证据: 计划 T4.8 要求在 `internal/command` 内用 `withTempConfig`/`newTestApp` 跑出 A4（`local:0` 实际端口与 `--json` 就绪输出），但可注入的会话缝只存在于 `internal/core`（`newForwardDialerForTest`，未导出），命令层无法访问；命令层测试若真跑 `StartLocalForward` 就必须建立真实 SSH 连接。仓库既有模式是命令层自建变量：`internal/command/run.go:14 var runRemote = core.ExecuteRemote`、`internal/command/login.go:17 var loginRemote = core.LoginRemoteWithOptions`，并在 `internal/command/hooks_test.go:5-44` 提供 setter。
- 影响: A4 的 CLI 级证据无法产生，等于把"输出约定"验收挂空；实现者会临时发明注入方式，造成测试结构漂移。
- 建议: 在 T4 明确命令层 seam `var startLocalForward = core.StartLocalForward`（以及信号 seam `var notifyContext = signal.NotifyContext`），setter 落在 `hooks_test.go`；把 `hooks_test.go` 列入 T4 owner 文件。已在计划 0.4 落点。

### P2 [HIGH] A6 缺少可运行的验收归属，且 traceability 表与任务文本自相矛盾

- 严重级别: HIGH（disposition: CORE_CORRECTIVE）
- 证据: 计划 T4.8（0.3 版）只列出 A4/A7；T7 的 A 项映射写明"A6→T3"，而 traceability 表写"验收 A2/A5/A6 | T3,T4"，两处不一致；`Ctrl-C` 属于命令层信号处理，T3 的 core 测试无法覆盖信号路径。
- 影响: Ctrl-C 有序关闭这一 design 验收项没有具名测试，实施时最容易留下未验证行为。
- 建议: A6 归 T4，补 `TestTunnelForwardStopsOnInterrupt`（通过可取消的 `notifyContext` seam 驱动），T3 侧保留 `TestForwardClose` 覆盖资源释放；traceability 表按 owner 拆分。已在计划 0.4 落点。

### P3 [MEDIUM] T2 的验证选择器 `TestCheckConfig` 在仓库中不存在，会空洞通过

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: `go test ./internal/core/ -run TestCheckConfig -count=1` → `no tests to run` + PASS/exit 0（评审者实测复现）；实际 doctor 测试为 `TestDoctorReportsDuplicateHosts`/`TestDoctorReportsMissingAuthRef`/…/`TestDoctorAcceptsCommandProxyHost`（`internal/core/config_test.go:404-460`）。
- 影响: 若实现者只按选择器命名或不加新增用例，T2 的"doctor 只 warn"证据可空转通过。
- 建议: 选择器改为 `'TestDoctor|TestTunnel|TestMergeImported|TestConfigExport'`，并要求 `-v` 输出出现新用例名（配合计划 0.3 已写入的"验证证据规则"）。已在计划 0.4 落点。

### P4 [MEDIUM] `TunnelForward` 与 `ForwardRule` 的映射未定义

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: 计划 T1 定义持久化 `TunnelForward{Local,Remote}`，T3 定义运行时 `ForwardRule{LocalAddr,RemoteAddr}`，但没有任何条目规定两者的转换点；design 0.3 只给出 `ForwardRule`。
- 影响: 命令层、core、测试可能各自拼装规则，出现字段错位（例如把 `Remote` 当 `LocalAddr`）或重复校验。
- 建议: 在 T1 增加唯一转换点 `func (p TunnelProfile) ForwardRules() ([]ForwardRule, error)`，命令层不得自行拼装。已在计划 0.4 落点。

### P5 [MEDIUM] 计划的 `forwardDialer` 形态与 design 矛盾

- 严重级别: MEDIUM（disposition: CORE_CORRECTIVE）
- 证据: 计划 T3 定义为 4 方法（`Dial`/`SendKeepalive`/`Wait`/`Close`），design 0.3 架构段只写 `Dial`/`Close`，而 design 决策 12 又要求 keepalive 与连接关闭观察；两者不能同时成立。
- 影响: 实施者按 design 写会缺存活方法，按计划写又与 design 不符，形成未声明偏离。
- 建议: 以决策 12 为准，把 design 的缝形态补齐为 4 方法并标注原因。已在 **design 0.4** 落点（本计划评审的 P5 对账），计划 T3 同步注明"形态与 design 0.4 一致"。

### P6 [LOW] baseline provenance 过期

- 严重级别: LOW（disposition: CORE_CORRECTIVE）
- 证据: 计划 0.3 写"HEAD `716428c`；`git status` clean"，实际评审时 HEAD 为 `4e00830` 且计划文件本身处于未提交修订中；评审者因此无法锁定候选身份。
- 影响: 计划的可复现基线声明失真；也使本轮评审只能是 advisory。
- 建议: 记录实施时点的 HEAD/status 采集规则，并在计划提交后由首个任务复录。已在计划 0.4 落点（并保留 advisory 说明）。

### P7 [LOW] 环境级 verbosity 机制未与 gcli 对齐

- 严重级别: LOW（disposition: DEFERRED_ENHANCEMENT）
- 证据: gcli v3 提供包级 verbosity（`gcli.SetVerbose`/`IsDebugMode`，环境变量 `GCLI_VERBOSE`），仓库当前未使用（`internal/` 内 0 命中）；计划只写 `--verbose` 控制连接级日志。
- 影响: 存在两套 verbosity 语义的可能，长期容易分叉。
- 建议: 明确 `--verbose` 只管连接级 `Logf`，环境级沿用 gcli 机制。已在计划 0.4 落点。

### P8 [LOW] `normalizeConfig` 就地规范化与 secrets 路径的关系未说明

- 严重级别: LOW（disposition: DEFERRED_ENHANCEMENT）
- 证据: `normalizeConfig` 就地修改 `config.Hosts[i]`（`store.go:604-621`）；`encryptConfigPasswords`/`decryptConfigPasswords` 只遍历 AuthProfiles 与 Hosts（`password_crypto.go:30-77`）。
- 影响: 实现者可能误以为需要为 tunnels 增加加解密分支，或担心共享切片副作用。
- 建议: 显式写明 tunnels 无秘密字段、就地规范化与 Hosts 行为一致、无需改动 crypto 路径。已在计划 0.4 落点。

## 覆盖缺口与限制

- 本轮为 advisory：subject 在评审时未提交，未做 candidate 身份锁定；0.4 的改动范围（T1/T2/T3/T4、baseline、traceability）尚未经过独立复评。
- 评审者与作者同属 agent 环境，隔离为流程性；两轴为合并评审（SR1409 低暴露度），未建 ledger。
- 未验证项：`signal.NotifyContext` 注入式测试在 Windows 上的实际行为、`TestForwardLiveness` 的时序稳定性（首版需注意 flake 风险）、真实主机 A9、以及 design 0.4 中除 seam 段外的其他段落。
- 计划设计阶段的代码图为不可用状态（MCP 连接失败），符号冲突结论来自全仓文本扫描，不构成调用图证明。
- 本评审不授权实施、提交、推送或发布；发现只作为计划修订与批准的证据。

## 结论

BLOCKED（advisory）。存在 1 项 `CORE_BLOCKING`（P1：命令层缺少可注入会话缝，导致 T4/A4 的 CLI 级验证不可执行），另有 5 项 `CORE_CORRECTIVE`（P2 A6 归属与矛盾、P3 空洞选择器、P4 类型映射缺失、P5 design↔plan 缝形态矛盾、P6 baseline provenance）与 2 项 LOW。计划 0.3 不满足"任务可独立执行且验证可失败"的验收标准。

同时确认计划的可复用基础是可靠的：无计划符号冲突、goph/x-crypto 的 `SendRequest`/`Wait` 可达、`gofmt` 既有偏差声明属实、命令组别名路由可用、无新增依赖、`web/` 与 `internal/server` 未被纳入范围。

## 剩余风险与后续

- 0.4 修订已按 P1-P8 落点（命令层 seam、A6 归属、选择器、类型转换点、design 0.4 seam 对齐、baseline、两项 LOW 说明）；由于 subject 变更，0.4 仍需一轮 changed-scope 复评后才能进入人工计划批准。
- 若批准直接由用户吸收本轮发现，请在批准语句中显式说明"接受 advisory 结论并免除 changed-scope 复评"，以保证批准证据可追溯。
- 实施时优先做 T1/T3 的 seam 与存活测试（P1/P2/P5 的落点），这两处是唯一有 flake 与结构风险的区域。
- 本报告不授权实施；实施需要单独的当前执行请求。

## 处置记录与 changed-scope 确认（追加于 2026-09-28）

本节只追加证据与处置，不修改上文对 0.3 的发现、severity 与结论。

### 处置（修订计划 0.4 / 设计 0.4，提交 `1186160`）

| 发现 | 处置 | 落点 |
|---|---|---|
| P1 | 已修正 | T4 新增命令层 seam `startLocalForward`/`notifyContext` 与 `hooks_test.go` setter；`hooks_test.go` 列入 T4 owner |
| P2 | 部分修正（残留见下） | A6 归 T4 并具名 `TestTunnelForwardStopsOnInterrupt`；traceability 按 owner 拆分 |
| P3 | 已修正 | T2 选择器改为 `TestDoctor\|TestTunnel\|TestMergeImported\|TestConfigExport` 并注明既有 `TestDoctor*` 位置 |
| P4 | 已修正 | T1 新增唯一转换点 `func (p TunnelProfile) ForwardRules() ([]ForwardRule, error)` |
| P5 | 已修正 | design 0.4 架构段与决策 7 补齐为 `Dial`/`SendKeepalive`/`Wait`/`Close`；计划 T3 注明与 design 0.4 一致 |
| P6 | 部分修正（残留见下） | 输入章的 baseline 刷新为提交链并注明 advisory 条件；新增 T1 复录要求 |
| P7 | 已修正 | T4 注明 `--verbose` 只管连接级 `Logf`，环境级沿用 `gcli.IsDebugMode()`/`GCLI_VERBOSE` |
| P8 | 已修正 | T2 注明就地规范化与 `Hosts` 一致、tunnels 无秘密字段、无需改 crypto 路径 |

### changed-scope 确认评审（独立子代理 `session_pawprint_1790614540343_be3be8b550af6f8e`）

- 候选：HEAD `1186160`，工作树 clean（锁定候选）；基线复核 `go build ./...` OK、`go vet ./...` 无输出、`go test ./... -count=1` 4 包全绿、`gofmt -l internal cmd` 恰为计划声明的 2 个既有偏差。
- 逐项结论：P1 RESOLVED、P2 PARTIALLY_RESOLVED、P3 RESOLVED、P4 RESOLVED、P5 RESOLVED、P6 PARTIALLY_RESOLVED、P7 RESOLVED、P8 RESOLVED。
- 新发现 **N1 [LOW]**（disposition: CORE_CORRECTIVE）：design 结论段仍写"当前为 `Draft 0.3`"，与 0.4 状态行不一致；属 0.4 编辑遗漏的版本文本，不改变语义。
- 结论原文：**PASS** —— 原唯一 `CORE_BLOCKING`（P1）已真正解决，P3/P4/P5/P7/P8 充分修复，0.4 未扩范围、未新增依赖、`host_or_non_offline_action=NOT_APPLICABLE` 恰一次且成立；残余 P2（`plan:224` 仍写 A6→T3）与 P6（`plan:104` 仍写 HEAD 716428c/clean）仅为文本记账级不一致，不产生孤儿验收项、不影响可执行性。该结论不授权实施。

### 残留项闭环（计划 0.5 / 设计元数据修正）

- P2 残留：`docs/plans/...-plan.md` 的 T7 第 2 步改为 "A6→T4（`TestTunnelForwardStopsOnInterrupt`）+ T3（`TestForwardClose`）"（计划 0.5）。
- P6 残留：前置检查的 `### Workspace baseline` 刷新为提交链 `a78aa21→716428c→7e1f5ee→4e00830→f7a046d→1186160` 与 `go vet` 证据（计划 0.5）。
- N1：design 结论段版本文本改为 `Draft 0.4`（纯元数据纠正，按合同不递增版本、不新开 review round）。

以上三项均为该轮确认评审已指出的文本级修正，未改变范围、任务合同、验证方式或验收语义；计划 0.5 因此可视为"确认轮发现的闭环修订"，仍需人工计划批准后才可进入实施。
