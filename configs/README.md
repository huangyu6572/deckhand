# configs

仓库里的示例模板，**不随 `product\` 分发**。程序运行时也不读这个目录。

生效配置在 `%LOCALAPPDATA%\LocalAIHub\`：把示例复制过去再改 host/user/COM，或第一次跑 `hub` 后直接改自动生成的 yaml。不要在 yaml 里写密码。

```text
copy configs\connections.example.yaml %LOCALAPPDATA%\LocalAIHub\connections.yaml
copy configs\settings.example.yaml %LOCALAPPDATA%\LocalAIHub\settings.yaml
mkdir %LOCALAPPDATA%\LocalAIHub\recipes
copy configs\recipes\artifact-service.example.yaml %LOCALAPPDATA%\LocalAIHub\recipes\artifact-service.yaml
```

- `connections.example.yaml`：SSH / COM 目标
- `settings.example.yaml`：内网范围、超时、日志上限
- `recipes/artifact-service.example.yaml`：`hub deploy` 配方

未知 YAML 字段会拒绝解析。发给别人时，同样内容写在 `product\配置说明.md`。
