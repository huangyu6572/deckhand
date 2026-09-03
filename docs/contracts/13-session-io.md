# 13 Session I/O 与 hub shell

SSH PTY 交互。可靠 exit code 仍只来自 `hub run`。`session exec` 在约束成立时**可解析** exit，不宣称可靠。细节补充 [04-session.md](04-session.md)。

无 TTY 时 `hub shell` 失败（退出 2），提示改用 `session open` + `exec`/`write`/`read`。

**不做** `hub daemon status`。

## Sentinel（session exec）

每次生成 128-bit nonce。写入**一条** sh 复合命令，例如：

```text
{ <user-command>
printf '\n__HUB_DONE_<nonce>_%s__\n' "$?"; }
```

只匹配含该 nonce 的标记。同 Session 的 `exec` **严格串行**。已有 raw `write` 未结束或 TUI 活跃 → `SESSION_BUSY`。只支持 POSIX `sh` 兼容交互 shell。原始 PTY 全量落盘；JSON `stdout` 是去掉回显与标记后的区间视图。超时后迟到的旧标记不得被下次 exec 消费（nonce 不同）。

`--no-sentinel`：无 exit_code。

## session write

```text
hub session write [flags] <session> -- <data>
```

或 `--bytes-b64 <b64>`。IPC `Session.Write`：`session_id`，`data` 或 `data_base64`。

输出：`ok=true`，`status=open`，`bytes_written`。不表示远端已处理完。

测试：WRI-01 写入后 `read` 见到回显或设备输出；WRI-02 未知 id → `SESSION_NOT_FOUND`。

## session resize

```text
hub session resize <session> --cols <n> --rows <n>
```

IPC `Session.Resize`。测试：RSZ-01 合法窗口；RSZ-02 非正数 `INVALID_ARGUMENT`。

## session attach / detach

- `hub session attach <session>`：当前终端接到已 open 的 PTY（需 TTY）。Ctrl+C 默认发给远端。Detach：`Enter ~ .`（与 OpenSSH 类似，文档写死）。stdin EOF：detach，不 close。
- `hub session detach`：对**本 CLI attach 连接**发 detach；无 attach 则 `INVALID_ARGUMENT`。
- IPC：`Session.Attach` 为无限流（无 final response），直到客户端断开；断开 = detach，Session 保持 `open`。

## session logs

别名：`hub session logs <session>` = `hub session read <session> --after 0`（可加 `--jsonl` 重放 events）。

## hub shell

```text
hub shell [flags] <target>
```

等价：`session open`（无名则生成）+ `attach`。窗口 SIGWINCH → resize。无 TTY：失败。`--name` 可选。

测试：SH-01 有 TTY 冒烟（人工/PTY 夹具）；SH-02 无 TTY 退出 2。
