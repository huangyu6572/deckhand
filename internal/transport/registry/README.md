# internal/transport/registry

Transport 编译期注册模块。

## 动作

- 注册 Transport 名称及 Factory。
- 根据 resolved Target 创建 Adapter。
- 拒绝重复注册和未知类型。
- 暴露已注册类型和能力元数据。

V1 不动态加载 DLL、脚本或网络插件。
