# cmd/hub

`hub.exe` 入口。

## 动作

- 创建 CLI 根命令。
- 调用 daemon 自动启动流程。
- 经 `wire` 建立 Named Pipe 客户端。
- 将最终状态映射为稳定进程退出码。
- 响应 Ctrl+C 并取消当前订阅，不默认取消后台 Job。

## 边界

不直接连接 SSH、COM、SQLite 或日志文件。
