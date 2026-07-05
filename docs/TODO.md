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
- [ ] 新增 host import 批量导入已有 hosts 清单
  - [ ] 支持 `--format ips` IP/hostname 清单
  - [ ] 支持 `--format plain` 多段 KV 文本
  - [ ] 支持 CSV with header
  - [ ] 支持 dry-run、skip-existing、overwrite
- [x] 新增在多个主机(逗号分隔指定多个或者从一个txt文件读取多个ip/host)批量执行指定脚本能力
- [x] 通过中间机器作为跳板到ssh另一个远程机器执行命令等
  - [x] add/host add 支持 --jump 持久化配置默认跳板
  - [ ] 另一种通过 pve 主机到上面的 lxc 或者 vhost 执行命令
- [x] login 命令
  - [x] 未输入或未匹配到host时，使用 cliui newui 交互选择目标
- [ ] 支持导入与导出 hosts 配置数据
  - 导出时会加密整个文件数据，同时生成一个一次性key string
  - 导入时需要指定导出的文件和配套的 key string 才行
- [ ] 安全增强
  - [ ] 后续可考虑配置级解锁机制：设置有效期，过期后本机验证，验证后短期缓存解锁态

## 优化增强

- [x] host 日志优化： 现在 host 日志 jsonl 完整记录了输入输出内容，但是果输出内容很大时音响json日志文件的查看/审计
  - [x] 为每个执行任务都生成 task_id(format=yyyymmdd-hhmmss-shorthash，jsonl 里要记录下来
  - [x] 输出大时，使用独立的文件来保存，文件名(`{task_id}.out.log`) , jsonl 里不再记录完整输出
  - [x] `{task_id}.out.log` 存放到配置的 {logs_path}/yyyymmdd/ 下面
  - [x] sshc log --id {task_id} 查看详细输出

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
