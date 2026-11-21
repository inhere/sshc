<!-- template_id: plan; template_version: 1.3.1 -->
# sshc scan 局域网 SSH 目标发现实施计划

> 状态：Draft 0.1 / 待人工计划批准
>
> thinking_mode=RIGOROUS；core_objective=按 `docs/design/2026-11-17-sshc-scan-design.md` Draft 0.1 实现 `sshc scan`（别名 `discover`）：IPv4 子网枚举 + IPv6 邻居缓存发现、SSH banner 确认、表格/JSON 输出、可选入库；allowed_scope=`internal/command`、`internal/core`、`internal/bootstrap` 注册、`README*.md` 帮助段落与上述代码的测试；non_goals=全端口扫描、IPV6 主动组播探测、公网扫描、凭据写入、发布/推送动作；expansion_policy=DEFER_OR_REQUEST；review budget=本机工具档一轮合并轴评审；停止条件=每个任务的 owner 文件、验证命令与可观察完成标准明确且可追溯到设计决策。暴露度：本机工具档；改动仅本机 CLI 与本地配置追加，可删除新增 host 回滚，无 Secret 处理与远程写入。

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-17 | Suga | 初稿：三波实施（core 发现与探测 / CLI 与入库 / 真实本机验证与文档） |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 目标与完成定义

目标：实现设计 Draft 0.1 的全部范围内行为。完成定义（全部满足才算完成）：

1. `sshc scan` 能在未配置任何 host 的情况下，列出本机 IPv4 网段与 IPv6 邻居缓存中真正提供 SSH 服务的目标，字段为 `Addr, Zone, Port, Banner, Latency, Source`，`--json` 输出等价的数组。
2. `sshc scan --save --group <g>` 只追加不含凭据的 host 条目，重名追加序号、重复条目跳过，随后 `sshc run <生成的名称> -- hostname` 能进入既有连接流程（真实可达性取决于环境）。
3. 非私网 `--cidr` 未加 `--force`、`--max-hosts` 溢出、`--parallel`/`--timeout` 越界、未声明位置参数均以退出码 2 拒绝且不执行探测。
4. IPv6 邻居缓存不可用（平台不支持或命令缺失）时 IPv4 结果照常输出，摘要中报告来源不可用，命令仍然退出 0。
5. `go build ./...` 与 `go test ./...` 在仓库根通过；`go vet ./...` 无新增报告。

## 范围、排除项与授权

范围：新增 `internal/command/scan.go`、`internal/core/scan.go`、`internal/core/scan_neighbors.go`（含 `_windows.go`/`_linux.go`/`_other.go` 平台分支）、对应 `_test.go`；在 `internal/bootstrap/init.go` 注册命令；`README.md`、`README.zh-CN.md` 增加用法段落；`docs/TODO.md` 仅在需要时补充待办勾选。

排除项：全端口/服务指纹扫描；IPv6 `ff02::1` 主动探测与 Windows `arp -a` 加速（二期）；公网与跨网段扫描；自动写入用户名/密码/密钥/`--auth`；任何 push、tag、release、安装或部署动作。

