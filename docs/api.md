# guchat-go API 文档

> 本文档以当前后端实现为准，面向前后端联调与后续接口维护。

---

## 目录

- [1. 快速说明](#1-快速说明)
- [2. 鉴权规则](#2-鉴权规则)
- [3. 通用约定](#3-通用约定)
- [4. 接口总览](#4-接口总览)
- [5. 详细接口](#5-详细接口)
- [6. SSE 事件格式](#6-sse-事件格式)
- [7. 已知边界行为](#7-已知边界行为)
- [8. 联调示例](#8-联调示例)

---

## 1. 快速说明

| 项目 | 说明 |
| --- | --- |
| 基础前缀 | `/api` |
| 健康检查 | `GET /health` |
| 普通请求/响应 | `application/json` |
| 流式输出 | `text/event-stream` |
| 用户角色 | `user`、`admin` |
| 消息角色 | `system`、`user`、`assistant` |
| 消息状态 | `pending`、`streaming`、`done`、`failed` |
| 模型 provider | `openai`、`openai_responses`、`fake` |

---

## 2. 鉴权规则

除以下接口外，其余接口都需要登录：

- `GET /health`
- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/refresh`
- `POST /api/auth/logout`

鉴权头格式：

```http
Authorization: Bearer <access_token>
```

常见鉴权失败响应：

```json
{
  "error": "missing Authorization header"
}
```

```json
{
  "error": "invalid Authorization header"
}
```

```json
{
  "error": "invalid access token"
}
```

模型管理接口权限：

- `GET /api/models`：已登录即可访问
- `GET /api/admin/models`、`POST /api/admin/models`、`GET /api/admin/models/:id`、`PATCH /api/admin/models/:id`、`DELETE /api/admin/models/:id`：需要 `admin`

---

## 3. 通用约定

### 3.1 成功状态码

- `200 OK`：查询、登录、刷新、更新成功
- `201 Created`：创建成功
- `204 No Content`：删除成功、登出成功

### 3.2 错误响应

未特别说明时，错误响应统一为：

```json
{
  "error": "错误描述"
}
```

常见状态码：

- `400 Bad Request`
- `401 Unauthorized`
- `403 Forbidden`
- `404 Not Found`
- `409 Conflict`
- `500 Internal Server Error`

### 3.3 时间格式

响应中的时间字段来自 Go 的 `time.Time` JSON 序列化，实际返回为 RFC3339 / RFC3339Nano 风格字符串。

### 3.4 关于请求体

- 只要 handler 使用了 `ShouldBindJSON`，请求体就必须是合法 JSON。
- 即使所有字段都是可选的，也不能发送空请求体；没有要传的字段时请发送 `{}`。

### 3.5 关于 `extra_body`

- `extra_body` 仅出现在模型配置接口中。
- 它必须是一个 JSON object，不允许传数组、字符串、数字或 `null`。
- 省略或传 `{}` 都表示“不追加额外参数”。
- 创建和更新模型时会将其压缩规范化后存库。
- 当前生成器会将 `extra_body` 合并进上游请求体，但不会允许覆盖保留字段：
  - Chat Completions：`model`、`messages`、`stream`
  - Responses API：`model`、`input`、`stream`

---

## 4. 接口总览

| 模块 | Method | Path | Auth | Role |
| --- | --- | --- | --- | --- |
| 系统 | GET | `/health` | No | - |
| 认证 | POST | `/api/auth/register` | No | - |
| 认证 | POST | `/api/auth/login` | No | - |
| 认证 | POST | `/api/auth/refresh` | No | - |
| 认证 | POST | `/api/auth/logout` | No | - |
| 认证 | GET | `/api/me` | Yes | user/admin |
| 会话 | GET | `/api/conversations` | Yes | user/admin |
| 会话 | POST | `/api/conversations` | Yes | user/admin |
| 会话 | GET | `/api/conversations/:conversation_id` | Yes | user/admin |
| 会话 | PATCH | `/api/conversations/:conversation_id` | Yes | user/admin |
| 会话 | DELETE | `/api/conversations/:conversation_id` | Yes | user/admin |
| 消息 | GET | `/api/conversations/:conversation_id/messages` | Yes | user/admin |
| 消息 | POST | `/api/conversations/:conversation_id/messages` | Yes | user/admin |
| 消息 | GET | `/api/conversations/:conversation_id/messages/:message_id` | Yes | user/admin |
| 消息 | PATCH | `/api/conversations/:conversation_id/messages/:message_id` | Yes | user/admin |
| 消息 | DELETE | `/api/conversations/:conversation_id/messages/:message_id` | Yes | user/admin |
| 生成 | POST | `/api/conversations/:conversation_id/messages/:message_id/generation` | Yes | user/admin |
| 生成 | GET | `/api/conversations/:conversation_id/messages/:message_id/events` | Yes | user/admin |
| 模型 | GET | `/api/models` | Yes | user/admin |
| 模型管理 | GET | `/api/admin/models` | Yes | admin |
| 模型管理 | POST | `/api/admin/models` | Yes | admin |
| 模型管理 | GET | `/api/admin/models/:id` | Yes | admin |
| 模型管理 | PATCH | `/api/admin/models/:id` | Yes | admin |
| 模型管理 | DELETE | `/api/admin/models/:id` | Yes | admin |

---

## 5. 详细接口

### 5.1 系统

#### `GET /health`

成功响应：

```json
{
  "status": "ok"
}
```

---

### 5.2 认证

#### `POST /api/auth/register`

请求体：

```json
{
  "username": "alex",
  "password": "123456"
}
```

成功响应：`201 Created`

```json
{
  "ok": true
}
```

常见失败：

- `400 invalid request body`
- `409 username already exists`
- `500 internal server error`

#### `POST /api/auth/login`

请求体：

```json
{
  "username": "alex",
  "password": "123456"
}
```

成功响应：`200 OK`

```json
{
  "access_token": "jwt-access-token",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "jwt-refresh-token",
  "refresh_expires_in": 2592000,
  "user": {
    "id": 1,
    "username": "alex",
    "role": "user"
  }
}
```

常见失败：

- `400 invalid request body`
- `401 invalid username or password`
- `500 internal server error`

#### `POST /api/auth/refresh`

请求体：

```json
{
  "refresh_token": "jwt-refresh-token"
}
```

成功响应：`200 OK`

```json
{
  "access_token": "new-jwt-access-token",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

常见失败：

- `400 invalid request body`
- `401 invalid refresh token`
- `500 internal server error`

#### `POST /api/auth/logout`

请求体：

```json
{
  "refresh_token": "jwt-refresh-token"
}
```

成功响应：`204 No Content`

常见失败：

- `400 invalid request body`
- `401 invalid refresh token`
- `500 internal server error`

#### `GET /api/me`

成功响应：`200 OK`

```json
{
  "id": 1,
  "username": "alex",
  "role": "user"
}
```

常见失败：

- `401 missing Authorization header`
- `401 invalid Authorization header`
- `401 invalid access token`
- `401 current user not found`
- `500 invalid current user context`

---

### 5.3 会话

#### `GET /api/conversations`

成功响应：`200 OK`

```json
[
  {
    "id": "uuid",
    "title": "默认标题",
    "created_at": "2026-03-20T18:00:00+08:00",
    "updated_at": "2026-03-20T18:05:00+08:00"
  }
]
```

常见失败：

- `500 internal server error`

#### `POST /api/conversations`

请求体示例：

```json
{
  "title": "可选标题"
}
```

如果不传标题，请发送：

```json
{}
```

成功响应：`201 Created`

```json
{
  "id": "uuid",
  "title": "可选标题",
  "created_at": "2026-03-20T18:00:00+08:00",
  "updated_at": "2026-03-20T18:00:00+08:00"
}
```

常见失败：

- `400 invalid request body`
- `500 internal server error`

#### `GET /api/conversations/:conversation_id`

成功响应：`200 OK`

```json
{
  "id": "uuid",
  "title": "会话标题",
  "created_at": "2026-03-20T18:00:00+08:00",
  "updated_at": "2026-03-20T18:05:00+08:00"
}
```

常见失败：

- `400 invalid conversation id`
- `404 conversation not found`
- `500 internal server error`

#### `PATCH /api/conversations/:conversation_id`

请求体：

```json
{
  "title": "新标题"
}
```

成功响应：`204 No Content`

常见失败：

- `400 invalid conversation id`
- `400 invalid request body`
- `404 conversation not found`
- `500 internal server error`

#### `DELETE /api/conversations/:conversation_id`

成功响应：`204 No Content`

常见失败：

- `400 invalid conversation id`
- `404 conversation not found`
- `500 internal server error`

---

### 5.4 消息

#### `GET /api/conversations/:conversation_id/messages`

查询参数：

- `limit`：可选，单页返回条数，默认 `20`，最大 `100`
- `before_seq`：可选，返回 `seq` 小于该值的更早消息；不传时返回最新一页

说明：

- 列表接口采用基于 `seq` 的游标分页，不使用 `offset`
- 响应中的 `items` 始终按正序返回，前端可直接渲染
- `next_before_seq` 表示下一次继续向前加载历史消息时应传入的游标
- 当 `has_more` 为 `false` 时，表示当前已加载到该会话的最早消息

成功响应：`200 OK`

```json
{
  "items": [
    {
      "id": "uuid",
      "conversation_id": "uuid",
      "role": "user",
      "content": "你好",
      "reasoning_content": "",
      "status": "done",
      "error_message": "",
      "created_at": "2026-03-20T18:00:00.000+08:00"
    },
    {
      "id": "uuid",
      "conversation_id": "uuid",
      "role": "assistant",
      "content": "你好，有什么可以帮你？",
      "reasoning_content": "",
      "status": "done",
      "error_message": "",
      "created_at": "2026-03-20T18:00:01.000+08:00"
    }
  ],
  "has_more": true,
  "next_before_seq": 4096
}
```

常见失败：

- `400 invalid conversation id`
- `400 invalid limit`
- `400 invalid before_seq`
- `404 conversation not found`
- `500 internal server error`

#### `POST /api/conversations/:conversation_id/messages`

请求体：

```json
{
  "content": "你好",
  "prev_id": "可选，上一条消息 ID"
}
```

说明：

- 该接口当前固定创建一条 `role=user` 的消息
- `prev_id` 为空时追加到当前会话末尾，也就是最新位置；非空时插入到指定消息之后

成功响应：`201 Created`

```json
{
  "id": "uuid",
  "conversation_id": "uuid",
  "role": "user",
  "content": "你好",
  "reasoning_content": "",
  "status": "done",
  "error_message": "",
  "created_at": "2026-03-20T18:00:00.000+08:00"
}
```

常见失败：

- `400 invalid conversation id`
- `400 invalid request body`
- `400 invalid prev id`
- `404 conversation not found`
- `404 prev message not found`
- `409 message position conflict`
- `500 internal server error`

#### `GET /api/conversations/:conversation_id/messages/:message_id`

成功响应：`200 OK`

```json
{
  "id": "uuid",
  "conversation_id": "uuid",
  "role": "assistant",
  "content": "生成结果",
  "reasoning_content": "推理内容",
  "status": "done",
  "error_message": "",
  "created_at": "2026-03-20T18:00:00.000+08:00"
}
```

常见失败：

- `400 invalid conversation id`
- `400 invalid message id`
- `404 conversation not found`
- `404 message not found`
- `500 internal server error`

#### `PATCH /api/conversations/:conversation_id/messages/:message_id`

请求体：

```json
{
  "content": "修改后的内容"
}
```

说明：

- 当前实现不区分角色，允许更新任意消息的 `content`
- 返回完整更新后的消息对象

成功响应：`200 OK`

```json
{
  "id": "uuid",
  "conversation_id": "uuid",
  "role": "assistant",
  "content": "修改后的内容",
  "reasoning_content": "推理内容",
  "status": "done",
  "error_message": "",
  "created_at": "2026-03-20T18:00:00.000+08:00"
}
```

常见失败：

- `400 invalid conversation id`
- `400 invalid message id`
- `400 invalid request body`
- `404 conversation not found`
- `404 message not found`
- `500 internal server error`

#### `DELETE /api/conversations/:conversation_id/messages/:message_id`

成功响应：`204 No Content`

实现说明：

- 如果该消息当前仍存在内存中的生成任务，服务端会同步取消该任务
- 删除后以数据库状态为准；后续再查询该消息应返回 `404`

常见失败：

- `400 invalid conversation id`
- `400 invalid message id`
- `404 conversation not found`
- `404 message not found`
- `500 internal server error`

---

### 5.5 生成

#### `POST /api/conversations/:conversation_id/messages/:message_id/generation`

说明：

- 路径中的 `:message_id` 是源消息 ID
- 成功后会新建一条 assistant 消息，并异步开始生成
- `context_limit` 为可选字段，表示本次生成最多携带多少条最近的非 `system` 消息
- 所有 `system` 消息都会始终保留，不计入 `context_limit`
- 不传 `context_limit` 时，当前实现默认使用 `25`
- 上下文裁剪范围截至源消息本身

请求体：

```json
{
  "model_id": 1,
  "context_limit": 25
}
```

成功响应：`201 Created`

```json
{
  "id": "assistant-message-uuid",
  "conversation_id": "uuid",
  "role": "assistant",
  "content": "",
  "reasoning_content": "",
  "status": "pending",
  "error_message": "",
  "created_at": "2026-03-20T18:00:00.000+08:00"
}
```

常见失败：

- `400 invalid conversation id`
- `400 invalid message id`
- `400 invalid request body`
- `400 invalid context limit`
- `404 conversation not found`
- `404 message not found`
- `404 model not found`
- `409 model is disabled`
- `409 message position conflict`
- `500 internal server error`

#### `GET /api/conversations/:conversation_id/messages/:message_id/events`

说明：

- 用于订阅指定 assistant 消息的流式事件
- 如果对应 runtime task 不存在，会立即返回一条 `message.snapshot` 后结束连接
- 如果消息在建立连接时已经是 `done` 或 `failed`，也只会返回一条 `message.snapshot`

常见失败：

- `400 invalid conversation id`
- `400 invalid message id`
- `404 conversation not found`
- `404 message not found`
- `500 internal server error`
- `500 streaming unsupported`

---

### 5.6 模型

#### `GET /api/models`

说明：

- 仅返回已启用模型
- 返回简版信息，不包含 `provider`、`base_url`、`api_key`、`extra_body`

成功响应：`200 OK`

```json
[
  {
    "id": 1,
    "name": "DeepSeek R1"
  }
]
```

常见失败：

- `500 internal server error`

#### `GET /api/admin/models`（admin）

说明：

- 返回全部模型配置，包括已启用和未启用模型
- 当前实现直接返回模型详情数组

成功响应：`200 OK`

```json
[
  {
    "id": 1,
    "name": "DeepSeek R1",
    "provider": "openai",
    "model_key": "deepseek-r1-0528",
    "base_url": "https://api.openai.com/v1",
    "api_key": "sk-xxx",
    "extra_body": {
      "temperature": 0.3,
      "top_p": 0.95
    },
    "is_enabled": true,
    "created_at": "2026-03-20T18:00:00+08:00",
    "updated_at": "2026-03-20T18:00:00+08:00"
  },
  {
    "id": 2,
    "name": "Disabled Model",
    "provider": "openai_responses",
    "model_key": "gpt-4.1-mini",
    "base_url": "https://api.openai.com/v1",
    "api_key": "sk-yyy",
    "extra_body": {},
    "is_enabled": false,
    "created_at": "2026-03-20T18:10:00+08:00",
    "updated_at": "2026-03-20T18:10:00+08:00"
  }
]
```

常见失败：

- `403 forbidden`
- `500 internal server error`

#### `POST /api/admin/models`（admin）

请求体：

```json
{
  "name": "DeepSeek R1",
  "provider": "openai",
  "model_key": "deepseek-r1-0528",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-xxx",
  "extra_body": {
    "temperature": 0.3,
    "top_p": 0.95
  },
  "is_enabled": true
}
```

说明：

- `extra_body` 可省略；省略时等价于空对象
- `extra_body` 必须是 JSON object，否则返回 `400 invalid extra_body`
- 当前内置可用 provider：
  - `openai`
  - `openai_responses`
  - `fake`

成功响应：`201 Created`

```json
{
  "id": 1,
  "name": "DeepSeek R1",
  "provider": "openai",
  "model_key": "deepseek-r1-0528",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-xxx",
  "extra_body": {
    "temperature": 0.3,
    "top_p": 0.95
  },
  "is_enabled": true,
  "created_at": "2026-03-20T18:00:00+08:00",
  "updated_at": "2026-03-20T18:00:00+08:00"
}
```

常见失败：

- `400 invalid request body`
- `400 invalid extra_body`
- `403 forbidden`
- `500 internal server error`

#### `GET /api/admin/models/:id`（admin）

成功响应：`200 OK`

```json
{
  "id": 1,
  "name": "DeepSeek R1",
  "provider": "openai",
  "model_key": "deepseek-r1-0528",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-xxx",
  "extra_body": {
    "temperature": 0.3,
    "top_p": 0.95
  },
  "is_enabled": true,
  "created_at": "2026-03-20T18:00:00+08:00",
  "updated_at": "2026-03-20T18:00:00+08:00"
}
```

常见失败：

- `400 invalid model id`
- `403 forbidden`
- `404 model not found`
- `500 internal server error`

#### `PATCH /api/admin/models/:id`（admin）

请求体示例：

```json
{
  "name": "DeepSeek R1 New",
  "extra_body": {
    "temperature": 0.8,
    "max_output_tokens": 2048
  },
  "is_enabled": false
}
```

说明：

- 所有字段都是可选的，仅传需要修改的字段即可
- 如果要清空额外参数，建议传：

```json
{
  "extra_body": {}
}
```

成功响应：`200 OK`

响应体同 `GET /api/admin/models/:id`。

常见失败：

- `400 invalid model id`
- `400 invalid request body`
- `400 invalid extra_body`
- `403 forbidden`
- `404 model not found`
- `500 internal server error`

#### `DELETE /api/admin/models/:id`（admin）

成功响应：`204 No Content`

常见失败：

- `400 invalid model id`
- `403 forbidden`
- `404 model not found`
- `500 internal server error`

---

## 6. SSE 事件格式

接口：

`GET /api/conversations/:conversation_id/messages/:message_id/events`

响应头：

- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`
- `X-Accel-Buffering: no`

基础格式：

```text
event: <event_name>
data: <json>

```

事件类型：

- `message.snapshot`
- `message.delta`
- `message.reasoning_delta`
- `message.completed`
- `message.failed`

### 6.1 `message.snapshot`

示例：

```json
{
  "message_id": "uuid",
  "status": "streaming",
  "content": "当前完整内容",
  "reasoning_content": "当前完整推理内容",
  "error": ""
}
```

说明：

- 建立连接后总会先收到一条 `message.snapshot`
- 如果对应 runtime task 不存在，接口只会返回这一条 `snapshot`，然后结束连接
- 如果消息在连接建立时已经是 `done` 或 `failed`，也只会返回这一条 `snapshot`，然后结束连接

### 6.2 `message.delta`

示例：

```json
{
  "message_id": "uuid",
  "delta": "新增文本"
}
```

说明：

- 表示 `content` 的增量

### 6.3 `message.reasoning_delta`

示例：

```json
{
  "message_id": "uuid",
  "delta": "新增推理文本"
}
```

说明：

- 虽然事件名是 `message.reasoning_delta`，字段名仍然是 `delta`
- 表示 `reasoning_content` 的增量

### 6.4 `message.completed`

示例：

```json
{
  "message_id": "uuid",
  "status": "done",
  "content_bytes": 120,
  "reasoning_bytes": 2048
}
```

### 6.5 `message.failed`

示例：

```json
{
  "message_id": "uuid",
  "status": "failed",
  "error": "错误信息",
  "content_bytes": 30,
  "reasoning_bytes": 0
}
```

---

## 7. 已知边界行为

- 删除生成中的消息后，数据库记录会被删除，后续 `GET /messages/:id` 应返回 `404`
- 删除后，如果已有 SSE 连接仍处于极小的并发窗口，当前实现下它仍可能观察到 terminal 事件；业务状态应以数据库查询结果为准
- 在极小的时序窗口里，`message.completed` 可能略早于数据库最终 `done` 状态可见

---

## 8. 联调示例

### 8.1 登录

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alex","password":"123456"}'
```

### 8.2 获取会话列表

```bash
curl http://localhost:8080/api/conversations \
  -H "Authorization: Bearer <access_token>"
```

### 8.3 分页获取消息

获取最新 20 条消息：

```bash
curl "http://localhost:8080/api/conversations/<conversation_id>/messages?limit=20" \
  -H "Authorization: Bearer <access_token>"
```

继续向前加载更早消息：

```bash
curl "http://localhost:8080/api/conversations/<conversation_id>/messages?before_seq=4096&limit=20" \
  -H "Authorization: Bearer <access_token>"
```

### 8.4 创建用户消息

```bash
curl -X POST "http://localhost:8080/api/conversations/<conversation_id>/messages" \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"content":"你好"}'
```

### 8.5 触发生成

```bash
curl -X POST "http://localhost:8080/api/conversations/<conversation_id>/messages/<message_id>/generation" \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"model_id":1,"context_limit":25}'
```

### 8.6 创建模型

```bash
curl -X POST "http://localhost:8080/api/admin/models" \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"DeepSeek R1",
    "provider":"openai",
    "model_key":"deepseek-r1-0528",
    "base_url":"https://api.openai.com/v1",
    "api_key":"sk-xxx",
    "extra_body":{"temperature":0.3},
    "is_enabled":true
  }'
```

### 8.7 订阅 SSE

```bash
curl -N "http://localhost:8080/api/conversations/<conversation_id>/messages/<assistant_message_id>/events" \
  -H "Authorization: Bearer <access_token>"
```

---
