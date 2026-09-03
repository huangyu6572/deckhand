# 11 输出游标

V1 **只**使用 **operation 内事件序号**。作用域 = 该 `job_` 或 `sess_`。不是 daemon 全局游标。

## 事件

`events.jsonl` 是有序事实源。stdout/stderr 派生文件损坏时可由 events 重建。

每条已持久化事件：

| 字段 | 必填 | 说明 |
|------|------|------|
| `type` | 是 | `started` \| `stdout` \| `stderr` \| `data` \| `state` \| `completed`。`heartbeat` **不写**事实日志、**无** `cursor` |
| `operation_id` | 是 | |
| `cursor` | 事实事件必填 | 从 1 起，该 operation 内单调；重启后续增，不重置 |
| `timestamp` | 是 | UTC RFC3339Nano |
| `data` / `data_base64` | 块类型时 | |

`after_cursor=N`：只返回 `cursor > N` 的**完整**事件，不从事件中间切字节。

`max_event_bytes`（默认 256KiB）：更大的块在 **Persist 前**拆成多条事件，各有 cursor。

本次响应累计 payload 受 `max_response_bytes` 限制；下一个事件放不下则本轮停，`next_cursor` = 已返回最后一条的 cursor。

`next_cursor`：本次最后一个返回事件的 cursor；无新事件时等于输入的 `after_cursor`（0 表示还没有任何事件）。

日志被清理导致缺口：`events_lost` > 0，并给 `first_available_cursor`。不再使用 `bytes_lost`。

## JSONL 流（CLI stdout）

与事实事件相同字段；heartbeat 仅线上，无 cursor。

## 真值表（验收）

| 场景 | 行为 |
|------|------|
| 空日志 | `next_cursor=0`，无事件 |
| 分片 | 大块 → 多 cursor，replay 无缺口 |
| 响应截断 | `truncated=true`，磁盘仍完整直至 `max_operation_log_bytes` |
| 日志清理 | 活跃 writer 不删；已删则 `events_lost` |
| replay→live | cursor 不重复、不回退 |
| daemon 重启 | 已 fsync 的 cursor 仍在；未 fsync 不得出现在对外响应 |

耐久：chunk 可批量 flush；`started`、Transport 派发、终态必须 `fsync` 后才能对外确认该 cursor/状态。
