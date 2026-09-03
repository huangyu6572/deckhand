# 04 session

持久 SSH PTY。write/resize/attach/shell/sentinel 的字段级说明以 [13-session-io.md](13-session-io.md) 为准。

串口不要用 session 命令，用 `serial monitor`/`exec`。

## 4.1 session open

```text
hub session open [flags] <target> [--name <id>]
```

`--name` 可选。同名且 `open|disconnected`：返回现有 `sess_`，不建第二条 PTY。

输出：`operation_id`（`sess_`）、`status=open`、`kind=ssh_pty`。

测试：O-01 新开；O-02 同名复用；O-03 串口目标 `CAPABILITY_UNSUPPORTED`。

## 4.2 session exec

见 13 的 nonce sentinel。同 PTY 串行；busy → `SESSION_BUSY`。

```text
hub session exec [flags] <session> -- <command...>
```

`--no-sentinel`：无可靠 exit。超时无标记：`JOB_TIMEOUT`，不得假 `exit_code=0`。

`false`：`ok=false`，`REMOTE_EXIT_NONZERO` 或解析到的非零码。

未知 session：`SESSION_NOT_FOUND`（不是 `INVALID_ARGUMENT`）。

测试：X-01 cwd 保留；X-02 false；X-03 超时；X-04 `SESSION_NOT_FOUND`；X-05 jsonl；X-06 伪造无 nonce 的 `__HUB_DONE_0__` 不结束；X-07 并发 exec → `SESSION_BUSY`。

## 4.3 session read

```text
hub session read [flags] <session> --after <cursor> --wait <duration>
```

`--after` 为事件序号（见 11）。输出：拼接的 `data`（本次返回的 stdout/data 事件）、`next_cursor`、`events_lost`、`first_available_cursor`、`eof`。

测试：磁盘补齐 `events_lost=0`；日志删除 `events_lost>0`。

## 4.4 close / I/O

`session close`：`status=closed`，幂等。write/resize/attach/detach/logs/shell → [13-session-io.md](13-session-io.md)。