授权边界：本计划只授权实施与本地验证所需的代码改动和本地提交；推送、打 tag、发布与真实网络扫描的运行需在实施后的外部动作 Gate 单独批准（见“人工 Gate”）。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-17-sshc-scan-design.md`（Draft 0.1，Git candidate `b8c9bb7`）。
- 设计批准证据（用户原话，2026-11-17）：`批准 Draft 0.1，进入实施计划`。
- 设计阶段已由用户确认的决策：命令名 `scan`、别名 `discover`；IPv6 一期只读邻居缓存；`--save` 默认关闭；保留 `--cidr`。
- 适用规范：SR1403、SR1405、SR1410、SR1204、SR2003、GR104、GR110、SR1301、SR1402.1（真实边界验证）。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | IPv4 网段枚举与地址上限保护 | `internal/core/endpoint.go`、`internal/core/check.go`、stdlib `net`/`net/netip` | `net.Interfaces` + `netip.Prefix` 直接构造 | 在 `internal/core` 内新增纯函数，不新建包 | DIRECT_REUSE | none | 无重复实现；随 sshc 版本发布 |
| CAP-02 | IPv6 邻居缓存读取 | `internal/core/ssh_knownhosts.go`、`internal/core/store.go`、`golang.org/x/net` 未引入 | 无现有能力 | 在 `internal/core` 内按平台文件薄封装系统命令输出解析 | OWNER_EXTENSION | 系统命令输出是唯一无特权来源 | 单一 owner `core/scan_neighbors*.go`；平台文件随 sshc 发布 |
| CAP-03 | 并发 TCP 探测与 SSH banner 判定 | `internal/core/check.go:43-45,95`、`internal/command/check.go:31-120` | `checkTCPDial` 与 `knownHostAddress` 直接复用 | `internal/core/scan_probe.go` 复用现有原语 | DIRECT_REUSE | none | 不新增连接层 |
| CAP-04 | 表格与 JSON 输出 | `internal/command/check.go:1-14,120-190`、`cliui/show/table` | `table` 与 `encoding/json` 直接复用 | 沿用 check 的输出写法 | DIRECT_REUSE | none | 无 |
| CAP-05 | 发现结果入库 | `internal/core/store.go:380-400,571-575`、`validateHost`、`NormalizeHostFields` | `LoadStore`/`SaveStore` 直接复用 | `--save` 调用一次 `SaveStore` | DIRECT_REUSE | none | 不改变存储 schema |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 独立扫描器二进制/子包 | CAP-01 | 网段枚举可在 internal/core 内完成；新增包不消除任何已知 gap，删除后无功能损失 |
| 独立扫描器二进制/子包 | CAP-03 | 探测直接复用 checkTCPDial；独立包无额外收益，删除后无功能损失 |
| 第三方局域网扫描库 | CAP-02 | 仅需解析系统邻居缓存，引入依赖不减少代码且增加供应链与 go.mod 生命周期 |

## 前置检查与 fail-closed 条件

- workspace baseline：Git root `D:/work/inhere/my-tools-dev/inhere-tools/sshc`，branch `main`，HEAD `b8c9bb7e9689b7ab09fba901a4408b65989e9bf4`，`git status --short --branch` 为 `## main...origin/main [ahead 1]`（ahead 1 = 设计文档提交），无 dirty/untracked 文件。
- expected paths/symbols：新增 `internal/command/scan.go`（`NewScanCmd`）、`internal/core/scan.go`（`ScanTarget`、`BuildScanTargets`）、`internal/core/scan_neighbors.go`（`ReadIPv6Neighbors`）、`internal/core/scan_probe.go`（`ProbeSSH`）、对应 `_test.go`；修改 `internal/bootstrap/init.go`（仅注册命令）、`README.md`、`README.zh-CN.md`。
- 环境假设：Go 1.25（`go.mod` 已声明）；Windows 走 `netsh interface ipv6 show neighbors`，Linux 走 `ip -6 neigh show`；两者都缺失时按设计降级为“来源不可用”，不失败。
- fail-closed：`git status` 出现本计划 owner 之外的 dirty/untracked 文件时停止（Ownership Conflict）；`go build ./...` 或既有测试在实施前已失败时先停下报告，不把既有失败计入本计划。

## 波次与依赖

- Wave 1：core 发现与探测（T01、T02、T03），无外部依赖。
- Wave 2：CLI 命令、输出、入库（T04、T05），依赖 Wave 1 的 `BuildScanTargets`/`ProbeSSH`/`ReadIPv6Neighbors`。
- Wave 3：文档与真实边界验证、全量回归（T06、T07），依赖 Wave 2。

## 任务

### T01 IPv4 目标枚举与上限保护

- 文件: `internal/core/scan.go`、`internal/core/scan_test.go`
- 动作: 定义 `ScanTarget{Address, Family, Zone, Interface, Source}` 与 `ScanOptions{MaxHosts int}`；实现 `BuildScanTargets(cidrs []string, interfaces []string, opts ScanOptions) ([]ScanTarget, []ScanSourceStatus, error)`：无 `cidr` 时按本机接口 IPv4 地址+掩码枚举同网段主机（跳过网络号、广播号、本机地址）；有 `cidr` 时按网段枚举；超过 `MaxHosts`（默认 4096）返回可识别的错误；实现 `IsScannableTarget(target ScanTarget) bool` 与私网守卫（RFC1918、169.254/16、`fe80::/10`、`fc00::/7`）。
- 验证: 在仓库根运行 `go test ./internal/core -run 'TestBuildScanTargets|TestIsScannableTarget' -count=1`
- 完成标准: `/30` 网段枚举出 2 个可用地址；`/8` 触发上限错误且错误文案包含 `--max-hosts`；公网网段被守卫拒绝、加 `forced=true` 后通过；上述用例覆盖接口枚举、`cidr` 枚举和守卫三类分支。
- 依赖: 无

