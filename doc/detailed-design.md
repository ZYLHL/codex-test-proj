# 自托管可安装 PWA 在线便笺详细设计（detailed-design）

> 本文基于 `doc/proposal.md`，用于指导可执行开发。目标是保证模块高内聚、低耦合，模块可独立测试与替换。

---

## 1. 设计目标与约束

## 1.1 设计目标
- 面向 MVP 到 v1.0 的可演进实现。
- 模块边界清晰：每个模块可独立单测、可通过 mock 进行集成测试。
- 前端采用 **VueJS**，后端采用 **Go net/http 标准库优先**。
- 离线优先 + 前端加密 + 自托管同步。

## 1.2 已确认约束（来自需求）
- 核心优先功能：标签、全文搜索、置顶/颜色。
- 单主密码模型；遗忘密码不可恢复。
- 密钥派生：PBKDF2；内容加密：AES-GCM。
- 冲突策略：LWW（以服务端接收时间为准）。
- 同步触发：手动 + 自动。
- 服务端存储：SQLite（`modernc.org/sqlite`）。
- 文档需提供：模块化设计、API JSON 示例、cursor 规则、幂等规则、参数默认值、配置清单、模块/集成/E2E 测试方案、开发任务拆分。

---

## 2. 总体架构

## 2.1 逻辑分层
- **UI 层（Vue 组件）**：渲染与交互，不直接访问加密和网络。
- **应用服务层（前端）**：用例编排（创建便笺、同步、解锁）。
- **领域层（前端）**：Note 规则、排序、标签筛选、查询条件。
- **基础设施层（前端）**：IndexedDB、Web Crypto、HTTP Client、Service Worker。
- **后端 API 层（Go net/http）**：路由、鉴权、参数校验、错误映射。
- **后端仓储层（SQLite）**：密文记录、设备游标、幂等日志。

## 2.2 模块独立性原则
- 模块间仅通过接口依赖，不跨层调用内部实现。
- 每个模块有独立测试入口和 mock 契约。
- I/O（DB、网络、浏览器 API）统一包裹为 adapter，便于替身测试。

---

## 3. 模块设计

## 3.1 前端应用壳层模块（App Shell）

### 职责
- 应用启动、全局状态初始化、主题与布局。
- 页面级路由状态（便签墙、设置、同步状态）。
- 解锁态控制（未解锁仅展示解锁页）。

### 输入/输出
- 输入：用户操作事件、网络在线状态、Service Worker 消息。
- 输出：调用应用服务层（NoteService/SyncService/AuthService）。

### 关键接口（前端内部）
- `bootstrapApp(): Promise<void>`
- `setUnlocked(masterPassword: string): Promise<void>`
- `onOnlineStatusChanged(online: boolean): void`

### 可独立测试点
- 未解锁状态下禁止进入便笺编辑。
- 在线状态变化触发自动同步调度，不直接发请求（委托 SyncService）。

---

## 3.2 便笺领域模块（CRUD、标签、置顶、颜色）

### 职责
- 便笺创建、更新、删除、恢复。
- 标签增删、置顶、颜色变更。
- 排序规则（置顶优先，次级按更新时间）。

### 领域模型（明文）
```ts
interface Note {
  id: string
  title: string
  content: string
  tags: string[]
  color: 'yellow' | 'green' | 'blue' | 'pink' | 'purple' | 'gray'
  pinned: boolean
  createdAt: string // ISO8601
  updatedAt: string // ISO8601
  deleted: boolean
}
```

### 关键接口
- `createNote(input): Note`
- `updateNote(noteId, patch): Note`
- `deleteNote(noteId): Note`
- `listNotes(query): Note[]`

### 独立测试
- 创建后必含 `id/createdAt/updatedAt`。
- `pinned=true` 的排序优先级高于更新时间。
- 标签筛选正确处理多标签 AND/OR（MVP 默认 AND，参数可配置）。

---

## 3.3 全文搜索模块

### 职责
- 提供标题+正文本地搜索。
- 支持标签组合过滤后的二次搜索。

