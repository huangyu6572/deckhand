# cmd/hubd

`hubd.exe` 入口。

## 动作

- 调用 `app` 完成依赖装配。
- 启动当前用户唯一 daemon 实例。
- 接收 Windows 退出信号。
- 按顺序停止 Pipe、Job、Session、连接池和存储。

## 边界

不包含业务状态机和 Transport 细节。
