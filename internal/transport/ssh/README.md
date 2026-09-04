# internal/transport/ssh

SSH Transport Adapter。

## 动作

- 使用 ssh-agent、私钥或受保护凭据认证。私钥目标会继续尝试 `~/.ssh` 默认身份、本机 agent，以及已保存的密码 / keyboard-interactive。`AUTH_FAILED` 带公钥指纹和原始 SSH 错误，不写秘密。
- 自动记录首次 host key，阻断已知指纹变化。
- 建立可复用 SSH Client 并执行 keepalive。
- 创建并关闭 exec、PTY 和 SFTP Channel。
- 分离 stdout/stderr，获取 exit code。
- 支持 PTY resize、SFTP 临时上传、哈希校验和原子 rename。

## 边界

不管理 Job 状态、CLI 输出和审计授权。
