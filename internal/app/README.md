# internal/app

daemon 装配与生命周期（原 bootstrap）。

## 动作

- 创建数据目录并加载配置。
- 以 mutex + lockfile 保证单实例。
- 执行数据库迁移。
- 创建 Repository、log、Transport Registry 和服务。
- 启动 Named Pipe。
- 恢复未结束 Job/Session 元数据。
- 按依赖逆序关闭。

## 边界

不解析 argv，不实现 SSH/Serial 业务。
