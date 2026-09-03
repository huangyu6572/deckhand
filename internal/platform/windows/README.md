# internal/platform/windows

Windows 平台适配模块。

## 动作

- 获取当前用户 SID。
- 创建 Named Pipe ACL。
- 启动并约束当前用户 daemon。
- 包装 Credential Manager 和 DPAPI。
- 提供 Windows 路径、原子 rename 和进程终止处理。

## 测试重点

不同用户访问隔离、重复 daemon、长路径、文件占用和退出资源释放。
