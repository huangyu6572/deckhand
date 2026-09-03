# test/contract

共享契约测试。

## 套件

- Transport 生命周期、Capability、超时、取消、输出、断线、并发和重连。
- Repository 事务、迁移、幂等和并发。
- Output append、flush、replay、cursor、rotation 和写入失败。

每个 Adapter 使用同一测试套件，避免复制断言。
