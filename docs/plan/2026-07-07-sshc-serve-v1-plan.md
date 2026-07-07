# sshc serve v1 实施计划

## 修订记录

| 版本 | 日期 | 修改人 | 调整说明 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-07 | Codex | 初版，基于 serve 整体设计拆分 v1 本地 Web 管理台和 Web Terminal 实施阶段 |

## 关联文档

- 整体设计：`docs/2026-07-07-sshc-serve-design.md`
- 后续能力设计：`docs/2026-07-04-sshc-next-features-design.md`
- 配置与凭证模型设计：`docs/2026-07-04-sshc-config-auth-design.md`
- password 加密设计：`docs/password-encryption-design.md`

## 背景

`sshc serve` v1 的目标是提供本机 Web 管理台和浏览器终端：

```text
browser(localhost) -> sshc serve(127.0.0.1) -> sshc core -> ssh/jump/command_proxy
```

v1 只聚焦本机使用体验：

- 启动本地 HTTP server。
- 内嵌 Web UI。
- 查看和管理 config、host、auth、logs。
- 通过 xterm.js 连接已配置 host。
- 默认仅监听 `127.0.0.1`。

分享链接、远程多人访问、token 持久化、share revoke 等能力放到 v2。

## 已确认决策

- WebSocket 使用 `github.com/coder/websocket`。
- HTTP router 使用 `github.com/gookit/rux/v2`。
- v1 默认监听 `127.0.0.1:8822`。
- v1 默认不做 share link。
- v1 默认不记录完整 terminal 输入输出，只记录连接元信息。
- v1 不把 `sshc serve` 做成完整堡垒机。
- v1 Web Terminal 的 unknown host key 不走 CLI stdin prompt，应返回错误并提示先 trust。
- 前端采用 Vite + TypeScript + xterm.js。
- 正式版通过 `go:embed` 内嵌 Web dist，开发时支持 `--web-dir`。
- 每个阶段完成后独立提交，避免一次提交过大。

## 非目标

- 不做公网穿透。
- 不做多用户账号、RBAC、审批流。
- 不做 share token/TTL/revoke。
- 不默认监听 `0.0.0.0`。
- 不直接暴露完整 config JSON 编辑保存作为主入口。
- 不返回明文 password/password_enc。
- 不默认录制 terminal 完整输入输出。
- 不实现 command_proxy 的 upload/download。
- 不把前端做成 landing page 或营销页。

## 依赖计划

### Go 依赖

新增：

```bash
go get github.com/gookit/rux/v2
go get github.com/coder/websocket
```

用途：

| 依赖 | 用途 |
| --- | --- |
| `github.com/gookit/rux/v2` | HTTP router、API 路由分组、中间件 |
| `github.com/coder/websocket` | Web Terminal WebSocket 连接 |

约束：

- HTTP handler 仍使用标准 `net/http` handler 形态，便于测试。
- WebSocket session 生命周期必须绑定 `context.Context`。
- 不引入完整 Web terminal server 框架，例如 gotty/ttyd/Guacamole。

### 前端依赖

新增 `web/`：

```bash
npm create vite@latest web -- --template vanilla-ts
cd web
npm install @xterm/xterm @xterm/addon-fit
```

建议依赖：

| 依赖 | 用途 |
| --- | --- |
| `@xterm/xterm` | 浏览器 terminal |
| `@xterm/addon-fit` | terminal 自适应尺寸 |

约束：

- v1 不引入 Vue/React。
- UI 以工具型界面为主。
- build 产物由 Go `embed` 使用。
- `web/dist` 不建议提交，除非后续 release 流程明确需要。

## 目标目录结构