### 设计
- MVP 使用本地倒排索引简化实现：
  - 文本分词（中英文基础规则）
  - 索引存储于 IndexedDB `search_index` store
- 更新策略：便笺变更时增量更新索引。

### 关键接口
- `indexNote(note: Note): Promise<void>`
- `removeNoteIndex(noteId: string): Promise<void>`
- `search(keyword: string, filters): Promise<string[]> // noteIds`

### 独立测试
- 索引增量更新后检索结果与当前明文一致。
- 删除便笺后不可命中旧索引。

---

## 3.4 本地存储模块（IndexedDB）

### 职责
- 管理本地数据库 schema、迁移与事务。
- 提供 repository 接口给上层。

### Store 设计
- `notes_plain`（可选，仅解锁会话缓存）
- `notes_encrypted`
- `sync_queue`
- `search_index`
- `meta`（salt、kdfParams、lastCursor、deviceId）

### 关键接口
- `saveEncryptedNote(record): Promise<void>`
- `loadEncryptedNotes(): Promise<EncryptedNoteRecord[]>`
- `enqueueSync(op): Promise<void>`
- `pullSyncQueue(limit): Promise<SyncOp[]>`

### 独立测试
- 事务回滚一致性。
- schema 升级不丢数据。

---

## 3.5 加密模块（PBKDF2 + AES-GCM）

### 职责
- 主密码派生主密钥。
- 便笺明文加/解密。
- 参数版本管理。

### 默认参数（MVP 固定值）
- KDF: `PBKDF2-HMAC-SHA256`
- iterations: **600000**
- salt length: **16 bytes**
- derived key length: **32 bytes**
- cipher: `AES-256-GCM`
- nonce/iv length: **12 bytes**
- tag length: **128 bits**

### 数据格式
```json
{
  "kdf": {"name":"PBKDF2","hash":"SHA-256","iterations":600000,"salt":"base64"},
  "enc": {"alg":"AES-GCM","iv":"base64","ct":"base64","tagBits":128},
  "v": 1
}
```

### 关键接口
- `deriveKey(masterPassword, salt, params): Promise<CryptoKey>`
- `encryptNote(note, key): Promise<EncryptedBlob>`
- `decryptNote(blob, key): Promise<Note>`

### 独立测试
- 错误密码解密必失败。
- 同明文不同 iv 输出不同密文。
- 参数版本不匹配时拒绝解密并提示迁移。

---

## 3.6 同步模块（Push/Pull、队列、LWW）

### 职责
- 管理离线变更队列与同步编排。
- 执行 push/pull 流程与冲突裁决。

### 同步触发
- 手动：用户点击同步按钮。
- 自动：应用启动、网络恢复、固定轮询（默认 60s，在线且解锁态）。

### LWW 规则（已确认：服务端时间）
- 服务端接收写入时写入 `serverUpdatedAt`。
- 冲突比较 `serverUpdatedAt`，较新者生效。
- 相同时间戳按 `recordId` 字典序兜底。

### 幂等规则
- 每个变更生成 `idempotencyKey = sha256(deviceId + localOpId + noteId + opType)`。
- 服务端在 `idempotency_log` 表保留近 7 天键值。
- 重复提交返回上次处理结果（200 + `deduplicated=true`）。

### cursor 规则
- cursor 采用不透明 token：`base64("v1:<last_server_seq>")`
- 服务端返回 `nextCursor`，客户端仅存储与回传，不解析业务字段。

### 关键接口
- `runSyncOnce(): Promise<SyncReport>`
- `pushChanges(queue): Promise<PushResult>`
- `pullChanges(cursor): Promise<PullResult>`

### 独立测试
- 幂等重试不产生重复写入。
- 断网恢复后队列可继续消费。
- 冲突按服务端时间一致收敛。

---

## 3.7 PWA 模块（Manifest + Service Worker）

### 职责
- 可安装能力与离线缓存策略。
- 静态资源更新控制。

