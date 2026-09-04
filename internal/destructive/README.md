# internal/destructive

检测远端命令/脚本里的 `rm` 一类删除。

## 动作

- 识别 `rm` / `rmdir` / `unlink` / `Remove-Item` / `find -delete` / `xargs rm`，包括 hub `--shell bash` 的 base64 包装。
- 没有真人确认则返回 `DESTROY_NEEDS_HUMAN`。没有 `--yes`。

## 边界

不是通用审批流。不拦截人在 `hub shell` 里亲手敲的键。配方里的 rm 直接拒绝，不能确认后执行。
