# test/fault

故障注入测试。

覆盖 SSH 断网、串口拔线、磁盘满、Output Store 写失败、SQLite 锁定、daemon 强杀、重复启动、高速输出和系统时间跳变。

禁止出现伪成功、无日志执行、不安全重放或资源泄漏。
