# internal/wire

稳定通信：DTO + Windows Named Pipe（原 protocol + ipc）。

## 动作

- IPC 请求、响应、事件 DTO；JSON/JSONL envelope；错误码与协议版本。
- 当前用户 SID 隔离的 Pipe 与 ACL。
- 带 `kind` 的长度前缀 JSON；**单连接单请求或单流**。
- 慢消费者有界队列，超限断开本连接。

## 边界

不执行业务。字段形状见 `docs/contracts/00-envelope.md` 与 `01-ipc.md`。
