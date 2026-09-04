# 00 公共信封

领域状态、ID、`ok` 算法见 [10-state-machines.md](10-state-machines.md)。游标见 [11-output-cursor.md](11-output-cursor.md)。配置目录 `%LOCALAPPDATA%\LocalAIHub`。

AI 默认只需：`run`、`cp`、`serial exec`、`deploy`，以及 `job wait` / `job follow`。

## 1. 命令语法

```text
hub <verb> [object] [flags] [--] [payload...]
```

- 全局 flag **只能出现在 verb 之后**。
- `--` 之后全部进入 payload。
- `--json` 与 `--jsonl` 同时出现时以 `--jsonl` 为准。

全局 flag：

| Flag | 类型 | 默认 | 说明 |
|------|------|------|------|
| `--json` | bool | false | stdout 仅一个 JSON 对象 |
| `--jsonl` | bool | false | stdout 每行一个事件 |
| `--quiet` | bool | false | 文本模式少打印 |
| `--timeout` | duration | Target/settings | 如 `30s` |
| `--allow-public` | bool | false | 允许本次公网 |
| `--detach` | bool | false | 立即返回 ID |
| `--sensitive` | bool | false | 命令/payload 不写入 request 正文 |
| `--workdir` | string | 空 | 远端工作目录。Hub 先 cd。查询其他目录可以；写入/删除/`cd` 离开不得越出 |

Duration：Go `time.ParseDuration`。非法 → 退出 2，`INVALID_ARGUMENT`。

## 2. 进程退出码

| 码 | 何时 |
|----|------|
| 0 | `ok=true` |
| 1 | 业务失败 |
| 2 | 本地参数/配置错误 |
| 3 | daemon/Pipe 不可用 |

`--json`：诊断只进 stderr。

## 3. `--json` 对象

| 字段 | 类型 | 必有 | 说明 |
|------|------|------|------|
| `ok` | bool | 是 | 见 10-state-machines |
| `request_id` | string | 是 | `req_` |
| `operation_id` | string | 否 | `job_` 或 `sess_` |
| `target` | string | 否 | |
| `status` | string | 是 | **该对象的领域状态** |
| `error_code` | string | `ok=false` 时是 | |
| `message` | string | 失败时是 | |
| `retryable` | bool | 失败时是 | |
| `truncated` | bool | 否 | 响应或列表截断 |
| `log_path` | string | 否 | |

命令特有字段同顶层追加。响应体受 `max_response_bytes` 限制，UTF-8 边界截断。

## 4. `--jsonl`

见 11-output-cursor。CLI 流事件与事实事件同形；heartbeat 无 cursor。

## 5. 错误码注册表

| error_code | 退出码 | retryable | 含义 |
|------------|--------|-----------|------|
| `INVALID_ARGUMENT` | 2 | false | argv 不合法 |
| `CONFIG_INVALID` | 2 | false | YAML 未知字段/类型 |
| `RECIPE_INVALID` | 2 | false | Recipe 不合法 |
| `RECIPE_NOT_FOUND` | 2 | false | |
| `TARGET_NOT_FOUND` | 1 | false | |
| `JOB_NOT_FOUND` | 1 | false | |
| `SESSION_NOT_FOUND` | 1 | false | |
| `CONNECTION_NOT_FOUND` | 1 | false | |
| `TARGET_DISABLED` | 1 | false | |
| `SCOPE_NOT_INTRANET` | 1 | false | |
| `OPENSSH_UNSUPPORTED` | 2 | false | 关键 ssh_config 不支持 |
| `DESTROY_NEEDS_HUMAN` | 2 | false | `rm` 需真人 TTY 输入 `DELETE <target>`；无 `--yes` |
| `CAPABILITY_UNSUPPORTED` | 1 | false | |
| `HOST_KEY_CHANGED` | 1 | false | |
| `AUTH_FAILED` | 1 | false | |
| `REMOTE_UNREACHABLE` | 1 | true | |
| `REMOTE_EXIT_NONZERO` | 1 | false | SSH 退出码非 0 |
| `JOB_TIMEOUT` | 1 | true | |
| `JOB_CANCELLED` | 1 | false | 本端已取消 |
| `EXECUTION_UNKNOWN` | 1 | true | |
| `SESSION_BUSY` | 1 | true | PTY 上另有 exec/write |
| `OUTPUT_LIMIT_EXCEEDED` | 1 | false | 磁盘/operation 日志上限 |
| `OUTPUT_PERSIST_FAILED` | 1 | true | |
| `LOCAL_IO_ERROR` | 1 | true | |
| `STORAGE_ERROR` | 1 | true | |
| `IPC_PROTOCOL_ERROR` | 3 | false | |
| `SERIAL_BUSY` | 1 | true | OS 打开失败 |
| `SERIAL_DISCONNECTED` | 1 | true | |
| `SERIAL_TIMEOUT` | 1 | true | |
| `FILE_OUTSIDE_WORKSPACE` | 1 | false | 尽力拦截，非沙箱 |
| `FILE_CHANGED` | 1 | true | |
| `DAEMON_INSTANCE_CONFLICT` | 3 | true | |
| `HEALTHCHECK_FAILED` | 1 | false | |
| `ROLLBACK_FAILED` | 1 | false | |
| `IDEMPOTENCY_CONFLICT` | 1 | false | 同 key 不同指纹 |
| `OUTPUT_TRUNCATED` | 0 | false | 仅响应截断且命令成功时；此时 `ok=true` 且 `truncated=true` |

删除：`SERIAL_REGEX_TIMEOUT`（RE2，改用 matcher 窗口）。

## 6. 公共测试

| ID | 前置 | 输入 | 期望 |
|----|------|------|------|
| E-01 | 无 | `hub --json run t -- true` | 退出 2，`INVALID_ARGUMENT` |
| E-02 | 内网 | `hub run --json t -- --timeout 1` | `--timeout` 进远端 |
| E-03 | 超大 stdout | 有 `max_response_bytes` | UTF-8 合法，`truncated=true` |
| E-04 | `--json --jsonl` | `hub job follow --json --jsonl j1` | stdout 为 JSONL |
| E-05 | 诊断 | `hub run --json nosuch` | stdout 可解析 |
| E-06 | `false` | `hub run --json t -- false` | `REMOTE_EXIT_NONZERO` |
