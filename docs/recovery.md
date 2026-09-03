# Daemon 崩溃与传输故障恢复

SSH exec / SSH PTY 在 `hubd` 进程死后 **不能** 重附着原 channel。串口可按设备身份尝试重开同一 Session 流。

## 重启扫描

| 崩溃前 | 重启后 |
|--------|--------|
| Job `queued` 且 `dispatched_at` 为空 | 可重新排队 |
| Job `queued` 且已 `dispatched_at` | `execution_unknown`，不重放 |
| SSH exec `running` | `execution_unknown`，不重放 |
| cp `running` | `failed`；清理/保留 `.hubtmp-*`，不自动 rename |
| SSH PTY `open`/`opening` | `disconnected` 然后 `closed`；保留历史 events |
| Serial Session `open` | 按 USB 序列号或 COM 名重开，插入 `state=disconnected` 再 `open` 事件，**不**重发上次 payload |
| Serial transact `running` | Job `failed` 或 `execution_unknown`，不重发 |
| Deployment apply/verify/rollback | Job `execution_unknown`；**V1 不自动续跑** Recipe |

## 提交顺序

1. 创建日志文件与 `request.json`（无秘密正文）
2. SQLite 插入 `queued`
3. `dispatched_at` + fsync events `started`
4. 副作用（SSH/串口）
5. 终态：fsync `completed` 事件 → CAS 更新 SQLite 终态

崩溃夹在 4 与 5 之间 → `execution_unknown`。

## 其它故障

| 故障 | 行为 |
|------|------|
| SSH 执行中断网 | 已有 events 保留；`execution_unknown` |
| 串口事务中拔线 | 解锁；部分 events 落盘；不重发 |
| events 写入失败 | 不发布未持久化 cursor；操作 `OUTPUT_PERSIST_FAILED` |
| 磁盘满 / 超 `max_operation_log_bytes` | 本端停止等待；`OUTPUT_LIMIT_EXCEEDED`；远端可能仍在跑 |
| 重复 `request_id` | 返回原 operation，不新建 |