### T02 IPv6 邻居缓存读取（平台分支）

- 文件: `internal/core/scan_neighbors.go`、`internal/core/scan_neighbors_windows.go`、`internal/core/scan_neighbors_linux.go`、`internal/core/scan_neighbors_other.go`、`internal/core/scan_neighbors_test.go`
- 动作: `ReadIPv6Neighbors() ([]ScanTarget, ScanSourceStatus)`；平台文件只提供“执行命令并返回原始文本”的最小函数，解析逻辑共用 `parseNeighborTable(text string) []ScanTarget`；Windows 解析 `netsh interface ipv6 show neighbors` 的行（地址 + 接口索引），Linux 解析 `ip -6 neigh show` 的行（地址 + `dev <if>`）；跳过 `incomplete`/`failed`、组播、`::1` 与非可扫描地址；zone 保留为接口索引/名称。
- 验证: 在仓库根运行 `go test ./internal/core -run 'TestParseNeighborTable' -count=1`
- 完成标准: 用 Windows 与 Linux 两段真实样例输出各一条用例，均解析出预期地址与 zone；`incomplete` 行与组播地址被跳过；空输入返回空切片且 `ScanSourceStatus.Available=false` 时不报错。
- 依赖: T01（复用 `ScanTarget` 与守卫）

### T03 并发探测与 SSH banner 判定

- 文件: `internal/core/scan_probe.go`、`internal/core/scan_probe_test.go`
- 动作: `ProbeSSH(ctx context.Context, targets []ScanTarget, port int, timeout time.Duration, parallel int, dial func(network, addr string, timeout time.Duration) (net.Conn, error)) ([]ScanResult, error)`；复用 `checkTCPDial` 作为默认 dial；连接成功后在 deadline 内读首行判定 `SSH-` 前缀，记录 `Banner`、`LatencyMS`；`parallel` 用固定 worker 池，`ctx` 取消时返回已收集结果。
- 验证: 在仓库根运行 `go test ./internal/core -run 'TestProbeSSH' -count=1`
- 完成标准: 本地 `net.Listen` 伪服务器返回 `SSH-2.0-Test` 时结果 `Banner` 命中；返回非 SSH 文本时不计入结果；端口未监听被忽略；`ctx` 取消后函数在 1s 内返回且不 panic。
- 依赖: T01

### T04 scan 命令、参数校验与输出

- 文件: `internal/command/scan.go`、`internal/command/scan_test.go`、`internal/bootstrap/init.go`
- 动作: `NewScanCmd()` 绑定 `--cidr`（可重复）、`--interface`、`--port`（可重复，默认 22）、`--timeout`（默认 400ms，上限 10s）、`--parallel`（默认 128，上限 1024）、`--max-hosts`（默认 4096）、`--json`、`--force`、`--save`、`--group`（默认 `lan`）；`Config` 中 `c.SetValidateNum(true)` 并 `c.AddArg` 不声明位置参数；handler 先校验参数与守卫再触发探测；表格列固定 `Addr, Zone, Port, Banner, Latency, Source`，`--json` 输出结构化数组；来源不可用时在表格后输出一行提示。注册命令到 `bootstrap.NewApp()`（仅一行 `command.NewScanCmd()`）。
- 验证: 在仓库根运行 `go test ./internal/command -run 'TestScanCmd' -count=1`
- 完成标准: `app.Run([]string{"scan","--parallel","0"})`、`--timeout 20s`、`--max-hosts 0`、`scan extra` 均返回退出码 2 且 handler 未执行探测（用替换的探测函数计数断言）；`scan --help` 退出码 0 且包含 `--cidr`、`--save`、`--force`；`--json` 输出可被 `json.Unmarshal` 解析。
- 依赖: T01、T02、T03

### T05 `--save` 入库

- 文件: `internal/command/scan.go`、`internal/command/scan_test.go`
- 动作: 实现 `saveScanHosts(results []core.ScanResult, group string) (saved, skipped int, err error)`：名称按设计生成（IPv4 `lan-192-168-1-20`，IPv6 `lan-<后 4 hextet>`），重名追加 `-2`/`-3`，同名同 IP+端口跳过；`LoadStore` → 追加 → `validateHost`+`NormalizeHostFields` → 单次 `SaveStore`；不写入用户、密码、密钥、`--auth`；接线到 handler 末尾，输出 `saved: N, skipped: M`。
- 验证: 在仓库根运行 `go test ./internal/command -run 'TestScanSave' -count=1`
- 完成标准: 用临时 home（`core.SetUserHomeDirForTest`）写入两条结果后重新 `LoadStore` 能读回名称/IP/端口/分组；再次保存同样结果 `skipped` 计数正确且不产生重复；生成名冲突时自动加序号；保存后的 host 字段中 `Password`、`KeyPath`、`AuthRef` 均为空。
- 依赖: T04

