# internal/transport/serial

Serial Transport Adapter。

## 动作

- 枚举 COM 及 USB 元数据。
- 按 baud、数据位、停止位、校验和流控独占打开。
- 启动唯一连续原始字节读取循环。
- 串行化事务写入并跨分片执行 matcher。
- 检测拔出，按 USB 标识退避重连。
- 可靠关闭并释放 COM 句柄。

## 规则

不假造退出码、当前目录或文件能力；断线后不重放旧写入。