```txt
sshc/
|- cmd/sshc/main.go
|- internal/
|  |- bootstrap/init.go
|  |- command/
|  |  |- serve.go
|  |- server/
|  |  |- server.go
|  |  |- routes.go
|  |  |- response.go
|  |  |- auth.go
|  |  |- assets.go
|  |  |- api_health.go
|  |  |- api_config.go
|  |  |- api_hosts.go
|  |  |- api_auth.go
|  |  |- api_logs.go
|  |  |- api_terminal.go
|  |  |- terminal.go
|  |  |- terminal_audit.go
|  |  |- *_test.go
|- web/
|  |- package.json
|  |- index.html
|  |- src/
|  |  |- main.ts
|  |  |- api.ts
|  |  |- terminal.ts
|  |  |- views/
|  |  |- styles/
|- docs/
```

## 命令面

### P1 命令

```bash
sshc serve
sshc serve --addr 127.0.0.1:8822
sshc serve --open
sshc serve --no-open
sshc serve --readonly
sshc serve --web-dir ./web/dist
```

### v1 可选安全参数

```bash
sshc serve --token
sshc serve --token abc...
sshc serve --addr 0.0.0.0:8822 --token
```

规则：

- `--addr` 默认 `127.0.0.1:8822`。
- `--open` 默认 true。
- `--no-open` 禁止启动后打开浏览器。
- `--web-dir` 指向本地 Web dist，用于开发调试。
- `--readonly` 禁止写配置和打开 terminal。
- 非 loopback addr 必须启用 token。

## API 范围

### P1: 基础 API

```text
GET /api/health
```

### P2: 管理 API

```text
GET    /api/config/summary
GET    /api/hosts
GET    /api/hosts/{name}
POST   /api/hosts
PUT    /api/hosts/{name}
DELETE /api/hosts/{name}
POST   /api/hosts/{name}/trust

GET    /api/auth-profiles
GET    /api/auth-profiles/{name}
POST   /api/auth-profiles
PUT    /api/auth-profiles/{name}
DELETE /api/auth-profiles/{name}

GET    /api/logs
GET    /api/logs/{task_id}
GET    /api/logs/{task_id}/output
```

### P4: Terminal API

```text
POST   /api/terminal/sessions
GET    /api/terminal/sessions
GET    /api/terminal/sessions/{id}/ws
POST   /api/terminal/sessions/{id}/resize
DELETE /api/terminal/sessions/{id}
```

WebSocket 用途：

```text
browser xterm input -> websocket -> ssh stdin
ssh stdout/stderr -> websocket -> browser xterm
```

Resize v1 使用独立 HTTP API：

```text
POST /api/terminal/sessions/{id}/resize
```

原因：

- WebSocket 可以保持纯 terminal byte stream。
- resize 请求结构简单，HTTP API 更容易测试。
- 后续需要优化时再考虑 WebSocket JSON/binary 多路协议。

## 通用响应格式

成功：

```json
{
  "ok": true,
  "data": {}
}
```

失败：

```json
{
  "ok": false,
  "error": "host \"devhost\" not found"
}
```

约束：

- 所有 API error 返回 JSON。
- 敏感字段统一 mask。
- 写 API 在 readonly 模式下返回 403。

## P1: serve 基础 server

### 范围

- 增加 Go 依赖：`rux/v2`、`coder/websocket`。
- 新增 `internal/server` 基础包。
- 新增 `internal/command/serve.go`。
- 在 `internal/bootstrap/init.go` 注册 `serve` 命令。
- 提供 `/api/health`。
- 提供静态资源服务。
- 支持 embedded assets 和 `--web-dir`。
- 支持 `--addr`、`--open`、`--no-open`、`--readonly`。

### 关键实现

新增 server config：

```go
type Config struct {
    Addr     string
    Open     bool
    Readonly bool
    WebDir   string
    Token    string
}
```

新增 server：

```go
type Server struct {
    config Config
    router http.Handler
    http   *http.Server
}
```

路由使用 rux：

```go
r := rux.New()
r.GET("/api/health", s.handleHealth)
r.GET("/*", s.handleAssets)
```

注意：

- 如果 `--addr :0`，启动后需要打印实际监听地址。
- `--open` 只在明确可获取访问 URL 时执行。
- Windows 打开浏览器使用现有或新增 util，不要把平台命令散落在 handler 中。

