# TODO

- run 命令
  - [x] 执行记录日志
  - [x] host 不一定要完整匹配，根据输入 'ab cd' 能匹配到一个就可以，多个时提示
  - [x] 新增 --script 执行本地脚本; --keep-remote-script 保留远端临时脚本; --cwd 指定远端工作目录
  - [x] 新增 --sudo / --sudo-user 支持 sudo 或切换远端执行用户
  - [x] 新增参数支持 --timeout 远端超时设置; --kill-after 超时后强制清理延迟; --env k=v 可以多次设置; --env-file 从文件加载ENV
- add 命令
  - [x] -I, --interactive 交互录入信息(引入 github.com/gookit/cliui 包)
  - [ ] add pwd 加密，已补 docs/password-encryption-design.md 设计边界
  - [x] add 支持从clipboard 读取指定格式的 ip,user,pwd
  - [x] 支持 keypath file
  - [x] 新增备注字段 --remark; 新增 --group 配置 server group，默认 "default"
- [x] 支持读取 ~/.ssh/config 中带 IdentityFile 的 Host 配置
- [ ] 支持读取 ~/.ssh/ 的 password 文件
- [x] 新增 scp/upload -l local-path -r remote-path hostname 命令上传文件到remote
  - [x] 输出 size/files/dirs/elapsed 传输统计
  - [x] 新增 --sha256 文件级校验
  - [x] local-path 支持使用 * 通配符上传多个文件
  - [ ] local-path 支持逗号分隔多个文件
  - [x] 新增选项 --remove-dir 是否上传前先删除远程目录
- [x] 新增 download/dl 从远程下载 文件/目录 到本地路径下
  - [x] 输出 size/files/dirs/elapsed 传输统计
  - [x] 新增 --sha256 文件级校验
- [x] 新增 login/connect 命令，连接并打开 pty 可以连续操作，默认只记录连接元信息

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
