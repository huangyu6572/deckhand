# 08 hub deploy

一条命令走完 Recipe。目标必须 SSH。子步骤 **不** 对外创建子 Job，只在父 `job_` 上写 `state` 事件。崩溃后 **不自动续跑**（见 [../recovery.md](../recovery.md)）。

回滚 **只执行 Recipe 里写的 rollback 命令**，不承诺通用备份恢复。

```text
hub deploy [flags] <target> --recipe <name> --artifact <local-file>
```

## Recipe

路径：`%LOCALAPPDATA%\LocalAIHub\recipes\<name>.yaml`。未知字段拒绝。快照 `recipe_hash` 写入 DB，运行中改文件不影响本次。

```yaml
name: artifact-service
precheck:
  - "test -d /opt/app"
upload:
  remote_dir: /opt/app
  remote_name: app.tar.gz
apply:
  - "tar -xzf /opt/app/app.tar.gz -C /opt/app"
  - "systemctl restart myapp"
verify:
  type: command
  command: "systemctl is-active myapp"
  timeout: 30s
rollback:
  - "systemctl restart myapp"
```

V1 **`verify.type` 只允许 `command`**，且 **始终在 SSH 目标内** `Job.Run` 语义执行。不从 Windows 发起 HTTP/TCP。遇错即停。全局 `--timeout`（默认 30m）包整次部署；步骤 `verify.timeout` 只约束 verify 命令。

命令字符串按原样发送，无占位符展开（artifact 路径写死在 Recipe 或与 `upload.remote_dir/name` 一致）。

`--idempotency-key`：键 `(target_identity, recipe, key)`。指纹含 recipe_hash + artifact sha。同 key 不同指纹 → `IDEMPOTENCY_CONFLICT`。同指纹且已 succeeded → 返回原 `job_`，不二次 apply。并发同 key：等待或复用，禁止双 apply。

## 输出

`operation_id`（`job_`）、`kind=deploy`、`current_step`、`deploy_status`（见 10）、`artifact_sha256`。Job `status` 用 Job 枚举。

成功：Job `succeeded`，`deploy_status=succeeded`，`ok=true`。  
verify 失败且 rollback 成功：`ok=false`，`HEALTHCHECK_FAILED`，`deploy_status=rolled_back`。  
rollback 失败：`ROLLBACK_FAILED`，`deploy_status=rollback_failed`。

## 测试

| ID | 期望 |
|----|------|
| D-01 | 成功 |
| D-02 | verify 失败 → rolled_back |
| D-03 | 缺文件 `RECIPE_NOT_FOUND` |
| D-04 | 同指纹不重复 apply |
| D-05 | 同 key 不同 artifact `IDEMPOTENCY_CONFLICT` |
| D-06 | 串口 `CAPABILITY_UNSUPPORTED` |
| D-07 | 上传失败不 apply |
| D-08 | rollback 命令失败 → `rollback_failed` |
