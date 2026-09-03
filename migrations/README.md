# migrations

SQLite schema migration 目录。

## 规则

- 文件按严格递增版本命名。
- 每个 migration 在事务内执行。
- 已发布 migration 不修改，只新增。
- migration 必须支持从上一发布版本升级测试。
- 不存放运行日志或秘密正文。

当前 schema：`0001_init.sql`（version 1）。见 `docs/storage-schema.md`。
