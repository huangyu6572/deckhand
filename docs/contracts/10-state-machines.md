# 10 状态机与 ID

公共 JSON 的 `status` **取该 operation 的领域状态**，不是一张混用枚举。见 [recovery.md](../recovery.md) 崩溃表。

## ID

| 前缀 | 对象 | 说明 |
|------|------|------|
| `req_` | 一次 IPC/CLI 请求 | `operations.request_id` 唯一 |
| `job_` | Job（含 deploy） | Deployment **不**另发 `dep_` |
| `sess_` | Session | PTY 或串口内部会话 |

`operation_id`：Job 命令填 `job_…`；Session 命令填 `sess_…`。

## `ok`

- **变更类**（run/cp/exec/deploy/cancel/close/write…）：仅当领域终态为 `succeeded`（Session 关闭成功为 `closed` 且本次动作为 close 时 `ok=true`，见 13）才 `ok=true`。
- **查询类**（list/wait 快照/read/status/follow 当前快照）：查询本身成功则 `ok=true`，即使被查询 Job 仍为 `running`。被查询对象状态放在 `status` 或列表元素里。
- 远端 `exit_code != 0`：`ok=false`，`status=failed`，`error_code=REMOTE_EXIT_NONZERO`。

## Job

`queued → running → {succeeded, failed, timed_out, cancelled, execution_unknown}`

终态不可逆。合法边只允许进入上列终态之一。`version` 乐观锁，CAS 写终态。

## Session

`opening → open → {disconnected, closed, failed}`  
`disconnected → open`（仅串口按设备身份重连成功）  
`disconnected → closed`（SSH PTY 重启后必须关闭，不可重附着原 channel）

## Deployment（附属 Job，`kind=deploy`）

Job.`status` 仍用 Job 枚举。步骤在 `current_step` 与 `deploy_status`：

`running | verifying | rolling_back | succeeded | rolled_back | failed | rollback_failed`

- 验证失败且 rollback 命令成功：Job `failed`，`deploy_status=rolled_back`，`error_code=HEALTHCHECK_FAILED`，`ok=false`
- rollback 命令也失败：Job `failed`，`deploy_status=rollback_failed`，`error_code=ROLLBACK_FAILED`

## 测试

| ID | 断言 |
|----|------|
| SM-01 | Job 不能从 `succeeded` 回到 `running` |
| SM-02 | 同一 `version` 并发写终态只有一次成功 |
| SM-03 | `hub run --json t -- false` → `REMOTE_EXIT_NONZERO`，`exit_code=1`，`ok=false` |
