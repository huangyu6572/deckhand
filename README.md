# Deckhand

[![CI](https://github.com/huangyu6572/deckhand/actions/workflows/ci.yml/badge.svg)](https://github.com/huangyu6572/deckhand/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/huangyu6572/deckhand?include_prereleases&label=release)](https://github.com/huangyu6572/deckhand/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/huangyu6572/deckhand/total)](https://github.com/huangyu6572/deckhand/releases/latest)
[![Platform](https://img.shields.io/badge/Windows-amd64-0078D6?logo=windows&logoColor=white)](https://github.com/huangyu6572/deckhand/releases/latest)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev/dl/)

本机 AI / 终端通过 **`hub.exe`** 操作内网 Linux（SSH）和本机串口（COM）。远端只需已有 `sshd` 或 COM 口，**不要装插件**。

```mermaid
flowchart LR
  AI["本地 AI / 终端"] --> hub["hub.exe"]
  hub --> hubd["hubd.exe 后台"]
  hubd --> SSH["内网 Linux SSH"]
  hubd --> COM["本机串口 COM"]
```

| 你要做的 | 对方机器要做的 |
|----------|----------------|
| 在 Windows 上装 `hub.exe` + `hubd.exe` | 已有 sshd，或插着串口设备 |
| 可选：写几个常用主机短名 | **什么都不用装** |

---

## 一键安装（Windows）

在 **PowerShell** 里整行粘贴：

```powershell
irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install.ps1 | iex
```

脚本会：

1. 下载 [最新 Release](https://github.com/huangyu6572/deckhand/releases/latest) 里的 zip（没有 Release 且本机有 Go 时，会改从源码编译）
2. 解压到 `%LOCALAPPDATA%\Programs\Deckhand\`
3. 把该目录加入**当前用户 PATH**
4. 第一次执行 `hub` 时自动创建 `%LOCALAPPDATA%\LocalAIHub\` 并拉起 `hubd.exe`

指定安装目录、强制从源码编译、或不改 PATH（一行一个，装前先设）：

```powershell
$env:DECKHAND_PREFIX = "D:\Tools\Deckhand"
$env:DECKHAND_FROM_SOURCE = "1"
$env:DECKHAND_NO_PATH = "1"
irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install.ps1 | iex
```

装好后**新开一个终端**再敲 `hub`。

也可以在 [Releases](https://github.com/huangyu6572/deckhand/releases/latest) 手动下载 `deckhand-windows-amd64.zip`，或拷贝仓库里的 [`product/`](product/) 整个目录。

安装细节：[docs/install.md](docs/install.md)

---

## 60 秒上手

打开**新的**终端：

```text
hub run --json user@192.168.1.20 -- uname -a
hub serial exec --json COM3 "version" --wait ">"
hub target list --json
```

认证用本机 **OpenSSH ssh-agent**，或已有 `%USERPROFILE%\.ssh\config` 里的 **精确 Host 名**。也可以先不写配置文件。

常用机器再写进 `%LOCALAPPDATA%\LocalAIHub\connections.yaml`（不要写密码）：

```yaml
targets:
  dev-web:
    transport: ssh
    host: 192.168.1.20
    user: devops
    auth:
      type: ssh-agent
```

密码认证：yaml 里 `auth.type: password`，再执行一次 `hub secret set dev-web`。配置说明：[product/配置说明.md](product/配置说明.md)

---

## 常用命令

**参数必须写在动词后面。** `hub run --json …` 可以；`hub --json run …` 会退出 2。`--` 之后原样发给远端。

| 场景 | 命令 |
|------|------|
| 维护 / 排错 | `hub run --json <目标> -- <命令>` |
| 传文件 | `hub cp --json <本地> <别名:/远端>` |
| 串口一问一答 | `hub serial exec --json COM3 "version" --wait ">"` |
| 按配方发布 | `hub deploy --json <目标> --recipe <名> --artifact <文件>` |
| 等后台任务 | `hub job wait --json <job_id>` |

```text
hub run --json dev-web -- uname -a
hub cp --json .\app.tar.gz dev-web:/opt/app/app.tar.gz
hub serial exec --json COM3 "version" --wait ">"
hub job list --json
```

公网地址默认拒绝，需要时加 `--allow-public`。完整命令表：[product/CLI使用说明.md](product/CLI使用说明.md)

---

## 给本地 AI

在 Cursor / 其它本机 AI 里，把安装目录中的 **`hub.exe`** 设为允许执行的命令（与 `hubd.exe` 必须同目录）。优先让模型用上表四条默认命令，并带 `--json`。

---

## 从源码编译

需要 [Go 1.24+](https://go.dev/dl/)（PATH 里有 `go`）。在仓库根目录：

```text
git clone https://github.com/huangyu6572/deckhand.git
cd deckhand
powershell -File scripts\build.ps1
```

产物在 [`product/`](product/)：`hub.exe`、`hubd.exe` 以及使用说明。不要只拷其中一个 exe。

打 Release 包（zip）：

```text
powershell -File scripts\pack-release.ps1
```

---

## 文档

| 文档 | 给谁看 |
|------|--------|
| [一键安装](docs/install.md) | 从 GitHub 拉下来装 |
| [发给别人的目录说明](product/README.md) | 拷贝 `product/` 的人 |
| [CLI 使用说明](product/CLI使用说明.md) | 命令、参数、退出码 |
| [配置说明](product/配置说明.md) | `connections.yaml` / Recipe |
| [仓库内使用说明](docs/usage.md) | 源码侧安装与命令 |
| [编译](docs/编译.md) | 本机构建 |
| [接口契约](docs/contracts/) | 字段级输入/输出（实现以它为准） |
| [产品与架构](docs/v1-design.md) | 设计总稿 |

示例 yaml 在 [`configs/`](configs/)，程序运行时**不读**这个目录；生效配置在 `%LOCALAPPDATA%\LocalAIHub\`。