### 缓存策略
- App Shell（HTML/CSS/JS）：`stale-while-revalidate`
- API 请求：默认 `network-first`，失败回退本地缓存（仅只读路径）
- 离线页面：`/offline.html`

### 关键接口/事件
- `self.addEventListener('install'|'activate'|'fetch')`
- 与主线程消息通道：`SKIP_WAITING`、`CACHE_VERSION_READY`

### 独立测试
- 首次安装后可离线打开。
- 发布新版本后可提示刷新并激活新 SW。

---

## 3.8 后端 API 模块（Go net/http）

### 路由
- `POST /api/v1/sync/push`
- `GET /api/v1/sync/pull?since=<cursor>`
- `POST /api/v1/sync/full`
- `GET /healthz`

### 统一错误码
- `400 Bad Request` 参数错误
- `401 Unauthorized` token 无效
- `409 Conflict` 写入版本冲突（仅调试场景，MVP 一般收敛于 LWW）
- `500 Internal Server Error` 服务异常

### 鉴权
- MVP: `Authorization: Bearer <API_TOKEN>`

### API 示例

#### 1) Push
`POST /api/v1/sync/push`

请求：
```json
{
  "deviceId": "dev-01",
  "ops": [
    {
      "idempotencyKey": "6d7f...",
      "noteId": "n-1001",
      "opType": "upsert",
      "payload": {
        "cipherText": "base64...",
        "nonce": "base64...",
        "aad": "base64...",
        "clientUpdatedAt": "2026-05-21T08:00:00Z",
        "deleted": false
      }
    }
  ]
}
```

响应：
```json
{
  "accepted": [
    {
      "noteId": "n-1001",
      "serverUpdatedAt": "2026-05-21T08:00:02Z",
      "deduplicated": false
    }
  ],
  "rejected": []
}
```

#### 2) Pull
`GET /api/v1/sync/pull?since=djE6MTAw`

响应：
```json
{
  "changes": [
    {
      "noteId": "n-1001",
      "cipherText": "base64...",
      "nonce": "base64...",
      "aad": "base64...",
      "deleted": false,
      "serverUpdatedAt": "2026-05-21T08:00:02Z"
    }
  ],
  "nextCursor": "djE6MTAx"
}
```

#### 3) Full
`POST /api/v1/sync/full`

请求：
```json
{"deviceId":"dev-02"}
```

响应：
```json
{
  "snapshot": [
    {
      "noteId": "n-1001",
      "cipherText": "base64...",
      "nonce": "base64...",
      "aad": "base64...",
      "deleted": false,
      "serverUpdatedAt": "2026-05-21T08:00:02Z"
    }
  ],
  "nextCursor": "djE6MTAx"
}
```

#### 4) Health
`GET /healthz`

响应：
```json
{
  "status": "ok",
  "version": "0.1.0",
  "sqlite": "up",
  "time": "2026-05-21T08:00:02Z"
}
```

---

## 3.9 后端持久化模块（SQLite）

### 表结构（建议）
- `encrypted_notes`
  - `note_id TEXT PK`
  - `cipher_text BLOB NOT NULL`
  - `nonce BLOB NOT NULL`
  - `aad BLOB NULL`
  - `deleted INTEGER NOT NULL`
  - `server_updated_at TEXT NOT NULL`
  - `server_seq INTEGER NOT NULL UNIQUE`
- `idempotency_log`
  - `idempotency_key TEXT PK`
  - `response_json TEXT NOT NULL`
  - `created_at TEXT NOT NULL`
- `device_state`
  - `device_id TEXT PK`
  - `last_seen_at TEXT NOT NULL`

### 独立测试
- push 写入后 `server_seq` 单调递增。
- 按 cursor 拉取增量准确。
- 幂等日志命中后返回稳定结果。

---

## 3.10 可观测性与健康检查模块

### 职责
- 基础日志、指标与健康检查。

### 指标（MVP）
- `sync_push_requests_total`
- `sync_pull_requests_total`
- `sync_conflicts_resolved_total`
- `sync_queue_length`
- `crypto_decrypt_fail_total`

