# 15 connection

查看 hubd 持有的传输连接。不替代 `hub run`。

## hub connection list

`--json` 字段 `connections`：最多 500。元素：`id`、`target_ref`、`transport`、`state`（`connecting|ready|busy|backoff|closing|closed`）、`idle_seconds`。

## hub connection status

```text
hub connection status --json <id-or-target>
```

IPC `Connection.Status`。未知 → `CONNECTION_NOT_FOUND`。

## hub connection close

```text
hub connection close <id-or-target>
```

幂等：已关闭再 close 仍 `ok=true`，`status=closed`。不取消已有 Job（Job 随后会 `REMOTE_UNREACHABLE` / `execution_unknown`）。

IPC：`Connection.List|Status|Close`。

## 测试

| ID | 期望 |
|----|------|
| CN-01 | run 后 list 含该 target |
| CN-02 | 未知 id → `CONNECTION_NOT_FOUND` |
| CN-03 | close 两次均 ok |