### T06 README 用法与设计文档批准留痕

- 文件: `README.md`、`README.zh-CN.md`、`docs/design/2026-11-17-sshc-scan-design.md`
- 动作: 在两个 README 的命令列表/示例处增加 `sshc scan` 段落（枚举来源、私网守卫、`--save`、二期限制）；在设计文档状态行补充批准证据（纯 provenance 变更，不升版本）。
- 验证: 在仓库根运行 `idevstd script validate_document --kind design docs/design/2026-11-17-sshc-scan-design.md` 与 `grep -c "sshc scan" README.md README.zh-CN.md`
- 完成标准: 设计文档校验 `ok: true`；两份 README 各含至少一处 `sshc scan` 可执行示例，且示例中的 flag 与 T04 实现的 flag 集合一致。
- 依赖: T04

### T07 真实边界验证与全量回归

- 文件: 无新增（只运行验证；如发现缺陷按 implementation-discovery 分类处理）
- 动作: 在仓库根运行 `go build ./...`、`go vet ./...`、`go test ./...`；构建二进制到 `tmp/` 并在真实 Windows 终端执行一次 `sshc scan --timeout 300ms --parallel 64` 与一次 `sshc scan --cidr 127.0.0.1/32 --port 22 --json`；记录实际输出片段与来源可用性。
- 验证: 上述命令的退出码与输出；无法在真实链路验证的部分写 `UNVERIFIED_REAL_ENV: <面>（复现：<命令>）`。
- 完成标准: 全量测试通过；真实终端输出中 `Addr/Zone/Port/Banner/Latency/Source` 列齐全且无 `%!` 等格式化垃圾；`--json` 输出可解析；若局域网无 SSH 主机，如实记录 0 条发现而不是报错。
- 依赖: T05、T06

## 回滚与恢复

- 回滚：删除 `internal/command/scan.go`、`internal/core/scan*.go` 与其测试，移除 `internal/bootstrap/init.go` 的一行注册与 README 段落即可完全回退；`--save` 写入的条目用现有 `sshc host remove <name>` 清理。
- 恢复点：每个任务一个原子提交（T01→T07 顺序），任一任务失败可从上一提交继续，不 reset 他人改动。
- dirty 保护：每个任务提交前 `git status --short` 只包含该任务 owner 文件；出现其他改动即停。

## 人工 Gate

1. **计划批准**：本计划 Draft 0.1 需要你明确批准后才能开始实施（`批准实施计划 0.1, 开始实施`）。
2. **外部动作 Gate（实施完成后，逐项单独批准）**：
   - 在真实局域网运行探测扫描（`sshc scan`，会向本链路设备发 TCP 连接）；
   - `git push` 到 `origin/main`；
   - 打 tag / 发布版本。

## 可追溯性

| 设计决策/验收 | 任务 | 验证 |
|---|---|---|
| D1 命令名 `scan`/`discover` | T04 | `scan --help`、别名分发用例 |
| D2 IPv4 枚举 + IPv6 邻居缓存（一期不发探测） | T01、T02 | `TestBuildScanTargets`、`TestParseNeighborTable` |
| D3 banner 判定 SSH | T03 | `TestProbeSSH` |
| D4 私网守卫与 `--max-hosts` | T01、T04 | 守卫用例、退出码 2 用例 |
| D5 `--save` 默认关闭、只追加无凭据条目 | T05 | `TestScanSave` |
| D6 输出契约（表格/JSON、来源不可用降级） | T04、T07 | 表格列断言、`--json` 解析、真实终端输出 |
| 完成定义 5（构建/测试/vet） | T07 | `go build ./...`、`go vet ./...`、`go test ./...` |

## 完成 Gate 与剩余工作

- 完成 Gate：T01–T07 全部 `done`，`go build ./...`、`go vet ./...`、`go test ./...` 通过，真实终端验证片段已记录；完成后在 progress 写 ≤8 行完成总结（主旨、最终状态、自主决策、commit 数与区间、剩余项）。
- 剩余工作（明确不做，留待二期设计）：IPv6 `ff02::1` 主动探测、Windows `arp -a` 加速、mDNS 发现、`--family` 显式开关的取舍。
