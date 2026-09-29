# TODO

- run 命令
  - [x] 执行记录日志
  - [x] host 不一定要完整匹配，根据输入 'ab cd' 能匹配到一个就可以，多个时提示
  - [x] 新增 --script 执行本地脚本; --keep-remote-script 保留远端临时脚本; --cwd 指定远端工作目录
  - [x] 新增 --sudo / --sudo-user 支持 sudo 或切换远端执行用户
  - [x] 新增参数支持 --timeout 远端超时设置; --kill-after 超时后强制清理延迟; --env k=v 可以多次设置; --env-file 从文件加载ENV
- add 命令
  - [x] -I, --interactive 交互录入信息(引入 github.com/gookit/cliui 包)
  - [x] add pwd 加密，默认写入 password_enc，兼容读取旧 password 明文
  - [x] add 支持从clipboard 读取指定格式的 ip,user,pwd
  - [x] 支持 keypath file
  - [x] 新增备注字段 --remark; 新增 --group 配置 server group，默认 "default"
- [x] 支持读取 ~/.ssh/config 中带 IdentityFile 的 Host 配置
- [x] 新增 scp/upload -l local-path -r remote-path hostname 命令上传文件到remote
  - [x] 输出 size/files/dirs/elapsed 传输统计
  - [x] 新增 --sha256 文件级校验
  - [x] local-path 支持使用 * 通配符上传多个文件
  - [x] 新增选项 --remove-dir 是否上传前先删除远程目录
- [x] 新增 download/dl 从远程下载 文件/目录 到本地路径下
  - [x] 输出 size/files/dirs/elapsed 传输统计
  - [x] 新增 --sha256 文件级校验
- [x] 新增 login/connect 命令，连接并打开 pty 可以连续操作，默认只记录连接元信息
- [x] 新增 config/cfg 命令，用于简单的管理 sshc 的配置
  - [x] 新增 auth/cred 保存账号或凭证信息用于多个主机共享登录信息
  - [x] 新增 host/hosts 管理命令，避免管理类命令干扰常用命令
- [x] 新增 host import 批量导入已有 hosts 清单
  - [x] 支持 `--format ips` IP/hostname 清单
  - [x] 支持 `--format plain` 多段 KV 文本
  - [x] 支持 CSV with header
  - [x] 支持 dry-run、skip-existing、overwrite
- [x] 新增在多个主机(逗号分隔指定多个或者从一个txt文件读取多个ip/host)批量执行指定脚本能力
- [x] 通过中间机器作为跳板到ssh另一个远程机器执行命令等
  - [x] add/host add 支持 --jump 持久化配置默认跳板
  - [x] 通过 command_proxy 支持经由 pve/docker/vhost 宿主机代理执行 run/batch-run/login
  - [ ] 后续设计 command_proxy 的脚本注入和 upload/download 模板能力
- [x] login 命令
  - [x] 未输入或未匹配到host时，使用 cliui newui 交互选择目标
- [x] 支持 cfg export/import 完整配置迁移
  - [x] 导出时加密完整配置包，同时生成一次性 export key
  - [x] 导入时指定导出文件和 export key，并用目标机器本地 key 重新加密密码
  - [x] import 支持默认 merge、overwrite、replace，并在导入前备份当前配置
- [ ] 安全增强
  - [ ] 后续可考虑配置级解锁机制：设置有效期，过期后本机验证，验证后短期缓存解锁态

## 新增功能

- [x] sshc serve v1 启动 server + webui(内嵌)
  - [x] 使用亮色简洁美观的 UI 设计
  - [x] 首版本地访问即可，方便通过UI管理信息
  - [x] 查看并管理 config, host, auth 等
  - [x] 通过 xterm.js 连接访问目标主机
  - [ ] 后续增强 command_proxy host 的 Web Terminal 支持
- [x] 本地端口转发 `tunnel/tun`（docs/design/2026-09-27-sshc-port-forwarding-design.md）
  - [x] `tunnel add/list/show/rm/forward`，别名 `tun`，默认前台运行
  - [x] 双目标模式：`--target`（已保存 host，保存时固化为规范 host 名）与 `--address`（未登记地址 + `--auth`）
  - [x] profile 可保存可选 `port`/`jump`；一个 SSH 会话承载多条 `local=remote` 规则
  - [x] 本地端口 `0` 由系统分配并在就绪行/`--json` 输出实际端口；只监听 loopback
  - [x] 空闲会话 keepalive（30s/10s）与连接断开检测，Ctrl-C 有序关闭 listener/连接/会话
  - [x] `tunnels` 参与 `cfg export/import`；`cfg doctor` 对过期 tunnel 只报 warn；`auth rm`/`host rm` 维护引用一致性
  - [ ] 后续：后台 daemon 与 `forward list/stop`、`ssh -R`/SOCKS、command_proxy 转发、转发审计日志
  - [ ] A9 真机验证（用户执行，跟踪 my-tools-bwp）：隧道与协议可达性、断线检测已在 cd-testing 实测通过（Redis PING→-NOAUTH、MySQL 握手 5.7.28、Postgres SSLRequest→N、杀死会话后 exit 2 且端口释放）；仍需用真实凭据做一次业务读写，并在独立控制台里验证真实 Ctrl-C（退出码 0）
- [ ] sshc serve v2 还需要思考完善逻辑
  - [ ] 允许通过浏览器 xterm 访问已配置的 hosts，避免直接给出 host 密码
  - [ ] 分享单个 /xterm/{uni-hashid} 主机 xterm.js 访问，免密/token + 时效 + 审计
    - 也可以生成一次性的临时 token 给对方使用

## 优化增强

- [x] sshc host set 命令不使用options进行设置，改为 `host set hostname k=v k1=v1 ...` 这种格式
- [x] 新增 host tags，支持 add/import/list/match 和 `--tag` 过滤
- [x] 新增 group defaults，支持 `group set testing auth=dev-root jump=bastion port=22`
- [x] 新增 `sshc check` 主机健康检查
- [x] `host import` 支持 `--from-ssh-config`
- [x] `batch-run` 新增 batch summary JSONL 和 `--rerun-failed`
- [x] 使用 log/slog 记录 sshc 自己的运行日志到文件
- [x] 新增环境变量 SSHC_CONFIG_DIR 设置默认的 config-dir 方便自定义和测试
- [x] host 日志优化： 现在 host 日志 jsonl 完整记录了输入输出内容，但是果输出内容很大时音响json日志文件的查看/审计
  - [x] 为每个执行任务都生成 task_id(format=yyyymmdd-hhmmss-shorthash)，jsonl 里要记录下来
  - [x] 输出大时，使用独立的文件来保存，文件名(`{task_id}.out.log`) , jsonl 里不再记录完整输出
  - [x] `{task_id}.out.log` 存放到配置的 {logs_path}/yyyymmdd/ 下面
  - [x] `sshc log --id {task_id}` 查看详细输出

## [x] 整理项目结构

```txt
sshc/
|- cmd/sshc/main.go
|- internal/
|  |- bootstrap/init.go   # 引导启动 create app, add commands, run app...
|  |- core/               # 核心逻辑文件
|  |- command/            # 子命令，每个命令一个 go 文件
|  |- util/               # 工具文件包
|  |- ... 其他独立功能子包
|- README.md
|- ...
```
