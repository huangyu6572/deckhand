# 接口契约

冲突时本目录优先于 [`../v1-design.md`](../v1-design.md)。数据目录：`%LOCALAPPDATA%\LocalAIHub`。

| 文件 | 内容 |
|------|------|
| [00-envelope.md](00-envelope.md) | argv、退出码、JSON、错误码表 |
| [01-ipc.md](01-ipc.md) | 单连接 Pipe、带 kind 的帧 |
| [02-run.md](02-run.md) | `hub run` |
| [03-cp.md](03-cp.md) | `hub cp` |
| [04-session.md](04-session.md) | session open/exec/read/close |
| [05-serial.md](05-serial.md) | 串口 |
| [06-job.md](06-job.md) | job wait/follow/cancel |
| [07-target.md](07-target.md) | 目标解析 |
| [08-deploy.md](08-deploy.md) | 部署 |
| [09-transport.md](09-transport.md) | 能力 |
| [10-state-machines.md](10-state-machines.md) | 状态与 ok |
| [11-output-cursor.md](11-output-cursor.md) | 事件游标 |
| [12-config-schema.md](12-config-schema.md) | YAML/OpenSSH/DNS |
| [13-session-io.md](13-session-io.md) | write/resize/shell |
| [14-secrets.md](14-secrets.md) | secret set |
| [15-connection.md](15-connection.md) | connection list/status/close |

另见 [`../recovery.md`](../recovery.md)、[`../storage-schema.md`](../storage-schema.md)。

**AI 默认 verb：** `run`、`cp`、`serial exec`、`deploy`，加上未完成任务的 `job wait` / `job follow`。其余为进阶。

**不做**：`hub daemon status`。
