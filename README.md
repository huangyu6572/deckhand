# Deckhand

本机 AI 通过 `hub.exe` 操作内网 SSH 与本机串口。远端只需已有 sshd 或 COM，不必装插件。

**发给别人请用 `product/` 整个目录**（内含 exe、CLI 说明、配置模板、配置说明与编译脚本）。

```text
powershell -File product\build.ps1
```

说明见 `product\编译.md`。源码侧说明：[docs/usage.md](docs/usage.md)。契约：[docs/contracts/](docs/contracts/)。