### 测试

- `TestHealthAPI`：`httptest` 请求 `/api/health`。
- `TestReadonlyVisibleInHealth`：readonly 状态返回到 health。
- `TestRejectNonLoopbackWithoutToken`：非 loopback 且无 token 拒绝启动或配置校验失败。
- `TestAssetsFallback`：静态首页可返回。

### 验收

```bash
go test ./internal/server ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
tmp\sshc.exe serve --help
```

### 提交

```text
feat(serve): add local web server
```

## P2: config/host/auth/log 管理 API

### 范围

- Config summary API。
- Host list/show/create/update/delete/trust API。
- Auth profile list/show/create/update/delete API。
- Logs list/detail/output API。
- 统一 mask 敏感字段。
- 写操作使用 `core.CheckConfig()`。
- readonly 模式禁止写操作。

### Host API 规则

- `GET /api/hosts` 默认 mask IP。
- `GET /api/hosts/{name}` 不返回明文 password/password_enc。
- `POST /api/hosts` 复用 host import/add 的字段模型。
- `PUT /api/hosts/{name}` 保存前校验 config。
- `DELETE /api/hosts/{name}` 删除前检查是否存在。
- `POST /api/hosts/{name}/trust` 调用 `core.TrustHostKey`。

### Auth API 规则

- password 不返回。
- password 修改只允许 reset。
- 删除被 host 引用的 auth profile 时，v1 可先返回明确错误。

### Logs API 规则

- 复用现有 run logs。
- output API 支持 `tail` 或 `lines`，避免读取超大文件。
- task detail 中包含 output 文件路径状态，但不泄露不必要的本机路径。

### 测试

- 使用临时 `SSHC_CONFIG`。
- host CRUD 后检查配置文件内容。
- auth CRUD 后检查 password 已加密。
- API response 不包含明文 password/password_enc。
- readonly 模式下 POST/PUT/DELETE 返回 403。
- trust API 用 hook/mock，避免真实网络连接。

### 验收

```bash
go test ./internal/server ./internal/core ./internal/command
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
```

### 提交

```text
feat(serve): add config management api
```

## P3: Web UI v1

### 范围

- 新增 `web/` Vite + TypeScript 项目。
- 引入 `@xterm/xterm`、`@xterm/addon-fit`。
- 实现基础 layout。
- 实现 Hosts 页面。
- 实现 Auth 页面。
- 实现 Logs 页面。
- 实现 Config summary 页面。
- 前端 build 产物由 Go embed 加载。

### UI 原则

- 第一屏直接进入工具界面，不做 landing page。
- 亮色、简洁、工具型。
- 信息密度适中，适合日常运维扫描。
- 不展示密码。
- IP 默认 mask，提供临时显示入口。
- 常用操作使用按钮和图标，但避免复杂动效。

### 页面

```text
Hosts:
  search, group filter, table, add/edit/delete/trust/connect

Auth:
  table, add/edit/reset password/delete

Logs:
  task list, task detail, output viewer

Config:
  config path, logs_path, defaults, doctor issues
```

### 前端结构

```txt
web/src/
|- main.ts
|- api.ts
|- terminal.ts
|- views/
|  |- HostsView.ts
|  |- AuthView.ts
|  |- LogsView.ts
|  |- ConfigView.ts
|- styles/
|  |- app.css
```

### 测试和验证

- `npm run build`。
- Go build 能嵌入 dist。
- Playwright 不是 v1 必需，但需要至少用本地 HTTP smoke 验证首页可访问。
- 需要检查移动端不要求完整适配，但桌面宽度下不能有明显重叠。

### 验收

```bash
npm --prefix web install
npm --prefix web run build
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
tmp\sshc.exe serve --addr 127.0.0.1:0 --no-open
```

### 提交

```text
feat(web): add serve web ui
```

## P4: Web Terminal

### 范围

