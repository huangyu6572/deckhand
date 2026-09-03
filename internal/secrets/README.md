# internal/secrets

凭据引用解析模块。

## 动作

- 使用 ssh-agent。
- 读取 OpenSSH 私钥路径。
- 解析 Windows Credential Manager/DPAPI 引用。
- 以尽可能短的生命周期提供凭据材料。
- 在错误和日志中隐藏秘密。

CLI 契约见 `docs/contracts/14-secrets.md`。

## 边界

不将秘密正文写入 YAML、SQLite、请求日志或审计。
