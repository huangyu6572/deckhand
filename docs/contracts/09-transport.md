# 09 Transport 能力

实现层接口（Go），不是 AI 调用面。CLI 见 02–08。

## 能力

| 能力 | SSH | Serial |
|------|-----|--------|
| exec | 是 | 否（用 transact） |
| interactive / stream | 是 | 是 |
| transact | 否 | 是 |
| files | 是 | 否 |
| resize | 是 | 否 |
| exit_code | 是 | 否 |
| reconnect | 是 | 是（序列号规则见总稿 §9.4） |

未声明能力 → `CAPABILITY_UNSUPPORTED`，不得打到设备。

## 连接键

SSH：`user|host|port|auth_ref|host_key_fp|jump_chain`（无跳板则 jump 为空）  
拨号使用 DNS 钉死的 IP，键仍含原 hostname。

## 错误映射（实现必须稳定）

| 底层 | error_code |
|------|------------|
| ssh 认证失败 | `AUTH_FAILED` |
| 拨号/超时 | `REMOTE_UNREACHABLE` |
| host key mismatch | `HOST_KEY_CHANGED` |
| 串口 open 失败 | `SERIAL_BUSY` |
| 读循环设备消失 | `SERIAL_DISCONNECTED` |

## 契约测试（每个 Adapter）

见总稿 §15.3 `C-TRN-*`。另：C-KEY-01 不同 user 不得复用同一 `ssh.Client`。