- 新增 terminal session manager。
- 后端使用 `github.com/coder/websocket` 提供 WebSocket。
- 前端使用 xterm.js 连接 WebSocket。
- 支持普通 SSH host。
- 支持单级 jump host。
- 支持 resize。
- 支持断线释放资源。
- 写 terminal 审计元信息。
- unknown host key 返回明确错误，提示 trust。

### TerminalManager

建议结构：

```go
type TerminalManager struct {
    sessions map[string]*TerminalSession
}
```

Session：

```go
type TerminalSession struct {
    ID        string
    Host      core.Host
    StartedAt time.Time
    CloseOnce sync.Once
}
```

需要封装：

- SSH session。
- stdin writer。
- stdout/stderr reader。
- close cleanup。
- resize。

### SSH PTY

普通 SSH host：

```text
core.ResolveHostWithSSHConfig -> new SSH client -> ssh.Session -> RequestPty -> Shell
```

jump host：

```text
复用现有 jump connection 逻辑创建 SSH client
```

command_proxy：

- v1 可选。
- 如果支持，只支持 `login_command`。
- 如果不支持，返回明确错误：

```text
command_proxy web terminal is not supported yet
```

### WebSocket 行为

`GET /api/terminal/sessions/{id}/ws`：

- Accept websocket。
- browser message 写入 SSH stdin。
- SSH stdout/stderr 写回 websocket。
- 任一方向结束，关闭另一方向。
- close 后 manager 删除 session。

### Resize

`POST /api/terminal/sessions/{id}/resize`：

```json
{
  "cols": 120,
  "rows": 40
}
```

调用 SSH `WindowChange`。

### 审计日志

路径建议：

```text
{logs_path}/terminal/{yyyyMMdd}.jsonl
```

事件：

```text
start
resize
close
error
```

字段：

```json
{
  "time": "2026-07-07T12:00:00.123",
  "session_id": "term_...",
  "host": "devhost",
  "remote_addr": "127.0.0.1:54321",
  "event": "start",
  "message": ""
}
```

### 测试

- TerminalManager create/get/delete。
- Resize 参数校验。
- WebSocket handler 使用 fake terminal session 测试 byte 转发。
- 断开 WebSocket 后 session 被 cleanup。
- known_hosts unknown 错误路径测试。

真实 SSH 连接不作为单元测试依赖。

### 验收

```bash
go test ./internal/server ./internal/core
go test ./...
npm --prefix web run build
go build -o tmp\sshc.exe ./cmd/sshc
```

手工 smoke：

```bash
tmp\sshc.exe serve --addr 127.0.0.1:8822
```

浏览器验证：

- 打开 Hosts 页面。
- 点击普通 SSH host Connect。
- terminal 可输入 `pwd`、`exit`。
- 关闭 tab 后后端 session 释放。

### 提交

```text
feat(serve): add web terminal sessions
```

## P5: 安全和本地访问控制

### 范围

- token 登录。
- 非 loopback addr 强制 token。
- session cookie。
- CSRF header。
- readonly guard。
- sensitive response mask 统一收口。

### Token 规则

```bash
sshc serve --token
sshc serve --token abc
```

- `--token` 不带值时自动生成。
- 自动 token 只打印一次。
- 内存中只保存 token hash。
- token 登录成功后设置 cookie。

### CSRF

- 登录后返回 CSRF token。
- 写请求要求 `X-SSHC-CSRF`。
- readonly 模式下写请求先被拒绝。

### 测试

- 未登录访问管理 API 返回 401。
- token 登录成功。
- token 错误返回 401。
- 写请求缺 CSRF 返回 403。
- 非 loopback 无 token 拒绝启动。
- readonly 写请求返回 403。

### 验收

```bash
go test ./internal/server
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
```

### 提交

```text
feat(serve): add local access control
```

## P6: 文档和收口

### 范围

