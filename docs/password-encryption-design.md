# sshc password 加密说明

## 当前结论

`sshc` 默认不再把新增或更新的主机密码明文写入 `sshc.config.json`。

保存主机时：

- 内存中的 `Host.Password` 仍用于本次 SSH 认证。
- 写入 `sshc.config.json` 前会把 `password` 加密为 `password_enc`。
- 落盘后的 `password` 字段为空并因 `omitempty` 不写入。
- 旧版本已有的明文 `password` 字段仍可读取，避免升级后配置失效。

示例：

```json
{
  "name": "devhost",
  "ip": "192.168.1.10",
  "user": "root",
  "password_enc": "v1:...",
  "port": 22
}
```

## 加密方案

当前实现采用本机随机密钥文件：

```text
~/.config/sshc/key
```

首次保存带密码的 host 时自动生成 32 字节随机密钥，并以 base64 写入该文件。

密码加密使用：

- AES-256-GCM
- 每次加密生成随机 nonce
- 密文格式：`v1:<base64(nonce+ciphertext)>`

`v1:` 前缀用于后续兼容新格式。

## 安全边界

这个方案解决的是“GitHub 开源工具默认把密码明文写进配置文件”的问题，但它不是 OS keyring 级别的安全模型。

需要明确：

- 攻击者同时拿到 `~/.config/sshc/sshc.config.json` 和 `~/.config/sshc/key` 时，可以解密密码。
- 跨机器迁移密码配置时，需要同时迁移 `sshc.config.json` 和 `key`。
- 删除或丢失 `key` 后，已有 `password_enc` 无法恢复，只能重新添加密码。
- 仍然建议优先使用 SSH key 登录，而不是密码登录。

## 兼容策略

读取规则：

1. 如果存在明文 `password`，直接使用它。
2. 如果 `password` 为空且存在 `password_enc`，使用本机 key 解密。
3. 如果两者都为空，则必须配置 `key_path`。

写入规则：

1. 如果 `Host.Password` 非空，写入前加密为 `password_enc`。
2. 不再写入明文 `password` 字段。
3. 如果仅使用 `key_path`，不会创建本机 password key。

## 后续可选增强

后续如果需要更强安全边界，可以新增 OS keyring 支持：

```json
{
  "password_ref": "sshc/devhost/root"
}
```

建议读取优先级：

1. `password_ref`
2. `password_enc`
3. `password`

OS keyring 能避免密钥和密文都落在同一个配置目录里，但会引入平台差异、Linux 桌面环境依赖和跨机器迁移限制，因此当前版本先采用本机随机密钥文件方案。
