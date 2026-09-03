# test/integration

跨模块集成测试入口。

使用临时目录、临时 SQLite、测试 sshd 和虚拟串口。每个测试必须隔离资源并可靠清理。

子目录按 IPC、SSH、Serial、Storage 和 Logging 分类。
