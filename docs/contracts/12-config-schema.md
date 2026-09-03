# 12 配置字段表

数据根目录：**`%LOCALAPPDATA%\LocalAIHub`**（配置、db、日志、runtime 全部在此，不漫游）。

未知 YAML 字段：**拒绝**（`CONFIG_INVALID`），防止拼写静默走默认。重复 key：解析失败。不展开环境变量；`~` 仅在 `IdentityFile`/`key_path` 按用户主目录展开。相对路径相对数据根。不热重载连接参数：改 yaml 后对新 operation 生效；已建立的 SSH 连接空闲超时后按新配置再建。`auth` 规范化为 `auth_ref`：`agent` | `key:<abs-path>` | `cred:<target-name>`。

列表类命令默认最多 **500** 条，超出截断并 `truncated=true`。

## settings.yaml

| 字段 | 类型 | 默认 |
|------|------|------|
| `network.scope` | `intranet` \| `all` | `intranet` |
| `ssh.host_key` | `accept-new-intranet` | 同左 |
| `daemon.idle_exit` | duration 秒，0=不退出 | `0` |
| `limits.max_response_bytes` | int | 8388608 |
| `limits.max_event_bytes` | int | 262144 |
| `limits.max_operation_log_bytes` | int | 104857600 |
| `limits.max_total_log_bytes` | int | 1073741824 |
| `limits.max_concurrent_jobs` | int | 32 |
| `limits.matcher_max_bytes` | int | 65536 |
| `limits.pipe_max_frame_bytes` | int | 16777216 |
| `log.retain_days` | int | 30 |
| `connection.idle_timeout` | duration | `30m` |

废弃：`max_output_bytes`、`matcher_regex_timeout`（Go regexp 为 RE2）。

## connections.yaml

`targets.<name>`：`name` = `[A-Za-z][A-Za-z0-9_-]{0,62}`。

SSH：`transport: ssh`，`host`，`user`，`port` 默认 22，`auth.type` = `ssh-agent` \| `private-key` \| `password`（password 必须已 `hub secret set`），`workspace_root` 可选。

Serial：`transport: serial`，`port`，`baud_rate` 默认 115200，`line_ending` 固定 **`LF`**（不可选 auto），`prompt_pattern` 可选，`reconnect` 默认 true。

## Recipe

见 [08-deploy.md](08-deploy.md)。未知字段拒绝。`verify` V1 仅 `type: command`（在 SSH 目标内执行）。

## 合并优先级（SSH 字段）

CLI `--allow-public` > 本次请求 > connections.yaml > OpenSSH 支持子集 > 默认值。

## OpenSSH（`%USERPROFILE%\.ssh\config`）

- V1 **精确** `Host` 名，不做通配。
- **连接关键**且不支持则整 Host 失败 `OPENSSH_UNSUPPORTED`：`Match`、多跳 `ProxyJump`、`ProxyCommand`。
- **忽略并 stderr 告警**：`ServerAliveInterval`、`ServerAliveCountMax`、`LogLevel`、`ForwardAgent`、`Compression`、`UserKnownHostsFile` 等非拨号关键项（Hub 用自己的 known_hosts）。
- 支持：`HostName`、`User`、`Port`、`IdentityFile`（多值取第一条）、`IdentitiesOnly`、单跳 `ProxyJump`、一层 `Include`（文件不存在=失败）。
- `IdentityFile`：`~`、`%h/%p/%r` 按 OpenSSH 规则展开。
- Jump 与最终目标分别做 DNS scope 与 host key。

## DNS 与内网

先解析。**所有**候选地址必须通过 scope（intranet 时不得混入公网）。选用其中一条内网地址并 **钉死该 IP 拨号**。记录 hostname、IP、判定。短主机名本身不能证明内网。IPv4-mapped IPv6 按 v4 判定。解析超时 → `REMOTE_UNREACHABLE`。

## 测试

| ID | 场景 | 期望 |
|----|------|------|
| CFG-01 | yaml 多未知字段 | `CONFIG_INVALID` |
| CFG-02 | Host 仅有 ServerAliveInterval | 可连，stderr 告警 |
| CFG-03 | 短名解析到公网 A 记录 | `SCOPE_NOT_INTRANET` |
| CFG-04 | 双栈一公一私 | 失败，不回退公网 |