- 更新 `README.md`。
- 更新 `README.zh-CN.md`。
- 更新 `docs/TODO.md` serve v1 状态。
- 更新 `docs/2026-07-07-sshc-serve-design.md` 修订记录和实现状态。
- 补充 serve 安全说明。
- 补充 Web Terminal known_hosts/trust 说明。
- 补充开发构建说明。

### README 示例

```bash
sshc serve
sshc serve --addr 127.0.0.1:8822
sshc serve --addr 0.0.0.0:8822 --token
```

### 安全说明

必须说明：

- 默认仅本机访问。
- 绑定 `0.0.0.0` 等价于暴露 SSH 管理入口。
- 远程访问必须使用 token。
- 不要把 serve 裸露到公网。
- password 不会在 API/UI 中明文展示。

### 验收

```bash
go test ./...
npm --prefix web run build
go build -o tmp\sshc.exe ./cmd/sshc
git diff --check -- .
```

### 提交

```text
docs: document serve web ui
```

## 实施顺序

```text
P1 serve 基础 server
P2 管理 API
P3 Web UI v1
P5 安全和本地访问控制
P4 Web Terminal
P6 文档和收口
```

说明：

- P5 可在 P4 前完成，避免 terminal API 先暴露后再补安全。
- P3 可以先使用 mock/真实管理 API，不依赖 terminal。
- P4 是风险最高阶段，应在基础 UI 和访问控制稳定后实施。

## 验证总清单

最终完成 v1 时必须通过：

```bash
go test ./...
npm --prefix web run build
go build -o tmp\sshc.exe ./cmd/sshc
tmp\sshc.exe serve --help
git diff --check -- .
```

手工 smoke：

- `sshc serve --addr 127.0.0.1:0 --no-open` 可以启动。
- `/api/health` 正常。
- Web UI 首页正常。
- Hosts/Auth/Logs/Config 页面可访问。
- 可新增和编辑 host。
- password 不在页面/API 明文展示。
- 普通 SSH host terminal 可连接和退出。
- 断开 terminal 后 server session 清理。
- 非 loopback 无 token 被拒绝。

## 风险和处理

### WebSocket 与 SSH session cleanup

风险：

- 浏览器断开后 SSH session 未释放。
- SSH session 退出后 WebSocket 未关闭。

处理：

- 所有 goroutine 绑定 context。
- 使用 `sync.Once` 做 close。
- manager 删除 session 放在统一 cleanup 中。
- 单测覆盖 websocket close 和 SSH close 两条路径。

### known_hosts 交互

风险：

- Web Terminal 连接 unknown host 时阻塞在 CLI prompt。

处理：

- Web Terminal 使用非交互 host key 策略。
- unknown host key 返回错误。
- UI 提供 Trust 按钮调用 `POST /api/hosts/{name}/trust`。

### 配置并发写入

风险：

- CLI 和 serve 同时写 config。

处理：

- v1 每次写操作前重新 LoadConfig。
- 保存前 CheckConfig。
- 后续再设计文件锁。

### 前端构建产物

风险：

- dist 是否入仓和 release 流程未定。

处理：

- v1 先不提交 `web/dist`。
- Go embed 可先嵌入一个最小 fallback 静态页面。
- release 构建流程再决定是否生成并嵌入 dist。

## 待确认事项

- P4 是否必须在 v1 支持 command_proxy `login_command`。
- `web/dist` 是否提交到仓库，还是只在 release workflow 中生成。
- token 登录是否 v1 就默认开启，还是仅非 loopback 时要求。
- terminal audit log 是否使用 `{logs_path}/terminal/{yyyyMMdd}.jsonl`。
- `--open` 默认 true 是否适合 Windows/Linux/macOS 全平台。
- Web UI 是否需要 `host trust` 按钮作为 v1 必做项。

## 当前状态

- [ ] P1: serve 基础 server
- [ ] P2: config/host/auth/log 管理 API
- [ ] P3: Web UI v1
- [ ] P4: Web Terminal
- [ ] P5: 安全和本地访问控制
- [ ] P6: 文档和收口
