# 01 IPC（hub.exe ↔ hubd.exe）

全部 wire JSON 键 **snake_case**。一次 CLI 进程 = **一条** Named Pipe 连接 = **一个**请求或 **一条**流。V1 不在单连接上复用。

## 1. 传输

- 名称：`\\.\pipe\LocalAIHub-<sid>`
- ACL：当前用户 + `SYSTEM`
- 帧：`uint32` LE 长度 + UTF-8 JSON；`n==0` 或 `n>pipe_max_frame_bytes` 则断开
- `v` 必须为 `1`，否则 `IPC_PROTOCOL_ERROR` 或 payload `INVALID_ARGUMENT`

写 Pipe：有超时；订阅队列有界，超限断开**本连接**，Job 继续。重复 `id`：返回原结果，不新建副作用。

## 2. 帧（显式 kind）

```json
{"v":1,"kind":"request","id":"req_…","method":"Job.Run","params":{}}
{"v":1,"kind":"response","id":"req_…","ok":true,"payload":{}}
{"v":1,"kind":"event","id":"req_…","event":{}}
```

`payload` 与该命令 `--json` 对象同构。`event` 与 JSONL 事实事件同构（heartbeat 无 cursor）。

无法解码 kind → 断开，`IPC_PROTOCOL_ERROR`。

## 3. 流形

**有终态的流**（`Job.Follow`、`jsonl` 的 Run/Exec/Deploy/Copy）：

1. `response`（`status=running`，`stream=true`）
2. 零或多条 `event`
3. **必须**再一条最终 `response`（完整终态 payload）

**无限流**（`Serial.Monitor`、`Session.Attach` / `hub shell`）：只有 running `response` + `event`s，**无** final response；客户端断开 = 取消订阅（Monitor 不关口；Attach = detach）。

客户端断开本 Pipe：**只**取消该订阅，不 `Job.Cancel`。

## 4. 方法

| method | CLI | 流形 |
|--------|-----|------|
| `Target.List` | `hub target list` | 一次 |
| `Job.Run` | `hub run` | 一次或有终态流 |
| `Job.Wait` | `hub job wait` | 一次 |
| `Job.Follow` | `hub job follow` | 有终态流 |
| `Job.Cancel` | `hub job cancel` | 一次 |
| `Job.List` | `hub job list` | 一次 |
| `File.Copy` | `hub cp` | 一次或有终态流 |
| `Session.Open` | `hub session open` | 一次 |
| `Session.Exec` | `hub session exec` | 一次或有终态流 |
| `Session.Read` | `hub session read` | 一次 |
| `Session.Write` | `hub session write` | 一次 |
| `Session.Resize` | `hub session resize` | 一次 |
| `Session.Attach` | `hub session attach` / `hub shell` | 无限流 |
| `Session.Detach` | `hub session detach` | 一次 |
| `Session.Leave` | `hub session leave` | 一次或有终态流 |
| `Serial.List` | `hub serial list` | 一次 |
| `Serial.Exec` | `hub serial exec` | 一次（**Job 门面**；内部用串口 Session，对外只产生 `job_`） |
| `Serial.Monitor` | `hub serial monitor` | 无限流 |
| `Deploy.Start` | `hub deploy` | 一次或有终态流 |
| `Connection.List` | `hub connection list` | 一次 |
| `Connection.Status` | `hub connection status` | 一次 |
| `Connection.Close` | `hub connection close` | 一次 |
| `Secret.Set` | `hub secret set` | 一次 |
| `Secret.Delete` | `hub secret delete` | 一次 |

无 `Target.Resolve` 对外方法（解析在各方法内部）。无 `Daemon.Status`。

## 5. 单实例 bootstrap

1. CLI **先连 Pipe**，成功则发 `request`。
2. 失败：获取短生命周期 mutex `Local\LocalAIHub-bootstrap-<sid>`；获得者用 `hub.exe` 同目录绝对路径启动 `hubd.exe`（禁止 PATH）；其余 CLI 等待 Pipe（总 10s）。
3. `hubd` 持有终身 mutex `Local\LocalAIHub-hubd-<sid>` 并打开 `%LOCALAPPDATA%\LocalAIHub\runtime\hubd.lock`（记录 pid、启动时间、exe 路径、协议版本）。**陈旧 lock 文件不是活实例**。
4. 启动进程提前退出：CLI 退出 3，stderr 含 hubd 原因。
5. 父进程高完整性：不拉起 hubd；连不上 → `DAEMON_INSTANCE_CONFLICT`。
6. CLI/daemon `v` 不匹配 → 明确升级提示，不半执行。

## 6. 测试

| ID | 期望 |
|----|------|
| I-01 | 当前用户往返 |
| I-02 | 其他用户连失败 |
| I-03 | 超长帧断开 |
| I-04 | `v=2` 不执行 |
| I-05 | 双 CLI 无 hubd → 一个实例 |
| I-06 | follow 中杀 CLI，Job 仍 running |
| I-07 | payload 与 `--json` 同构 |
| I-08 | 无 kind 的 JSON 断开 |
| I-09 | Monitor 无 final response |
