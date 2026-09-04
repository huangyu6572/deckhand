# internal/cli

CLI 命令与输出适配模块。

## 动作

- 解析命令。**默认面** `run`/`cp`/`serial exec`/`deploy`/`job wait|follow`；进阶 `session`/`shell`/`monitor`/`connection`/`secret`。
- 无 TTY 时拒绝 `hub shell`。
- `rm` 无真人 TTY 确认则拒绝；没有 `--yes`。
- `--workdir` 交给 daemon 做 cd 与越界检查。
- `secret set` 无回显读入后走 IPC，不把秘密写入 argv。
- 自动发现或启动 `hubd`。
- 经 `wire` 发送请求并订阅返回事件。
- 渲染文本、单个 JSON 或 JSONL 流。
- 将错误映射为稳定退出码。

命令 argv 与 JSON 见 `docs/contracts/00-envelope.md` 及 `02-run.md` 等。

## 规则

`--` 后参数原样传给远端。JSON stdout 不得混入诊断文本。
