# sshc serve 整体规划设计

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-07 | Codex | 初版，规划 sshc serve 本地 Web 管理台、Web Terminal、分享链接和安全边界 |

## 背景

`sshc` 当前已经具备轻量 SSH 运维 CLI 的主要能力：

- 使用 `~/.config/sshc/sshc.config.json` 管理配置。
- 支持 host/auth/cfg 管理命令。
- 支持 run、batch-run、login、upload/scp、download、log。
- 支持密码本机加密、`~/.ssh/config`、known_hosts、jump host、command_proxy。
- 支持按 host/task 记录执行日志。

这些能力适合 CLI 使用，但在以下场景里仍不够方便：

- 需要快速浏览和编辑 host/auth/config 信息。
- 需要用浏览器打开多个远程终端，避免本地终端窗口过多。
- 需要给他人临时访问某个主机的终端，但又不希望直接给出 host 密码或完整配置。

因此规划新增 `sshc serve`，提供本地 Web 管理台和浏览器终端能力。

## 总体定位

`sshc serve` 的定位是：

```text
本机 sshc CLI 能力的 Web 管理入口，不是完整堡垒机或中心化运维平台。
```

推荐边界：

```text
ssh/scp 手写命令 < sshc CLI < sshc serve < Teleport/堡垒机/Ansible 平台
```

`sshc serve v1` 应默认只服务本机使用：

```text
browser(localhost) -> sshc serve(127.0.0.1) -> sshc core -> ssh/jump/command_proxy
```

`sshc serve v2` 再考虑临时分享和远程访问：

```text
browser(remote) -> token/share link -> sshc serve -> selected host terminal
```

## 目标

- 提供 `sshc serve` 命令启动本地 HTTP server 和内嵌 Web UI。
- Web UI 使用亮色、简洁、工具型界面。
- v1 支持本地查看和管理 config、host、auth、logs。
- v1 支持通过浏览器 xterm.js 连接已配置 SSH host。
- v1 默认仅监听 `127.0.0.1`，避免意外暴露。
- v2 支持生成单个 host 的临时 terminal 分享链接。
- v2 分享链接具备 token、TTL、审计和撤销能力。
- 复用现有 core 能力，不复制配置保存、密码加密、host 解析和 SSH 连接逻辑。

## 非目标

- 不把 `sshc serve` 做成完整堡垒机。
- 不做多用户账号体系和 RBAC。
- 不做组织、项目、团队、审批流等平台能力。
- 不做公网穿透。
- 不默认监听 `0.0.0.0`。
- 不默认记录 Web Terminal 的完整输入输出。
- 不直接展示或导出明文 password/password_enc。
- v1 不做分享链接。
- v1 不要求支持 command_proxy 的 upload/download。

## 命令设计

### 基础命令

```bash
sshc serve
sshc serve --addr 127.0.0.1:8822
sshc serve --open
sshc serve --no-open
sshc serve --readonly
sshc serve --web-dir ./web/dist
```

建议默认值：

| 选项 | 默认值 | 说明 |
| --- | --- | --- |
| `--addr` | `127.0.0.1:8822` | HTTP 监听地址 |
| `--open` | true | 启动后打开浏览器 |
| `--readonly` | false | 只读模式下禁止修改配置和打开终端 |
| `--web-dir` | 空 | 开发调试时读取外部 Web dist，正式版使用 go:embed |

### 安全相关命令选项

```bash
sshc serve --token
sshc serve --token abc...
sshc serve --addr 0.0.0.0:8822 --token
```

建议规则：

- 默认 localhost 访问可以使用进程内 session cookie。
- 如果监听地址不是 loopback，必须启用 token。
- `--token` 不带值时自动生成随机 token，只在启动日志中打印一次。
- `--token <value>` 允许用户显式设置访问 token，适合脚本或临时内网使用。

### 后续分享命令

分享能力不建议塞进 `serve` 子选项，应单独建命令：

