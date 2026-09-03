# 05 serial

本机 COM。`serial exec` 是 **best-effort** 事务：写锁只防 Hub 内交叉写入，异步日志仍可能进入 matcher 窗口。

`line_ending` 固定 `LF`。matcher 窗口最多 `limits.matcher_max_bytes`。Go `regexp` 为 RE2。

## 对外状态：只有 Job

`hub serial exec` 是 **Job 门面**（IPC `Serial.Exec`，实现落在 `job`，内部租用串口 Session）：

- 对外唯一 ID 是 `operation_id` = `job_`。续读、取消、follow **只**用 `hub job wait|follow|cancel`。
- hubd 内部可创建/附着 `kind=serial_repl` 的 Session 以独占端口和读循环；**不**把该 Session 当作这次 exec 的第二套状态机。
- `--json` **不要**要求 AI 处理 `session_id`。若实现附带 `session_id`，仅为诊断可选字段。
- `hub serial monitor` 才是 Session 面（无限流、`sess_`、`--after` 事件游标）。

## 5.1 serial list

`hub serial list [--json]` → `ports` 数组。

## 5.2 serial exec

```text
hub serial exec [flags] <target> <payload>
```

`--wait`：字面量或 `re:` 正则。默认 Target `prompt_pattern`。`--timeout` 默认 5s。`--baud` 仅临时 COM。

内部流程：确保端口 Session → Job `running` → 写锁 → 记事件游标 → 写 payload+LF → 窗口内 matcher → Job 终态。事件写入 **该 Job** 的 `events.jsonl`。

超时：`SERIAL_TIMEOUT`。OS 打开失败：`SERIAL_BUSY`。

测试：

| ID | 期望 |
|----|------|
| S-01 | 匹配 version；仅 `job_`，`job wait` 能拿到同一结果 |
| S-02 | `SERIAL_TIMEOUT` + partial |
| S-03 | 并行 exec 写入串行 |
| S-04 | 拔线 `SERIAL_DISCONNECTED` |
| S-05 | 窗口超过 `matcher_max_bytes` 失败，读循环仍活 |
| S-06 | 二次 exec 与 monitor 可共享内部端口；exec 的 follow 仍走 Job |

## 5.3 serial monitor

```text
hub serial monitor [flags] <target> [--after <cursor>]
```

默认 `--jsonl`。无限流，无 final IPC response。`--after` 从 **monitor 的 Session** 事件续。Ctrl+C 只退 CLI。

测试：M-01 cursor 递增；M-02 杀 CLI 再 monitor `--after` 可续。
