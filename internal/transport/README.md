# internal/transport

远端和设备通信 Adapter 集合。

## 结构

- `contract`：稳定能力接口。
- `registry`：编译期 Adapter 注册。
- `pool`：连接键、租约、keepalive。
- `ssh`：SSH、PTY、SFTP。
- `serial`：COM 长连接、读循环和事务。

新 Transport 必须实现真实能力、映射标准错误、可靠关闭，并通过共享契约测试。