```bash
sshc share devhost --ttl 30m
sshc share devhost --ttl 30m --max-uses 1
sshc share list
sshc share revoke <share_id>
```

`share` 命令只负责创建和管理分享授权，实际访问仍由 `sshc serve` 提供。

## 模块结构

建议新增 `internal/server` 包，把 HTTP server 和 CLI command 解耦。

```txt
internal/
|- command/
|  |- serve.go              # gcli 命令入口
|- server/
|  |- server.go             # server 生命周期和配置
|  |- routes.go             # 路由注册
|  |- response.go           # JSON response helper
|  |- auth.go               # token/cookie/CSRF
|  |- assets.go             # go:embed Web dist
|  |- api_config.go         # config API
|  |- api_hosts.go          # host API
|  |- api_auth.go           # auth profile API
|  |- api_logs.go           # log API
|  |- api_terminal.go       # terminal session API
|  |- terminal.go           # Web Terminal session manager
|  |- share.go              # v2 share runtime model
```

前端建议放在项目根目录 `web/`：

```txt
web/
|- package.json
|- index.html
|- src/
|  |- main.ts
|  |- api.ts
|  |- terminal.ts
|  |- views/
|  |- styles/
|- dist/
```

正式构建产物通过 `go:embed` 打包进二进制。开发时可以用 `--web-dir web/dist` 指定本地构建产物。

## 后端架构

### 分层

```text
command/serve.go
  -> server.Server
    -> server API handlers
      -> core config/auth/host/ssh/log functions
```

约束：

- `command` 只解析 CLI 参数并启动 server。
- `server` 只处理 HTTP、WebSocket、session 和 UI 资源。
- `core` 继续保存业务能力，不依赖 HTTP。
- 配置保存仍走 `core.LoadConfig()`、`core.CheckConfig()`、`core.SaveConfig()`。
- 密码加密仍走现有本机 key 机制。

### Server 配置

建议建模：

```go
type Config struct {
    Addr     string
    Open     bool
    Readonly bool
    WebDir   string
    Token    string
}
```

server 启动后生成运行态：

```go
type Server struct {
    config   Config
    http     *http.Server
    sessions *TerminalManager
}
```

## HTTP API 设计

### 通用响应

建议保持轻量统一格式：

```json
{
  "ok": true,
  "data": {},
  "error": ""
}
```

错误：

```json
{
  "ok": false,
  "error": "host \"devhost\" not found"
}
```

### Health

```text
GET /api/health
```

返回：

```json
{
  "ok": true,
  "data": {
    "name": "sshc",
    "version": "0.0.0",
    "readonly": false
  }
}
```

### Config API

```text
GET /api/config
GET /api/config/summary
PUT /api/config/defaults
PUT /api/config/logs-path
```

原则：

- v1 不推荐直接提供完整 JSON 编辑保存作为主入口。
- `GET /api/config` 输出必须 mask 敏感字段。
- 修改配置应拆成结构化 API，便于校验。
- 如果后续支持 raw JSON 编辑，必须先经过 `core.CheckConfig()`。

### Host API

```text
GET    /api/hosts
GET    /api/hosts/{name}
POST   /api/hosts
PUT    /api/hosts/{name}
DELETE /api/hosts/{name}
POST   /api/hosts/{name}/trust
```

行为：

- list 默认 mask IP，与 CLI `list` 一致。
- show 不返回明文 password/password_enc。
- create/update 复用 core host 校验。
- delete 需要前端确认。
- trust 调用 `core.TrustHostKey`，用于避免 Web Terminal 中卡住 known_hosts prompt。

### Auth API

```text
GET    /api/auth-profiles
GET    /api/auth-profiles/{name}
POST   /api/auth-profiles
PUT    /api/auth-profiles/{name}
DELETE /api/auth-profiles/{name}
```

行为：

- password 不返回。
- password 修改只允许 reset，不允许读取旧值。
- 保存时继续写 `password_enc`。
- auth 被 host 引用时删除需要提示影响。

