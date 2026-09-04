# internal/job

一次性/后台任务，以及 Recipe 部署（含原 deployment）。

## 动作

- 创建 Job 和稳定 Operation ID；排队、等待、取消、超时、唯一终态。
- CLI 断开后继续后台执行。
- 输出交给 `log`，元数据交给 `storage`。
- `Serial.Exec`：本包拥有 Job；内部向 `session` 租用串口，不把 Session 暴露给该次 CLI。
- Recipe：precheck、上传、apply、远端 command verify、可选 rollback、幂等指纹。
- 失败时 JSON `message` 使用 completed 事件里的说明（例如 AUTH_FAILED 的公钥指纹），不只回错误码。
- `rm` 无真人键盘确认则 `DESTROY_NEEDS_HUMAN`。配方不得含 rm。
- `--workdir` / `workspace_root`：先 cd 再执行；查询其他目录可以，写入/删除越界则 `FILE_OUTSIDE_WORKSPACE`。

## 边界

不维持交互 PTY。不向 AI 返回 job+session 双 ID。不提供通用脚本语言，不引入 Approval。不保证远端进程被杀。
