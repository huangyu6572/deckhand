# Deckhand（发给别人用这个目录）

把**本文件夹整份**拷贝或打成 zip 即可，不必附带源码。

从 GitHub 装（推荐，不必拷这个目录）：

```powershell
irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install.ps1 | iex
```

发布包：[Releases](https://github.com/huangyu6572/deckhand/releases/latest)。安装说明在仓库 [docs/install.md](../docs/install.md)。

## 目录里有什么

| 文件 | 作用 |
|------|------|
| `hub.exe` | 唯一命令入口，给人或本地 AI 用 |
| `hubd.exe` | 后台进程，必须和 `hub.exe` **放在同一目录** |
| [CLI使用说明.md](CLI使用说明.md) | 命令、参数、退出码 |
| [配置说明.md](配置说明.md) | 配置放哪、怎么改（含示例 yaml） |

不要只拷 `hub.exe`。缺 `hubd.exe` 时命令会失败。

## 配置怎么用

程序**不读**本文件夹里的任何 yaml。真正生效的是：

```text
%LOCALAPPDATA%\LocalAIHub\
```

1. 先跑一条命令（如 `hub target list --json`），会自动创建该目录和默认 yaml。
2. 按 [配置说明.md](配置说明.md) 改 `%LOCALAPPDATA%\LocalAIHub\connections.yaml`（host、user、COM 口）。**不要写密码**。
3. 也可以不写 yaml：直接 `hub run --json user@192.168.1.20 -- uname -a`。

有源码时，仓库根目录 `configs\` 里是同样的示例，可复制到数据目录；发给别人的 `product\` 里不再带模板文件。

## 对方机器上怎么开始

1. 解压到任意目录，例如 `C:\Tools\Deckhand\`。
2. （推荐）跑仓库一键安装，会把该目录写入用户 PATH，并设置 `DECKHAND_HOME`；或手动加入 PATH / 用全路径调用 `hub.exe`。
3. 打开终端执行：

```text
hub run --json user@192.168.1.20 -- uname -a
```

第一次运行会自动创建 `%LOCALAPPDATA%\LocalAIHub` 并启动 `hubd.exe`。远端只要已有 sshd 或 COM，**不要装插件**。

给本地 AI 用时：一键安装会把 Skill 装进 Cursor；把 `hub.exe` 设为允许执行即可。只补装 Skill：`npx skills add huangyu6572/deckhand -g -y`。命令表见 [CLI使用说明.md](CLI使用说明.md)。