### Logs API

```text
GET /api/logs
GET /api/logs/{task_id}
GET /api/logs/{task_id}/output
```

行为：

- 复用当前 host/task 日志模型。
- task detail 中如果输出被拆到 `{task_id}.out.log`，通过 output API 读取。
- 大文件输出应支持 tail/lines 参数，避免一次性加载过大。

### Terminal API

```text
POST   /api/terminal/sessions
GET    /api/terminal/sessions
GET    /api/terminal/sessions/{id}/ws
POST   /api/terminal/sessions/{id}/resize
DELETE /api/terminal/sessions/{id}
```

创建 session：

```json
{
  "host": "devhost",
  "cols": 120,
  "rows": 40
}
```

返回：

```json
{
  "ok": true,
  "data": {
    "id": "term_20260707_abc123",
    "host": "devhost"
  }
}
```

## Web Terminal 设计

### 连接流程

```text
1. 用户在 Hosts 页面点击 Connect。
2. 前端 POST /api/terminal/sessions。
3. 后端解析 host 配置。
4. 后端建立 SSH PTY session。
5. 前端连接 WebSocket。
6. xterm.js 输入输出通过 WebSocket 双向转发。
7. 浏览器 resize 触发 PTY resize。
8. 断开时关闭 SSH session，并写审计日志。
```

### PTY 抽象

建议在 core 或 server 内部抽象 terminal session：

```go
type TerminalSession interface {
    ID() string
    HostName() string
    Write([]byte) (int, error)
    Read([]byte) (int, error)
    Resize(cols, rows int) error
    Close() error
}
```

后续可扩展：

- 普通 SSH host。
- jump host。
- command_proxy login_command。

### 支持范围

v1 推荐支持：

- 普通 SSH host。
- 单级 jump host。
- 通过 `host trust` 或 UI trust 预先处理 known_hosts。

v1 可选支持：

- command_proxy `login_command`。

v1 不建议支持：

- 在 terminal 内弹 CLI known_hosts prompt。
- Web Terminal 完整输入输出审计。
- 浏览器直接传 password 建立临时 host。

## Web UI 设计

### 页面结构

第一屏直接进入工具界面，不做 landing page。

```text
Topbar:
  Hosts | Auth | Logs | Config | Settings

Main:
  当前页面内容

Terminal Drawer/Tabs:
  多个 terminal session
```

### Hosts 页面

主要元素：

- group filter。
- search input。
- host table。
- add/edit/delete/trust/connect 操作。
- IP 默认 mask，可点击临时显示。

字段：

```text
name | ip | user/auth | group | jump | backend | remark | actions
```

### Auth 页面

字段：

```text
name | user | key_path | remark | used_by | actions
```

password：

- 不显示。
- 只提供 reset password。

### Logs 页面

能力：

- 按 host、task_id、时间过滤。
- 查看最近运行记录。
- 查看 task detail。
- 查看大输出文件，支持 tail/lines。

### Config 页面

展示：

- config path。
- logs_path。
- defaults。
- known_hosts_path。
- doctor issues。

修改：

- v1 优先提供结构化表单。
- raw JSON editor 可作为后续高级能力。

### Terminal

能力：

- xterm.js。
- fit addon 自动适配。
- 多 terminal tab。
- reconnect 不作为 v1 必须能力。
- terminal 关闭时确认。

## 前端技术选型

推荐：

- Vite。
- TypeScript。
- xterm.js。
- `@xterm/addon-fit`。
- 不强制引入 Vue/React。

理由：

- 页面结构简单，vanilla TS 足够。
- 依赖少，构建产物小。
- 更贴合 CLI 工具项目，不把前端复杂度放大。

如果后续 UI 复杂度上升，再考虑 Vue。

## 安全设计

### 默认本地访问

默认监听：

```text
127.0.0.1:8822
```

原因：

