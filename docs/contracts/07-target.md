# 07 target

用户日常：在 `%LOCALAPPDATA%\LocalAIHub\connections.yaml` 写 SSH/COM。解析与 DNS 见 [12-config-schema.md](12-config-schema.md)。

## 解析顺序

1. `connections.yaml` 里 `targets.<name>`
2. OpenSSH `%USERPROFILE%\.ssh\config` 的 `Host` 精确匹配（支持子集，见总稿 §8）
3. `user@host` 或 `user@host:port`（host 为 IP 或主机名）
4. `COM\d+`（大小写不敏感）或 `/dev/tty*`（V1 Windows 只保证 `COMn`）

冲突（同名既是 yaml 又是 ssh Host）：**yaml 优先**，stderr 提示一次。

`network.scope=intranet` 时，**解析得到的全部地址**须为内网，见 12。yaml 里写了公网 host：每次仍要 `--allow-public` 或 `scope: all`。

## hub target list

`--json` 字段 `targets`：持久目标 + `ephemeral=false`。不含一次性直连。

可选 `hub target add`：改 yaml 的向导，V1 可用交互或：

```text
hub target add --json --name dev-web --ssh user@192.168.1.20 --auth agent
```

非实现阻塞项；M1 可用手改 yaml。

## 测试

| ID | 场景 | 期望 |
|----|------|------|
| T-01 | yaml `dev-web` | run 用名成功 |
| T-02 | 仅 ssh config Host `pi` | `hub run --json pi -- true` 成功 |
| T-03 | yaml 与 ssh 同名 | 走 yaml |
| T-04 | `Match` 块 Host | `OPENSSH_UNSUPPORTED` |
| T-08 | 仅 ServerAliveInterval | 可连 |
| T-05 | `COM3` | serial exec 可解析 |
| T-06 | `hub target list --json` | 含 yaml 名，不含刚才的 `user@ip` 临时 |
| T-07 | yaml 公网 + 无 flag | `SCOPE_NOT_INTRANET` |
