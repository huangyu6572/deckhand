# internal

应用内部模块。外部项目不得直接依赖。

V1 **少包**：用户命令面不变；实现按下面分层，不要再拆回 `target` / `deployment` / `connection` / `audit` / `protocol` 等独立包。

## 分层

1. 入口：`cli`、`wire`。
2. 应用：`config`、`job`、`session`。
3. 契约：`transport/contract`、Repository 接口。
4. 基础设施：`transport/{pool,ssh,serial}`、`storage`、`log`、`secrets`、`platform/windows`。
5. 装配：`app`（由 `cmd/hubd` 调用）。

依赖只能朝契约方向。Adapter 不得泄漏第三方库对象。