- 本机 sshc 配置包含 SSH 凭证和 host 信息。
- Web Terminal 等价于打开远程 shell。
- 默认暴露到局域网风险过高。

### Token

规则：

- 非 loopback 监听必须启用 token。
- 自动 token 只打印一次。
- 服务端只保存 token hash。
- token 通过 cookie/session 维持登录态。

### CSRF

建议：

- 写操作要求 `X-SSHC-CSRF` header。
- 登录成功后下发 CSRF token。
- localhost 场景也保持同一套机制，避免后续远程访问时补安全债。

### 敏感字段

以下字段禁止明文返回：

- `password`
- `password_enc`
- export key
- server token

API 输出统一使用 mask：

```json
{
  "password_enc": "***"
}
```

### Terminal 审计

v1 记录连接元信息：

- session_id。
- host。
- started_at。
- ended_at。
- remote_addr。
- exit_status。
- close_reason。

v1 不默认记录完整输入输出。

后续可加显式开关：

```bash
sshc serve --terminal-record
```

## 分享链接设计 v2

### 目标

允许用户临时分享单个 host 的 Web Terminal 访问入口：

```text
http://127.0.0.1:8822/xterm/{share_id}?token=...
```

适用场景：

- 临时给同事操作某台机器。
- 不暴露 host 密码。
- 不导出完整 sshc 配置。

### Share 模型

```json
{
  "id": "st_abc123",
  "host": "devhost",
  "token_hash": "...",
  "created_at": 1780000000,
  "expires_at": 1780001800,
  "max_uses": 1,
  "used_count": 0,
  "revoked": false,
  "remark": "deploy support"
}
```

### 命令

```bash
sshc share devhost --ttl 30m
sshc share devhost --ttl 30m --max-uses 1
sshc share list
sshc share revoke st_abc123
```

### 限制

- 分享只授权 terminal，不授权 config/host/auth API。
- token 明文只显示一次。
- token 存储 hash。
- 过期或撤销后立即不可用。
- 所有 share session 必须写审计日志。

## 配置扩展

可以在 `sshc.config.json` 增加 serve 配置，但 v1 可先只通过命令参数控制。

后续配置：

```json
{
  "serve": {
    "addr": "127.0.0.1:8822",
    "open": true,
    "readonly": false,
    "token_required": false,
    "terminal_record": false
  }
}
```

分享链接如果需要跨进程持久化，可以增加：

```json
{
  "shares_path": "~/.config/sshc/shares.json"
}
```

但 v1 不建议引入 shares 持久化。

## 日志设计

`sshc serve` 自身运行日志可纳入后续 “使用 log/slog 记录 sshc 自己运行日志到文件” 项。

建议路径：

```text
~/.config/sshc/logs/serve.log
```

或复用全局 `logs_path`：

```text
{logs_path}/serve/serve.log
```

terminal 审计日志：

```text
{logs_path}/terminal/{yyyyMMdd}.jsonl
```

字段：

```json
{
  "time": "2026-07-07T12:00:00.123",
  "session_id": "term_...",
  "host": "devhost",
  "remote_addr": "127.0.0.1:54321",
  "event": "start|close|error",
  "exit_status": 0,
  "message": ""
}
```

## 分期计划

### P1: serve 基础 server

内容：

- 新增 `sshc serve` 命令。
- 新增 `internal/server` 基础结构。
- 支持 `--addr`、`--open`、`--no-open`、`--readonly`、`--web-dir`。
- 提供 `/api/health`。
- 支持 go:embed 静态文件。
- 开发模式支持外部 `web/dist`。

验收：

- `sshc serve --addr 127.0.0.1:0` 可以启动。
- `/api/health` 返回正常。
- 静态首页可访问。
- 测试覆盖 server 启动和 health API。

建议提交：

```text
feat(serve): add local web server
```

### P2: 管理 API

内容：

- Config summary API。
- Host CRUD API。
- Auth CRUD API。
- Logs list/detail API。
- 统一 mask 敏感字段。
- 写操作走 `core.CheckConfig()`。

