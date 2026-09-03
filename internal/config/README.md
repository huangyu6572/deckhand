# internal/config

连接设置与 Target 解析（含原 target）。

## 动作

- 读取校验 `connections.yaml`、`settings.yaml`；拒绝未知字段。
- OpenSSH Config 子集；不支持的关键字立即失败。
- 解析持久别名、内网 `user@host`、OpenSSH alias、`COMx`。
- 按 `network.scope` 拒绝默认公网直连；DNS 全地址必须过 scope。
- 原子 rename 写配置；连接参数不热重载到已建立的 SSH。

## 边界

不建连，不读秘密正文，不把明文密码写入配置文件。
