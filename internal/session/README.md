# internal/session

持久 SSH PTY 和串口 REPL 会话模块。

## 动作

- open、attach、detach、write、read、resize 和 close。
- 按游标读取历史及实时返回。
- 跟踪 opening、open、disconnected、closed、failed 状态。
- 关联 Transport 重连后的新数据流。
- 保持 PTY 的 cwd、环境变量和 shell 状态。
- `session exec`：随机 nonce 结束标记，同 Session 串行；exit 仅在收到标记时解析。

- 向 `job` 出租串口 Session（读循环/写锁），**不**为 `serial exec` 再对外分配一套 wait 协议。

## 边界

不伪造串口退出码。不对 vim/top 使用结束标记。SSH PTY 在 hubd 重启后不能恢复原 channel。不执行 Recipe（那是 `job`）。不把环形缓冲当唯一日志。不把 `serial exec` 的终态写成 Session 状态给 CLI。详见 `docs/contracts/13-session-io.md`、`05-serial.md`。
