# test

跨模块测试和测试资产目录。

## 子目录

- `contract`：共享 Adapter 契约测试。
- `integration`：真实模块组合测试。
- `e2e`：从 `hub.exe` 到测试目标。
- `fault`：断网、拔线、磁盘满和崩溃。
- `fixtures`：可启动的测试目标和固定配置。
- `testdata`：不可执行输入样本。

模块内部纯逻辑单元测试未来与对应 Go 文件同目录放置。
