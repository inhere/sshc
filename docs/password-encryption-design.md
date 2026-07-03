# sshc password 加密设计边界

## 结论

当前不直接实现 password 加密，原因是 `hosts.json` 的密码加密需要先明确密钥来源。没有可靠密钥来源时，简单 base64、固定密钥、机器内硬编码密钥都只是弱混淆，会让使用者误判安全性。

推荐优先使用 `--key`：

```bash
sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
```

## 可选方案

### OS keyring

把密码存到系统凭据管理器，`hosts.json` 只保存引用。

优点：

- 密码不直接落在项目配置文件中。
- Windows/macOS/Linux 都有可对接方案。

不足：

- 需要引入额外依赖和平台差异处理。
- 备份 `hosts.json` 后不能直接跨机器恢复密码。

### 用户口令派生

用户提供 master password，通过 KDF 派生密钥加密 host password。

优点：

- 配置可跨机器复制。
- 不依赖 OS keyring。

不足：

- 每次使用 password host 都需要输入 master password，或在进程内缓存。
- 需要设计错误次数、缓存时长、迁移和忘记口令后的恢复策略。

### 机器本地密钥文件

首次运行生成本地随机密钥，保存到 `~/.config/sshc/key`，用它加密 `hosts.json` 中的密码。

优点：

- 使用体验简单。
- 实现复杂度较低。

不足：

- 同一机器上拿到密钥文件和 hosts.json 即可解密。
- 跨机器迁移需要额外复制密钥文件。
- 安全边界比 OS keyring 弱。

## 建议实施顺序

1. 继续优先完善 key path 登录。
2. 如果 password 加密成为必要能力，优先选择 OS keyring。
3. 若需要跨机器同步，再单独设计用户口令派生方案。
4. 不实现固定密钥、base64、XOR 等弱混淆。

## 兼容策略

后续实现时建议支持以下字段：

```json
{
  "password": "",
  "password_ref": "sshc/devhost/root",
  "password_enc": ""
}
```

读取规则：

- `password_ref` 优先，表示从 OS keyring 读取。
- `password_enc` 次之，表示按用户口令或本机密钥解密。
- `password` 保留兼容旧配置，但 `add` 新写入时不再默认写明文。

迁移规则：

- 不自动删除旧明文密码。
- 提供显式命令迁移，例如 `sshc config encrypt-passwords`。
- 迁移完成前输出清晰提示，让用户知道当前配置仍包含明文密码。