### 日志规范
- 结构化 JSON；禁止记录明文、主密码、派生密钥。

### 独立测试
- 故障注入时 `healthz` 反映 sqlite down。
- 解密失败计数增长且日志无敏感数据。

---

## 4. 模块间契约与 Mock 边界

## 4.1 前端接口契约
- `INoteRepo`：读写本地加密记录。
- `ICryptoProvider`：derive/encrypt/decrypt。
- `ISyncGateway`：push/pull/full HTTP 调用。
- `ISearchIndex`：index/search/remove。

## 4.2 后端接口契约
- `SyncService`：处理业务规则（LWW、幂等）。
- `NoteStore`：SQLite 读写抽象。
- `Clock`：可注入时间源（用于可测 LWW）。

> 测试时通过 mock `Clock`/`NoteStore`/`SyncGateway` 实现模块级隔离。

---

## 5. 配置设计（环境变量）

- `APP_PORT`（默认 `8080`）
- `APP_ENV`（`dev|prod`，默认 `prod`）
- `SQLITE_DSN`（默认 `file:data/app.db`）
- `API_TOKEN`（必填，MVP 鉴权）
- `SYNC_AUTO_INTERVAL_SEC`（默认 `60`）
- `SYNC_BATCH_SIZE`（默认 `100`）
- `KDF_PBKDF2_ITERATIONS`（默认 `600000`）
- `KDF_SALT_BYTES`（默认 `16`）
- `CRYPTO_NONCE_BYTES`（默认 `12`）
- `IDEMPOTENCY_RETENTION_DAYS`（默认 `7`）
- `LOG_LEVEL`（`debug|info|warn|error`，默认 `info`）

---

## 6. 测试设计

## 6.1 模块级测试（Unit）
- 便笺领域：CRUD、排序、标签筛选。
- 加密：派生参数、解密失败、iv 唯一性。
- 搜索：索引增删改正确性。
- 同步编排：队列消费、失败重试、状态机。

## 6.2 集成测试（Integration）
- 前端：IndexedDB + Crypto + SyncGateway(mock server) 联调。
- 后端：HTTP Handler + SQLite（临时库）联调。
- cursor 流转：push 后 pull 增量闭环。
- 幂等：重复 push 返回同一处理结果。

## 6.3 端到端测试（E2E）
- 场景1：单设备离线编辑 → 恢复联网自动同步成功。
- 场景2：双设备并发编辑同一便笺 → 最终按服务端时间收敛。
- 场景3：安装 PWA 后断网重启应用 → 可浏览与编辑本地数据。
- 场景4：错误主密码解锁失败且不泄露明文。

## 6.4 验收映射
- 将 `proposal.md` 的 AC-01 ~ AC-05 映射到自动化用例 ID（UT/IT/E2E）。

---

## 7. 开发任务拆分（可直接转 Issue）

1. 搭建 VueJS 前端骨架与路由壳层。
2. 实现 Note 领域模块（CRUD/标签/置顶/颜色）。
3. 实现 IndexedDB repository 与 schema migration。
4. 实现 PBKDF2 + AES-GCM CryptoProvider。
5. 实现本地搜索索引与查询模块。
6. 实现 SyncService（前端队列 + push/pull 编排）。
7. 搭建 Go `net/http` API 路由与鉴权中间件。
8. 实现 SQLite NoteStore + idempotency_log + cursor 查询。
9. 实现 LWW（服务端时间）裁决与 full/pull/push 端点。
10. 实现 Service Worker 与 Manifest，完成安装体验。
11. 完成 Unit/Integration/E2E 测试基线。
12. 完成配置文档与运行手册。

---

## 8. 演进与兼容策略

- 加密版本字段 `v` 保留升级通道（PBKDF2 → Argon2id）。
- API 路径维持 `/api/v1`，破坏性变更走 `/api/v2`。
- 搜索模块可从本地倒排切换到 WASM 分词/索引实现，保持 `ISearchIndex` 不变。
- 鉴权可从单 token 升级到多用户体系，保持同步 payload 兼容。
