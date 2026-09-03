# internal/transport/pool

长连接池（原 `internal/connection`）。

## 动作

- 按连接键建立或复用连接。
- 租约、keepalive、并发 Channel、退避重连、空闲回收。
- `connection` CLI：list / status / close。

CLI/IPC 见 `docs/contracts/15-connection.md`。连接恢复不重放已发送的非幂等写入。
