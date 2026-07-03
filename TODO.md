# TODO

- run 命令
  - [x] 执行记录日志
  - host 不一定要完整匹配，根据输入 'ab cd' 能匹配到一个就可以，多个时提示
  - [ ] 新增参数支持 --timeout 超时设置; --env k=v 可以多次设置; --env-file 从文件加载ENV
- add 命令
  - -I, --interactive 交互录入信息
  - add pwd 加密
  - add 支持从clipboard 读取指定格式的 ip,user,pwd
  - 支持 keypath file
  - 新增备注字段 --remark; 新增 --group 配置 server group，默认 "default"
- 支持读取 ~/.ssh/ 的相关配置 
- [x] 新增 scp -l local-path -r remote-path hostname 命令上传文件到remote
- [ ] 新增 download/dl 从远程下载文件到本地
