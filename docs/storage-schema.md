# SQLite 存储

Target 配置真值源是 YAML（`connections.yaml`）。库只存 operation 当时的 `resolved_target_snapshot`（JSON 文本）。

时间：UTC，INTEGER Unix 微秒（`*_us` 列）。

DDL：[migrations/0001_init.sql](../migrations/0001_init.sql)

## 事务

- 单条状态 CAS：`UPDATE … WHERE id=? AND version=?`，rows!=1 则冲突重读。
- 终态不可逆：`finished_at IS NULL` 才允许进入终态。
- 幂等：`INSERT` 唯一键 `(target_identity, recipe_name, idempotency_key)`；已存在则比较 `request_fingerprint`，不同 → `IDEMPOTENCY_CONFLICT`。

## Repository

`internal/storage`：Get/Put operation、session、deployment、idempotency；不写 stdout 正文。

## 测试

| ID | 场景 |
|----|------|
| DB-01 | 空库 apply `0001_init.sql` 成功 |
| DB-02 | 重复 `request_id` 第二次插入失败或返回原行 |
| DB-03 | 幂等 key 相同指纹不同 → 应用层 `IDEMPOTENCY_CONFLICT` |
| DB-04 | CAS 旧 version 更新 0 行 |
