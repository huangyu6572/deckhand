# 06 job

后台任务查询。`hub run` / `hub cp` / `hub serial exec` / `hub deploy` 产生的未完成工作都在这里续读，**包括串口 exec**（不要改去 `session wait`）。

| 别名 | 等价 |
|------|------|
| `hub jobs` | `hub job list` |
| `hub logs <id>` | `hub job follow <id>` |
| `hub logs <id> --follow` | 同上 |

## 6.1 job list

`--json` 字段 `jobs`：数组，元素 `id,target,kind,status,started_at,finished_at?`。

## 6.2 job wait

```text
hub job wait [flags] <job-id>
```

阻塞到终态。输出与对应 `run`/`cp`/`serial exec`/`deploy` 的最终 `--json` 相同。已结束则立即返回。未知 id：`JOB_NOT_FOUND`。

测试：W-01 wait 跑完的 job；W-02 wait 期间 CLI 杀，Job 继续；W-03 再 wait 得终态；W-04 未知 id `JOB_NOT_FOUND`。

## 6.3 job follow

```text
hub job follow [flags] <job-id> [--after <cursor>]
```

默认 jsonl。`--after` 默认 0=从磁盘头。先 replay 再 live。

测试：F-01 replay+live 无重复 cursor；F-02 `--json` 不跟流，只返回当前快照（若仍 running 则 `status=running` 无完整 stdout）。

## 6.4 job cancel

关闭本端 channel/等待。`status=cancelled`。**不保证**远端进程退出。重复 cancel 幂等，仍返回终态 `cancelled`。

测试：K-01 sleep 被 cancel 后 wait 为 cancelled；K-02 远端可能仍有 sleep；K-03 两次 cancel 均 ok。

IPC：`Job.List|Wait|Follow|Cancel`。params：`job_id`、`after_cursor`。
