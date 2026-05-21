# sqlite-store 模块最小任务

- [ ] 创建 `encrypted_notes`、`idempotency_log`、`device_state` 表结构迁移。
- [ ] 实现密文 upsert（写入 `server_updated_at` 与递增 `server_seq`）。
- [ ] 实现按 cursor 增量读取。
- [ ] 实现幂等键命中返回历史结果。
- [ ] 增加集成测试：重复 push 不重复写入；pull 游标准确推进。
