# internal/transport/contract

Transport 核心契约。

## 能力

- 基础：Connect、Capabilities、Close。
- Exec：命令、输出和可选退出码。
- Stream：Read、Write、Subscribe。
- Transact：写入后按 matcher 等待。
- File：Upload、Download、Stat、List。
- Resize：调整 PTY。
- Reconnect：声明可重连语义。

接口只使用领域对象、字节和标准事件，不暴露第三方库类型。
