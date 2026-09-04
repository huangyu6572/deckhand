# internal/workspace

远端工作目录约束。

## 动作

- `--workdir` 与 yaml `workspace_root` 合成有效根。
- `hub run` 由 Hub `mkdir -p` 并 `cd` 再执行；`~/dir` 在远端展开为 `$HOME/dir`。
- **查询**（`ls`/`cat`/`find` 等）可以看工作目录以外。
- **写入、移动、删除、`cd` 离开、重定向 `>`** 越出工作目录则 `FILE_OUTSIDE_WORKSPACE`。
- `hub cp` 上传/下载路径仍必须在工作目录内。

## 边界

不是内核沙箱。相对路径在 cd 之后才安全；禁止混用 `~/a` 与 `/home/u/a` 两种写法假装同一目录。
