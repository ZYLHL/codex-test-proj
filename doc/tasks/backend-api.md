# backend-api 模块最小任务

- [ ] 搭建 Go `net/http` 路由：`/api/v1/sync/push|pull|full`、`/healthz`。
- [ ] 实现 Bearer Token 鉴权中间件。
- [ ] 实现统一错误响应结构（400/401/500）。
- [ ] 落地 `push/pull/full` 最小处理流程（参数校验 + 调用 service）。
- [ ] 增加 handler 集成测试：鉴权失败、参数错误、成功路径。
