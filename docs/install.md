# 从 GitHub 安装

仓库：<https://github.com/huangyu6572/deckhand>

只支持 **Windows amd64**。装的是 `hub.exe` + `hubd.exe`（必须放在同一目录）。

## 一键安装

PowerShell：

```powershell
irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install.ps1 | iex
```

默认安装到 `%LOCALAPPDATA%\Programs\Deckhand\`，并：

- 把该目录写入**当前用户 PATH**（`hub` / `hubd` 可直接敲）
- 写入用户环境变量 **`DECKHAND_HOME`**（等于安装目录）
- 安装 AI Skill 到 `%USERPROFILE%\.cursor\skills\deckhand\`（以及 `.agents` / `.copilot`）

装完请**新开终端**再执行 `hub`。PATH 和用户环境变量对当前窗口不一定生效。

| 环境变量 | 作用 |
|----------|------|
| `DECKHAND_PREFIX` | 安装目录 |
| `DECKHAND_VERSION` | Release 标签，默认 `latest` |
| `DECKHAND_FROM_SOURCE` | `1`：不下载 zip，拉源码并用本机 Go 编译 |
| `DECKHAND_NO_PATH` | `1`：不改 PATH（仍会写 `DECKHAND_HOME`） |
| `DECKHAND_NO_SKILL` | `1`：不装 AI Skill |
| `DECKHAND_MIRROR` | GitHub 加速前缀，如 `https://ghfast.top`（会优先于内置镜像） |
| `DECKHAND_ZIP` | 已下载的 `deckhand-windows-amd64.zip` 本地路径，跳过网络下载 |
| `DECKHAND_REPO` | 默认 `huangyu6572/deckhand` |

```powershell
$env:DECKHAND_PREFIX = "D:\Tools\Deckhand"
$env:DECKHAND_FROM_SOURCE = "1"
$env:DECKHAND_NO_PATH = "1"
$env:DECKHAND_NO_SKILL = "1"
$env:DECKHAND_MIRROR = "https://ghfast.top"
irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install.ps1 | iex
```

已克隆仓库时：

```text
powershell -File scripts\install.ps1
powershell -File scripts\install.ps1 -Prefix D:\Tools\Deckhand -FromSource
```

脚本先找 [GitHub Release](https://github.com/huangyu6572/deckhand/releases/latest) 里的 `deckhand-windows-amd64.zip`。还没有 Release 时，若本机有 Go 1.24+，会下载 `main` 源码并编译。

## 手动下载

命令行下 GitHub zip 若卡住：用浏览器打开 [Releases](https://github.com/huangyu6572/deckhand/releases/latest)，下载 `deckhand-windows-amd64.zip`，再：

```powershell
$env:DECKHAND_ZIP = "$env:USERPROFILE\Downloads\deckhand-windows-amd64.zip"
irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install.ps1 | iex
```

这会解压 zip、写入 PATH / `DECKHAND_HOME`、安装 Skill，与一键安装后半段相同。

也可以不解压脚本、自己解压 zip 后把该目录加入 PATH（两个 exe 不要拆开）。

也可以拷贝仓库里编好的 [`product/`](../product/) 整个目录（需先 `powershell -File scripts\build.ps1`）。

## 装好之后

```text
hub run --json user@192.168.1.20 -- uname -a
hub target list --json
```

配置在 `%LOCALAPPDATA%\LocalAIHub\`，不是安装目录。不要在 yaml 里写密码；需要时用 `hub secret set <名字>`。

一键安装默认已经装好 Skill。只补装 Skill：

```powershell
npx skills add huangyu6572/deckhand -g -y
```

或不装 Node：`irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install-skill.ps1 | iex`

命令：[../product/CLI使用说明.md](../product/CLI使用说明.md)。配置：[../product/配置说明.md](../product/配置说明.md)。Skill：[../skills/deckhand/SKILL.md](../skills/deckhand/SKILL.md)。

## 维护者如何发一版

推一个 `v*` 标签，或在 GitHub Actions 里手动跑 **release** 工作流并填 `v0.1.0` 这类版本号：

```text
git tag v0.1.0
git push origin v0.1.0
```

会编译 Windows 包，并上传 `deckhand-windows-amd64.zip` 与 `install.ps1`。
