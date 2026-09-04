# docs

- [`install.md`](install.md)：GitHub 一键安装与发版。
- 给别人下载的 AI Skill：仓库 [`skills/deckhand/`](../skills/deckhand/)。`npx skills add huangyu6572/deckhand -g -y`，或 `scripts/install-skill.ps1`。
- `v1-design.md`：产品与架构。
- `usage.md`：源码仓库内的安装与命令说明。
- 对外拷贝目录：仓库根目录 `product/`（README、CLI使用说明、配置说明；exe 由 `scripts\build.ps1` 生成）。示例 yaml 在仓库 `configs/`。
- 编译：[`编译.md`](编译.md)。
- `contracts/`：**唯一**字段级接口与测试。
- `recovery.md`：崩溃恢复表。
- `storage-schema.md`：SQLite。
- `implementation-readiness-review.md`：落地审阅原文。