验收：

- 使用临时 `SSHC_CONFIG` 完成 host/auth 增删改查测试。
- password 不出现在 API 响应中。
- 非法配置保存被拒绝。

建议提交：

```text
feat(serve): add config management api
```

### P3: Web UI v1

内容：

- 新建 `web/`。
- Vite + TypeScript。
- Hosts/Auth/Logs/Config 页面。
- 调用管理 API。
- build 产物嵌入 Go。

验收：

- `sshc serve` 打开浏览器后可查看 hosts。
- 可新增/编辑 host。
- 可查看 logs。
- UI 不展示敏感字段。

建议提交：

```text
feat(web): add serve web ui
```

### P4: Web Terminal

内容：

- Terminal session manager。
- WebSocket 转发。
- xterm.js terminal tab。
- resize 支持。
- 普通 SSH host 和 jump host 支持。
- terminal audit log。
- unknown host key 提示用户先 trust 或调用 trust API。

验收：

- 浏览器可连接普通 SSH host。
- 浏览器可连接 jump host 后面的 target。
- 关闭页面后 SSH session 释放。
- 审计日志记录 start/close。

建议提交：

```text
feat(serve): add web terminal sessions
```

### P5: 文档和安全说明

内容：

- README/README.zh-CN 增加 serve 使用说明。
- 文档说明默认 localhost、安全 token、不要公网裸露。
- TODO 标记 v1 已完成的子项。

验收：

- README 示例可执行。
- 安全边界说明清晰。
- `go test ./...` 和 build 通过。

建议提交：

```text
docs: document serve web ui
```

### P6: share v2

内容：

- `sshc share` 命令。
- share token/TTL/max-uses/revoke。
- `/xterm/{share_id}` 页面。
- share session 审计。

验收：

- 生成一次性链接。
- token 过期后不可访问。
- revoke 后不可访问。
- share 访问不能调用管理 API。

建议提交：

```text
feat(serve): add terminal share links
```

## 关键风险

### Web Terminal 复杂度

Web Terminal 涉及 SSH PTY、WebSocket、浏览器 resize、断线释放资源，是 v1 中最高风险部分。

缓解：

- 先做普通 SSH host。
- 再做 jump host。
- command_proxy login_command 放到后续验证。
- unknown host key 不在 terminal 内做 CLI prompt。

### 安全误暴露

如果用户监听 `0.0.0.0` 且无 token，等价于把 SSH 管理入口暴露给局域网。

缓解：

- 非 loopback 地址强制 token。
- 启动日志明确打印监听地址和安全提示。
- API 默认不返回敏感字段。

### 配置并发写入

CLI 和 serve 可能同时修改配置文件。

缓解：

- 保存前重新加载配置并校验。
- 后续可增加文件锁。
- v1 先保持单用户本机使用假设。

### 前端复杂度膨胀

如果一开始引入重型前端框架，项目维护成本会上升。

缓解：

- v1 用 Vite + TypeScript + xterm.js。
- UI 做工具型页面，不做复杂状态平台。

## 待确认事项

- v1 是否必须支持 command_proxy 的 `login_command` Web Terminal。
- v1 是否允许 `--addr 0.0.0.0:8822 --token`，还是只允许 localhost。
- Web UI 是否采用 vanilla TypeScript，还是使用 Vue。
- 是否需要在 v1 支持 raw config JSON editor。
- terminal 审计是否只记录元信息，还是提供可选完整录制。
- share v2 的 token 状态是否需要持久化到文件，还是仅 server 进程内有效。

## 结论

建议按以下边界推进：

```text
sshc serve v1 = localhost Web 管理台 + host/auth/config/log 管理 + 普通 SSH Web Terminal
sshc serve v2 = 单 host 临时 terminal 分享链接 + token/TTL/审计/撤销
```

该边界能先把本机使用体验做好，同时避免过早进入堡垒机、多用户、安全平台化等复杂领域。
