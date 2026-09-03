# internal/platform

操作系统能力抽象目录。

V1 仅包含 Windows 实现。领域模块通过小接口使用 SID、IPC ACL、进程和原子文件操作，避免直接散布平台调用。

## 子目录

- `windows`：Windows 具体适配。
