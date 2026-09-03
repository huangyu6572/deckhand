# internal/storage

领域元数据持久化模块。

## 动作

- 不提供 Target 配置表；Target 真值源为 YAML。
- 实现 SQLite transaction、查询和 `0001_init.sql`。
- 在启动时按 `docs/recovery.md` 扫描未结束记录。

## 边界

不保存大段输出和秘密正文；大输出由 `log` 管理。
