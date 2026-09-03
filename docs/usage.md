# 使用说明

本机 AI 用 `hub.exe` 操作内网 Linux（SSH）和本机串口（COM）。远端只需已有 sshd 或 COM 口，**不要装插件**。

把 `hub.exe` 和 `hubd.exe` 放在**同一目录**。第一次执行会自动：

1. 创建 `%LOCALAPPDATA%\LocalAIHub`
2. 写出默认 `settings.yaml` / `connections.yaml`
3. 拉起当前用户的 `hubd.exe`（单实例，一般不用管）

## 1. 构建

需要 Go 1.24+（PATH 里有 `go`，常见路径 `C:\Program Files\Go\bin`）。对外目录是 `product/`：

```text
powershell -File product\build.ps1
```

只编译、不跑测试。若要先自检：

```text
go test ./...
go build -o hub.exe ./cmd/hub
go build -o hubd.exe ./cmd/hubd
```

给本地 AI 用时：允许它执行 `product\hub.exe`（与 `hubd.exe` 同目录）。

## 2. 最少配置

可以**先不改配置**，直接连内网：

```text
hub run --json user@192.168.1.20 -- uname -a
hub serial exec --json COM3 "version" --wait ">"
```

认证用本机 **OpenSSH ssh-agent** 或已有 `%USERPROFILE%\.ssh\config` 别名。私钥口令和密码不要写进 YAML。

常用目标再写进 `%LOCALAPPDATA%\LocalAIHub\connections.yaml`（可从仓库 `configs/connections.example.yaml` 复制）：

```yaml
targets:
  dev-web:
    transport: ssh
    host: 192.168.1.20
    user: devops
    auth:
      type: ssh-agent

  board-01:
    transport: serial
    port: COM3
    baud_rate: 115200
    prompt_pattern: ">"
```

密码认证：`auth.type: password`，先执行一次 `hub secret set <target>`（无回显读入，写入 Windows 凭据管理器）。不要用 `--password`。

可选：`settings.yaml`（仓库 `configs/settings.example.yaml`）。默认 `network.scope: intranet`，内网第一次 SSH 自动记录 host key。

部署 Recipe 放到 `%LOCALAPPDATA%\LocalAIHub\recipes\<name>.yaml`（示例：`configs/recipes/artifact-service.example.yaml`）。

## 3. AI 默认命令

全局 flag **必须写在动词后面**：`hub run --json …` 可以；`hub --json run …` 会退出 2。

`--` 之后全部发给远端，Hub 不再包一层 `bash -lc`。

| 场景 | 命令 |
|------|------|
| 维护 / 排错 | `hub run --json <target> -- <cmd>` |
| 传文件 | `hub cp --json <本地> <别名:/远端>` 或反过来下载 |
| 串口一问一答 | `hub serial exec --json COM3 "version" --wait ">"` |
| 发布 | `hub deploy --json <target> --recipe <name> --artifact <文件>` |
| 长任务续读 | `hub job wait --json <job_id>` / `hub job follow --jsonl <job_id>` |

例子：

```text
hub run --json dev-web -- uname -a
hub run --json --timeout 2s dev-web -- sleep 30
hub run --json --detach dev-web -- sleep 60
hub cp --json .\app.tar.gz dev-web:/opt/app/app.tar.gz
hub serial exec --json board-01 "version" --wait ">"
hub deploy --json test-web --recipe artifact-service --artifact .\app.tar.gz
hub job wait --json job_01
hub job list --json
hub target list --json
```

公网地址默认拒绝。要出网时加 `--allow-public`，或在 `settings.yaml` 把 `network.scope` 改成 `all`。

## 4. 输出与退出码

- `--json`：stdout 一个 JSON 对象（`ok`、`status`、`request_id`，失败时有 `error_code`）。
- `--jsonl`：每行一个事件，适合跟长输出。
- 文本模式：远端 stdout / stderr 原样打到本机对应流。

| 退出码 | 含义 |
|--------|------|
| 0 | 成功（`ok=true`） |
| 1 | 业务失败（远端非 0、超时、找不到目标等） |
| 2 | 本地参数或配置错误 |
| 3 | 连不上 `hubd` / Pipe |

Ctrl+C 只取消本次订阅，**不会**取消后台 Job。要停任务用 `hub job cancel <id>`（不保证远端进程被杀掉）。

## 5. 进阶（人用，不是 AI 默认）

```text
hub session open dev-web --name optimize
hub session exec optimize -- "cd /opt/app && ./verify.sh"
hub serial list --json
hub connection list --json
hub secret set dev-web
```

`hub shell` 需要真实 TTY（open + attach，窗口变化会 resize；`Enter ~ .` detach）。一问一答请用 `serial exec`；持续流用 `hub serial monitor`。

## 6. 常见失败

| error_code | 怎么办 |
|------------|--------|
| `INVALID_ARGUMENT` | flag 放到动词后面；`run` 必须有 `-- 命令` |
| `SCOPE_NOT_INTRANET` | 加 `--allow-public`，或改 `network.scope` |
| `AUTH_FAILED` | 检查 ssh-agent / 私钥路径 / `hub secret set` |
| `HOST_KEY_CHANGED` | 主机密钥变了，不会自动连；确认后再改 known_hosts |
| `TARGET_NOT_FOUND` | 检查别名、`user@host`、或 `COM3` 写法 |
| `JOB_NOT_FOUND` | `job wait` 的 id 不对 |
| `CAPABILITY_UNSUPPORTED` | 例如对 COM 口执行 `hub run` |
| `DAEMON_INSTANCE_CONFLICT` | `hubd.exe` 不在 `hub.exe` 同目录，或权限/完整性级别不匹配 |

数据与日志：`%LOCALAPPDATA%\LocalAIHub`（配置、SQLite、`logs/operations/`）。字段级契约：[`contracts/`](contracts/README.md)。

## 7. 本机自检

不连真实 SSH 也可以先确认 CLI 与 daemon：

```text
go test ./...
hub --json run t -- true
hub run --json user@8.8.8.8 -- true
hub job wait --json job_nope
hub target list --json
```

预期：第一条退出 2（flag 必须在动词后）；第二条 `SCOPE_NOT_INTRANET`；第三条 `JOB_NOT_FOUND`；第四条 `ok=true`。连内网主机的 `hub run` 需要本机 ssh-agent 和可达的 sshd。
