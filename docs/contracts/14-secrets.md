# 14 Secrets

禁止 `--password`。禁止把密码放 argv、环境变量、`request.json` 正文、events、审计正文。

## hub secret set

```text
hub secret set [flags] <target>
```

从控制台 **无回显** 读一行（非 JSON stdin）。写入 Windows Credential Manager，名 `LocalAIHub/<target>`。

IPC：`Secret.Set` 的 `secret` 字段仅在内存中存在；hubd **不得**写入 request 日志。落盘仅 `secret_kind=password`、`length`。

**唯一允许 CLI 直打平台 API 而不经业务 IPC 的路径**：无。V1 仍走 IPC，由 hubd 调 Credential Manager，避免 CLI/hubd 两套存储。

输出 `--json`：`ok=true`，`target`，无秘密。

## hub secret delete

```text
hub secret delete <target>
```

删除 cred。测试：DEL-01 删除后 password 认证 `AUTH_FAILED`。

## 使用

`connections.yaml` `auth.type: password` 时 hubd 用 `cred:<target>` 取密。`hub run` 的远端命令默认可能含秘密；`--sensitive` 时 `command`/`payload` 不写入 request 正文（只记长度）。

## 测试

| ID | 场景 | 期望 |
|----|------|------|
| SEC-01 | `hub secret set t` 交互输入 | Credential Manager 有项；日志无密码 |
| SEC-02 | `hub run --json t -- echo leaked` 无 --sensitive | request 可含命令（V1 接受风险） |
| SEC-03 | `--sensitive` | request.json 无命令正文 |
| SEC-04 | argv 含疑似 `--password` | 退出 2，不启动认证 |
