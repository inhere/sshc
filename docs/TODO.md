# TODO

- run 命令
  - [x] 执行记录日志
  - host 不一定要完整匹配，根据输入 'ab cd' 能匹配到一个就可以，多个时提示
  - [x] 新增参数支持 --timeout 超时设置; --env k=v 可以多次设置; --env-file 从文件加载ENV
- add 命令
  - -I, --interactive 交互录入信息(引入 githu.com/gookit/cliui 包)
  - add pwd 加密
  - add 支持从clipboard 读取指定格式的 ip,user,pwd
  - 支持 keypath file
  - 新增备注字段 --remark; 新增 --group 配置 server group，默认 "default"
- [ ] 支持读取 ~/.ssh/ 的相关配置 config, password 文件
- [x] 新增 scp -l local-path -r remote-path hostname 命令上传文件到remote
  - [ ] local-path 支持使用 *通配符, 逗号分隔多个文件
  - [ ] 新增选项 --remove-dir 是否上传前先删除远程目录
- [x] 新增 download/dl 从远程下载 文件/目录 到本地路径下
- [ ] 新增 login/connect 命令，连接并打开 pty 可以连续操作(这种可以记录到命令执行日志吗？)

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
