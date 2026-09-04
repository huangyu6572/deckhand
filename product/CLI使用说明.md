# CLI 使用说明

`hub.exe` 是唯一入口。`hubd.exe` 会在第一次执行时自动拉起，一般不用手动开。

语法：

```text
hub <动词> [对象] [参数] [--] [发给远端的内容...]
```

**参数必须写在动词后面。** 正确：`hub run --json host -- uname`。错误：`hub --json run host -- uname`（退出码 2）。

`--` 之后原样发给远端，Hub 不会再包一层 `bash -lc`。

在 Windows PowerShell 里不要把复杂脚本塞进 `bash -c "..."`：`$?`、`2>&1`、`&&` 会被本机吃掉。改用本地文件（正文不经过 PowerShell）：

```text
hub run --json --allow-public --script-file .\deploy.sh --shell bash cloud-172
```

`--shell bash` 在 Go 里把脚本编成 `base64 | bash -s` 再发给 Linux；Windows 远端用 `--shell powershell`。简单命令仍用 `hub run --json <目标> -- uname -a`。

**禁止**在 argv 里写 `--password`（退出码 2）。密码用 `hub secret set`。

## 给 AI 的默认命令（优先用这些）

| 干什么 | 命令 |
|--------|------|
| 在 Linux 上执行一条命令 | `hub run --json <目标> -- <命令>` |
| 传一个文件（SFTP） | `hub cp --json <本地文件> <别名:/远端路径>` |
| 串口问一句、等提示符 | `hub serial exec --json COM3 "version" --wait ">"` |
| 按配方发布 | `hub deploy --json <目标> --recipe <名> --artifact <本地文件>` |
| 等后台任务结束 | `hub job wait --json <job_id>` |
| 跟任务输出 | `hub job follow --jsonl <job_id>` |

目标可以是：

- `connections.yaml` 里的名字，如 `dev-web`
- 内网直连 `user@192.168.1.20` 或 `user@192.168.1.20:22`
- 本机已有 OpenSSH 配置里的 **精确 Host 名**
- 串口 `COM3`（大小写不敏感）

### 示例

```text
hub run --json user@192.168.1.20 -- uname -a
hub run --json dev-web -- uname -a
hub run --json --timeout 30s dev-web -- uptime
hub run --json --detach dev-web -- sleep 60
hub job wait --json job_xxxxxxxx

hub cp --json .\app.tar.gz dev-web:/opt/app/app.tar.gz
hub cp --json dev-web:/tmp/out.log .\out.log

hub serial exec --json COM3 "version" --wait ">"
hub serial exec --json board-01 "AT" --wait "OK" --timeout 5s

hub deploy --json test-web --recipe artifact-service --artifact .\app.tar.gz

hub job list --json
hub target list --json
hub serial list --json
```

下载时远端路径必须是 **已有目标名 + 冒号 + 路径**。Windows 本地路径请用 `C:\...`（反斜杠），避免和 `别名:/unix路径` 搞混。

公网地址默认不能连。需要时加上 `--allow-public`，或见配置说明里的 `network.scope`。

命令里可能含秘密时加 `--sensitive`（不把命令正文写入 request 日志）。

## 常用参数

这些参数加在**动词后面**：

| 参数 | 含义 |
|------|------|
| `--json` | 标准输出只有一个 JSON 对象，方便程序/AI 解析 |
| `--jsonl` | 每行一个事件（长输出、follow） |
| `--timeout 30s` | 本次超时，写法与 Go duration 相同：`2s`、`1m` |
| `--allow-public` | 允许这次连公网 |
| `--detach` | 立刻返回任务 ID，任务继续在后台跑 |
| `--quiet` | 文本模式少打印 Hub 自己的话 |
| `--sensitive` | 不把命令正文写入请求日志 |

## 输出和退出码

- `--json`：看字段 `ok`、`status`、`request_id`。失败时有 `error_code`、`message`。
- 不加 `--json`：远端的 stdout/stderr 打到本机对应流。

| 退出码 | 含义 |
|--------|------|
| 0 | 成功 |
| 1 | 业务失败（命令非 0、超时、找不到目标、认证失败等） |
| 2 | 参数或配置写错（含 argv 出现 `--password`） |
| 3 | 找不到或无法启动 `hubd.exe`（检查是否与 `hub.exe` 同目录） |

Ctrl+C 只断开本次查看，**不会**取消后台任务。要停用：`hub job cancel <job_id>`（不保证远端进程一定被杀掉）。

## 其它命令（人用，不是 AI 默认）

```text
hub secret set <目标名>
hub secret delete <目标名>

hub session open <目标> --name optimize
hub session exec optimize -- "pwd"
hub session exec optimize --no-sentinel -- "top"
hub session read optimize --after 0 --wait 2s
hub session logs optimize
hub session write optimize -- hello
hub session write optimize --bytes-b64 aGVsbG8=
hub session attach optimize
hub session detach optimize
hub session close optimize

hub shell <目标>
hub serial monitor COM3
hub serial monitor board-01 --after 0

hub connection list --json
hub connection status --json <id或目标>
hub connection close <id或目标>
```

密码不要写在命令行。`hub secret set` 会无回显读入一行，存进 Windows 凭据管理器。

`hub shell` 需要真正的终端：先 `session open` 再 attach。窗口变化会转发 resize。退出交互：新行后输入 `~.`（与 OpenSSH 类似），或 stdin EOF；**不会** close 那个 session。

`hub serial monitor` 默认 JSONL 无限流；Ctrl+C 只退 CLI，不关串口。一问一答仍用 `serial exec`。

## 常见错误码

| error_code | 处理 |
|------------|------|
| `INVALID_ARGUMENT` | 参数放到动词后；`run` 必须有 `--` 和命令；不要用 `--password` |
| `SCOPE_NOT_INTRANET` | 加 `--allow-public`，或改 settings 里 `network.scope` |
| `AUTH_FAILED` | ssh-agent、私钥路径、或先 `hub secret set` |
| `HOST_KEY_CHANGED` | 机器密钥变了，不会自动连 |
| `TARGET_NOT_FOUND` | 检查别名 / `user@host` / `COM3` |
| `JOB_NOT_FOUND` | `job wait` 的 ID 不对 |
| `SESSION_NOT_FOUND` | session id/name 不对，或 hubd 重启后 SSH PTY 已关闭 |
| `CAPABILITY_UNSUPPORTED` | 例如对 COM 口执行 `hub run`（串口用 `serial exec`） |
| `DAEMON_INSTANCE_CONFLICT` | `hubd.exe` 缺失或未与 `hub.exe` 放一起 |

数据和日志在 `%LOCALAPPDATA%\LocalAIHub`，不是本程序目录。配置怎么写见 [配置说明.md](配置说明.md)。
