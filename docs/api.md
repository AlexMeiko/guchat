# guchat-go API 文档

> 本文档以当前后端实现为准，面向前后端联调与接口维护。

---

## 目录

- [1. 快速说明](#1-快速说明)
- [2. 鉴权规则](#2-鉴权规则)
- [3. 通用约定](#3-通用约定)
- [4. 接口总览](#4-接口总览)
- [5. 详细接口](#5-详细接口)
- [6. SSE 事件格式](#6-sse-事件格式)
- [7. 已知边界行为](#7-已知边界行为)
- [8. 工具调用](#8-工具调用)
- [9. 运行配置](#9-运行配置)
- [10. 联调示例](#10-联调示例)

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
| 工具模式 | `auto`、`none` |

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

记忆重建接口权限：

- `GET /api/admin/memory/reindex`、`POST /api/admin/memory/reindex`：当前实现只要求已登录，暂未做 `admin` 角色判断。

---

## 3. 通用约定

### 3.1 成功状态码

- `200 OK`：查询、登录、刷新、更新成功
- `201 Created`：创建成功
- `202 Accepted`：异步任务已接受
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
- 创建模型时，它必须是一个 JSON object，不允许传数组、字符串、数字或 `null`，省略或传 `{}` 都表示“不追加额外参数”。
- 更新模型时，`extra_body` 为可选字段；不传或传 `null` 都表示“不修改该字段”，传 JSON object 表示更新为对应配置，传 `{}` 表示将其更新为空对象配置。
- 创建模型时，以及更新模型时显式传入 `extra_body` 为 JSON object 时，会将其压缩规范化后存库。
- 当前生成器会将 `extra_body` 合并进上游请求体，但不会允许覆盖保留字段：
  - Chat Completions：`model`、`messages`、`stream`、`tools`、`tool_choice`
  - Responses API：`model`、`input`、`stream`
- `openai_responses` 额外支持 `extra_body.tools`：如果它是数组，会从请求体中取出并追加到 Responses API 的 `tools` 列表；如果不是数组，生成时会失败。

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
| 工作区 | POST | `/api/conversations/:conversation_id/files` | Yes | user/admin |
| 工作区 | GET | `/api/conversations/:conversation_id/files` | Yes | user/admin |
| 工作区 | GET | `/api/conversations/:conversation_id/files/download` | Yes | user/admin |
| 工作区 | DELETE | `/api/conversations/:conversation_id/files` | Yes | user/admin |
| 消息 | GET | `/api/conversations/:conversation_id/messages` | Yes | user/admin |
| 消息 | POST | `/api/conversations/:conversation_id/messages` | Yes | user/admin |
| 消息 | GET | `/api/conversations/:conversation_id/messages/:message_id` | Yes | user/admin |
| 消息 | PATCH | `/api/conversations/:conversation_id/messages/:message_id` | Yes | user/admin |
| 消息 | DELETE | `/api/conversations/:conversation_id/messages/:message_id` | Yes | user/admin |
| 生成 | POST | `/api/conversations/:conversation_id/messages/:message_id/generation` | Yes | user/admin |
| 生成 | GET | `/api/conversations/:conversation_id/messages/:message_id/events` | Yes | user/admin |
| 记忆 | GET | `/api/memory` | Yes | user/admin |
| 记忆 | PATCH | `/api/memory/:id/status` | Yes | user/admin |
| 记忆 | DELETE | `/api/memory/:id` | Yes | user/admin |
| 记忆索引 | GET | `/api/admin/memory/reindex` | Yes | user/admin |
| 记忆索引 | POST | `/api/admin/memory/reindex` | Yes | user/admin |
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
    "title": "",
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

不传标题时，当前实现会创建 `title=""` 的会话。

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

说明：

- 删除会话时会销毁该会话的终端容器，并删除对应工作区目录

成功响应：`204 No Content`

常见失败：

- `400 invalid conversation id`
- `404 conversation not found`
- `500 internal server error`

---

### 5.4 消息

说明：

- 消息响应中的 assistant 消息可能包含 `tool_calls` 字段
- `tool_calls` 字段结构同 SSE 中的 `tool_call.created` / `tool_call.updated` 事件数据

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
- 删除后以数据库状态为准；再次查询该消息返回 `404`

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
- `tool_mode` 为可选字段，支持 `auto`、`none`；不传时默认为 `auto`
- `tool_mode=auto` 时，生成器可使用内置工具和已配置的 MCP 工具；若已启用沙箱，还包括 `terminal_open` / `terminal_exec`
- `tool_mode=none` 时，本次生成不向模型提供工具
- 生成前会以 system 消息形式默认注入少量 active、未过期、scope=user 的基础记忆，分类限制为 `constraint`、`negative_preference`、`user_profile`、`preference`
- 其他历史记忆、事实、总结和知识不会默认注入；`tool_mode=auto` 时模型可通过 `search_memory` 按需检索
- 上下文范围截至源消息本身；系统会使用最近的摘要锚点和必要的原始消息构造上下文，超出压缩阈值时会进行历史摘要压缩
- 生成前历史压缩会写入被覆盖历史消息的摘要 checkpoint

请求体：

```json
{
  "model_id": 1,
  "tool_mode": "auto"
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
- `400 invalid tool mode`
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

### 5.6 记忆

通用 `/api/memory` 只用于管理当前用户自己的私有记忆条目，不提供公共创建接口。记忆由后端内部流程或内置工具创建。

#### `GET /api/memory`

说明：

- 返回当前用户自己的 `user` / `conversation` scope 记忆。
- 不返回 `global` 记忆。
- 不传 `status` 时默认返回 `active` 和 `disabled`。
- 支持按 `status`、`category`、`scope` 过滤，多个值用英文逗号分隔。
- `limit` 默认 `50`，最大 `100`。
- `offset` 默认 `0`。

查询参数：

| 参数 | 说明 |
| --- | --- |
| `status` | 可选，例如 `active`、`disabled`、`deleted` |
| `category` | 可选，例如 `user_profile`、`preference`、`fact`、`knowledge` |
| `scope` | 可选，只能是 `user` 或 `conversation` |
| `limit` | 可选，范围 `1..100` |
| `offset` | 可选，最小 `0` |

成功响应：`200 OK`

```json
{
  "items": [
    {
      "id": 1,
      "conversation_id": "uuid",
      "scope": "conversation",
      "category": "preference",
      "origin": "assistant_summary",
      "source_type": "conversation",
      "source_ref": "uuid",
      "source_title": "会话标题",
      "content": "用户希望助手称呼自己为 a。",
      "metadata": {},
      "confidence": 1,
      "expires_at": "2026-06-30T12:00:00+08:00",
      "status": "active",
      "created_at": "2026-06-04T12:00:00+08:00",
      "updated_at": "2026-06-04T12:00:00+08:00"
    }
  ]
}
```

常见失败：

- `400 invalid limit`
- `400 invalid offset`
- `400 invalid memory filter`
- `401 missing Authorization header`
- `500 internal server error`

#### `PATCH /api/memory/:id/status`

请求体：

```json
{
  "status": "disabled"
}
```

说明：

- 支持目标状态：`active`、`disabled`、`deleted`。
- 只能管理当前用户自己的 `user` / `conversation` scope 记忆。
- 已删除记忆不能重新启用。
- 普通用户不能通过该接口管理 `global` 记忆。

成功响应：`204 No Content`

常见失败：

- `400 invalid memory id`
- `400 invalid request body`
- `400 invalid memory status`
- `404 memory item not found`
- `500 internal server error`

#### `DELETE /api/memory/:id`

说明：

- 软删除当前用户自己的 `user` / `conversation` scope 记忆。
- 成功后该条目不再被默认注入或工具检索返回。

成功响应：`204 No Content`

常见失败：

- `400 invalid memory id`
- `404 memory item not found`
- `500 internal server error`

#### `POST /api/admin/memory/reindex`

说明：

- 启动后台 active memory_items 全量重建。
- 只有完整配置 Memory RAG 后才可使用；未配置 indexer 时返回 `400 memory indexer is not configured`。
- 如果已有重建正在运行，会直接返回当前运行状态，不启动第二个任务。
- 当前实现只要求已登录，暂未做 `admin` 角色判断。

成功响应：`202 Accepted`

```json
{
  "running": true,
  "indexed": 0,
  "failed": 0,
  "started_at": "2026-07-02T10:00:00+08:00"
}
```

常见失败：

- `400 memory indexer is not configured`
- `401 missing Authorization header`
- `500 internal server error`

#### `GET /api/admin/memory/reindex`

说明：

- 查看后台 active memory_items 全量重建状态。
- 当前实现只要求已登录，暂未做 `admin` 角色判断。

成功响应：`200 OK`

```json
{
  "running": false,
  "indexed": 120,
  "failed": 3,
  "started_at": "2026-07-02T10:00:00+08:00",
  "finished_at": "2026-07-02T10:03:00+08:00"
}
```

---

### 5.7 模型

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
- 创建 / 更新模型配置时不会校验 provider 是否受支持；生成时才根据 provider 选择生成器。当前内置可用 provider：
  - `openai`
  - `openai_responses`
  - `fake`
- 当 `provider=openai` 时，`base_url` 可以填写服务根地址或最终 `/chat/completions` 地址；不是最终地址时后端会追加 `/chat/completions`。
- 当 `provider=openai_responses` 时，`base_url` 可以填写服务根地址或最终 `/responses` 地址；不是最终地址时后端会追加 `/responses`。
- 当 `provider=openai_responses` 且 `extra_body.tools` 是数组时，这些工具定义会追加到上游 `tools` 列表；`extra_body.tools` 不是数组会导致生成失败。

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
- `extra_body` 为可选字段；不传或传 `null` 都表示不修改该字段
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

### 5.8 工作区

工作区文件落在当前会话目录，不入库。路径参数 `:conversation_id` 必须属于当前用户。列表、上传、下载、删除都只针对工作区根目录中的单层文件名，不接受路径分隔符。

#### `GET /api/conversations/:conversation_id/files`

列出当前会话工作区根目录。

成功响应：`200 OK`

```json
{
  "items": [
    {
      "name": "notes.txt",
      "path": "/workspace/notes.txt",
      "size_bytes": 12,
      "is_dir": false,
      "mod_time": "2026-09-03T02:00:00+08:00"
    }
  ]
}
```

`path` 是容器内路径，固定为 `/workspace/{name}`。

常见失败：

- `400 invalid conversation id`
- `404 conversation not found`
- `500 internal server error`

#### `POST /api/conversations/:conversation_id/files`

`Content-Type: multipart/form-data`

字段：

- `file`：必填，上传文件
- `overwrite`：可选，字符串 `true` 时覆盖已存在的普通文件

成功响应：`201 Created`

```json
{
  "name": "notes.txt",
  "path": "/workspace/notes.txt",
  "size_bytes": 12,
  "is_dir": false,
  "mod_time": "2026-09-03T02:00:00+08:00"
}
```

常见失败：

- `400 invalid conversation id`
- `400 file is required`
- `400 invalid file name`
- `400 cannot overwrite non-regular file`
- `404 conversation not found`
- `409 file already exists`
- `413 file too large`
- `500 internal server error`

#### `GET /api/conversations/:conversation_id/files/download`

查询参数：

- `name`：必填，工作区根目录中的文件名

成功响应：`200 OK`，响应体为文件内容；`Content-Disposition` 为 attachment。

常见失败：

- `400 invalid conversation id`
- `400 file name is required`
- `400 invalid file name`
- `400 cannot download directory`
- `400 cannot download non-regular file`
- `404 conversation not found`
- `404 file not found`
- `500 internal server error`

#### `DELETE /api/conversations/:conversation_id/files`

查询参数：

- `name`：必填，工作区根目录中的文件名

成功响应：`204 No Content`

常见失败：

- `400 invalid conversation id`
- `400 file name is required`
- `400 invalid file name`
- `404 conversation not found`
- `404 file not found`
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
- `tool_call.created`
- `tool_call.updated`
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
  "tool_calls": [
    {
      "id": 1,
      "provider_id": "call_xxx",
      "name": "tavily.tavily_extract",
      "arguments": "{\"url\":\"https://example.com\"}",
      "status": "running",
      "round": 1,
      "seq": 1
    }
  ],
  "error": ""
}
```

说明：

- 建立连接后总会先收到一条 `message.snapshot`
- 如果对应 runtime task 不存在，接口只会返回这一条 `snapshot`，然后结束连接
- 如果消息在连接建立时已经是 `done` 或 `failed`，也只会返回这一条 `snapshot`，然后结束连接
- `tool_calls` 仅在该 assistant 消息已有工具调用记录时返回

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

### 6.5 `tool_call.created`

示例：

```json
{
  "id": 1,
  "provider_id": "call_xxx",
  "name": "tavily.tavily_extract",
  "arguments": "{\"url\":\"https://example.com\"}",
  "status": "pending",
  "round": 1,
  "seq": 1
}
```

说明：

- 表示模型请求了一次工具调用
- `provider_id` 是模型侧工具调用 ID
- `arguments` 是模型传入工具的原始 JSON 字符串
- `round` 表示当前工具调用轮次，`seq` 表示该轮内的顺序

### 6.6 `tool_call.updated`

示例：

```json
{
  "id": 1,
  "provider_id": "call_xxx",
  "name": "tavily.tavily_extract",
  "arguments": "{\"url\":\"https://example.com\"}",
  "result": "{\"content\":\"...\"}",
  "status": "done",
  "round": 1,
  "seq": 1
}
```

说明：

- 表示工具调用状态或结果发生变化
- `status` 可能为 `running`、`done`、`failed`
- `result` 是工具调用结果的 JSON 字符串，仅在有结果时返回
- `error_message` 仅在工具调用失败时返回

### 6.7 `message.failed`

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

- 删除生成中的消息后，数据库记录会被删除，再次调用 `GET /messages/:id` 返回 `404`
- 删除后，如果已有 SSE 连接仍处于极小的并发窗口，当前实现下它仍可能观察到 terminal 事件；业务状态应以数据库查询结果为准
- 在极小的时序窗口里，`message.completed` 可能略早于数据库最终 `done` 状态可见
- stdio MCP 工具调用开始后，当前实现会等待该工具调用返回
- 启用沙箱时，终端容器按 `SANDBOX_IDLE_TIMEOUT_SECONDS` 空闲回收；服务重启后仍在运行的会话容器会重新纳入回收，已经退出的容器会被删除
- `terminal_exec` 在容器未打开或已被回收时返回 `terminal_not_open`，工作区 `/workspace` 中的文件仍然可用

---

## 8. 工具调用

工具调用由生成接口控制。`POST /api/conversations/:conversation_id/messages/:message_id/generation` 的 `tool_mode` 为 `auto` 时，后端会向模型提供当前可用工具；`tool_mode` 为 `none` 时，本次生成不提供工具。

当前内置工具：

| 工具名 | 启用条件 | 说明 |
| --- | --- | --- |
| `get_current_time` | 默认启用 | 获取指定 IANA 时区的当前时间 |
| `read_web_page` | 默认启用 | 读取公开网页正文 |
| `search_memory` | MemoryService 已配置 | 搜索当前用户、当前会话和公共范围内的 active 记忆或知识 |
| `add_memory` | MemoryService 已配置 | 写入 active 的 user / conversation scope 记忆 |
| `disable_memory` | MemoryService 已配置 | 禁用当前用户自己的 user / conversation scope 记忆 |
| `tavily_search` | 配置 `TAVILY_API_KEY` | 使用 Tavily 搜索互联网信息 |
| `terminal_open` | `SANDBOX_ENABLED=true` 且 Docker 可用 | 打开或重新打开当前会话的临时终端容器 |
| `terminal_exec` | `SANDBOX_ENABLED=true` 且 Docker 可用 | 在已打开的终端容器中执行一条 shell 命令 |

生成前会默认向模型提供少量基础记忆，但不会默认注入全部记忆或知识。默认注入只包含 active、未过期、scope=user 的 `constraint`、`negative_preference`、`user_profile`、`preference` 条目。需要更多历史记忆、事实、总结或知识时，模型应通过 `search_memory` 检索。

#### `get_current_time`

请求参数由模型生成。

```json
{
  "timezone": "Asia/Shanghai"
}
```

成功结果示例：

```json
{
  "timezone": "Asia/Shanghai",
  "time": "2026-07-02T10:00:00+08:00",
  "unix": 1782957600
}
```

#### `read_web_page`

请求参数由模型生成。

```json
{
  "url": "https://example.com",
  "max_chars": 10000
}
```

说明：

- `url` 必填；未带 scheme 时后端会按 `https://` 补齐。
- 只允许 `http` / `https`。
- 禁止 `localhost`、回环地址、私有 IP、未指定地址和 link-local 地址。
- 响应体最多读取 `2MB`，默认返回最多 `10000` 个字符；超出时 `truncated=true`。
- 网页正文会解析 HTML、去除 `script`、`style`、`noscript`、`svg` 并做空白归一化。

成功结果示例：

```json
{
  "url": "https://example.com",
  "final_url": "https://example.com/",
  "status": 200,
  "content_type": "text/html; charset=utf-8",
  "text": "Example Domain\nThis domain is for use in illustrative examples...",
  "truncated": false
}
```

#### `tavily_search`

请求参数由模型生成。只有配置 `TAVILY_API_KEY` 时该工具才会暴露给模型。

```json
{
  "query": "OpenAI latest API changes"
}
```

说明：

- 后端固定向 Tavily 发送 `search_depth=advanced` 和 `max_results=10`。
- `TAVILY_BASE_URL` 未配置时使用 `https://api.tavily.com`。
- 工具结果是 Tavily 返回的原始 JSON。

#### `terminal_open`

请求参数由模型生成，当前无字段。

```json
{}
```

说明：

- 仅在 `SANDBOX_ENABLED=true` 且启动时 Docker 检查通过后暴露给模型。
- 每个会话对应一个临时容器，工作区挂载为 `/workspace`。
- 只有 `/workspace` 会在容器销毁后保留；容器内其它状态不保留。
- 若打开结果包含 `description`，模型应优先遵循其中的环境说明。

成功结果示例：

```json
{
  "ok": true,
  "description": "沙箱环境说明（镜像内存在该文件时才返回）"
}
```

`description` 为可选字段。

#### `terminal_exec`

请求参数由模型生成。

```json
{
  "command": "ls -la /workspace",
  "timeout_seconds": 30
}
```

说明：

- `command` 必填。
- `timeout_seconds` 未传或小于等于 `0` 时默认 `30` 秒。
- 每次调用都是新的 shell；进入子目录需写在命令里，例如 `cd project && npm test`。
- 需要保留或供用户下载的文件必须写到 `/workspace`。

成功结果示例：

```json
{
  "ok": true,
  "exit_code": 0,
  "stdout": "notes.txt\n",
  "stderr": "",
  "duration_ms": 12,
  "timed_out": false,
  "truncated": false
}
```

容器未打开或已过期时：

```json
{
  "ok": false,
  "error_code": "terminal_not_open",
  "message": "Terminal is not open or has expired. Call terminal_open before executing commands. Files in /workspace are still available."
}
```

#### `search_memory`

请求参数由模型生成，后端不会允许模型传 `user_id`、`conversation_id` 或 `status`。

```json
{
  "query": "用户希望我如何称呼他？",
  "keywords": ["称呼", "用户"],
  "categories": ["user_profile", "preference"],
  "scopes": ["user", "conversation", "global"],
  "limit": 5
}
```

说明：

- `query`：自然语言检索意图。
- `keywords`：少量精确关键词或短语，只用于辅助匹配专有名词、函数名、文件名、项目名、错误码、API 名称等。不要堆词，也不要把自然语言整句拆成关键词；语义检索主要依赖 `query`，不确定时可以不传。
- `categories`、`scopes`：可选过滤条件。
- `limit`：可选，未传或小于等于 `0` 时使用后端默认值；当前最多返回 `20` 条。
- 只返回 `active` 且未过期的条目。

成功结果示例：

```json
{
  "items": [
    {
      "id": 1,
      "scope": "user",
      "category": "user_profile",
      "source_type": "conversation",
      "source_ref": "uuid",
      "content": "用户希望助手称呼自己为 a。",
      "confidence": 1,
      "updated_at": "2026-06-04T12:00:00+08:00"
    }
  ]
}
```

#### `add_memory`

请求参数由模型生成，后端不会允许模型传 `user_id`、`conversation_id`、`origin` 或 `status`。

```json
{
  "scope": "user",
  "category": "user_profile",
  "source_type": "conversation",
  "content": "用户希望助手称呼自己为 a。",
  "confidence": 1
}
```

说明：

- `content` 必填，应简洁、明确、可复用。
- `scope` 可选，只能是 `user` 或 `conversation`；缺省值为 `user`。
- `category` 可选；缺省值为 `fact`。
- `source_type` 可选，例如 `none`、`conversation`、`web`、`file`、`api`、`repo`、`manual`。在对话中未传时，`add_memory` 默认记录为当前会话来源，相关来源引用由后端自动处理。
- `source_ref` 可选。`source_type=conversation` 时不需要传，当前会话来源由后端自动处理；网页、文件、repo 等外部来源可传 URL、文件 key 或 repo path。
- `source_title` 可选。
- `confidence` 可选，范围 `0..1`；未传时后端默认处理。
- `expires_at` 可选，RFC3339 格式；长期有效的记忆不要传。
- `category=constraint`、`negative_preference`、`user_profile`、`preference` 的 user scope 条目会在新生成请求中作为基础记忆提供给模型，因此只应用于长期稳定、跨会话普遍有用的信息。普通事实、知识、总结、短期状态不要放入这些分类。
- 后端会根据来源设置 origin：`web` / `api` / `repo` / `file` 来源写入 `tool_generated`，其他来源写入 `assistant_summary`。

成功结果示例：

```json
{
  "ok": true
}
```

#### `disable_memory`

请求参数由模型生成，后端不会允许模型传 `user_id`、`conversation_id`、`scope` 或 `status`。

```json
{
  "id": 1
}
```

说明：

- `id` 必填，应来自 `search_memory` 返回的当前用户私有记忆 ID。
- 只能禁用当前用户自己的 `user` / `conversation` scope 记忆，不能禁用 `global` 记忆或其他用户的记忆。
- 禁用后状态变为 `disabled`，不会再被默认注入或被 `search_memory` 检索到；数据仍保留在数据库中。
- 只有当用户明确要求忘记、不要再记住、禁用某条记忆，或明确纠正已保存记忆时才应使用。通常应先调用 `search_memory` 定位目标 ID。

成功结果示例：

```json
{
  "ok": true
}
```

工具失败时，工具结果会以 JSON 字符串形式记录为：

```json
{
  "error": "错误描述"
}
```

外部 MCP 工具由服务端配置决定。MCP 工具暴露给模型时会自动加上服务名前缀，例如服务名为 `tavily` 且远端工具名为 `tavily_extract` 时，模型侧工具名为 `tavily.tavily_extract`。

---

## 9. 运行配置

配置来自环境变量，启动时会尝试加载 `.env`。

### 9.1 基础配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORT` | `8080` | HTTP 监听端口 |
| `DATABASE_URL` | 必填 | MySQL 连接地址 |
| `JWT_SECRET` | 必填 | JWT 签名密钥 |
| `JWT_ACCESS_TTL_SECONDS` | `3600` | access token 有效期 |
| `JWT_REFRESH_TTL_SECONDS` | `2592000` | refresh token 有效期 |

### 9.2 生成与上下文

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `CONTEXT_TOKEN_LIMIT` | `32000` | 上下文粗估 token 上限，最小为 `1` |
| `CONTEXT_COMPRESS_RATIO` | `0.8` | 历史压缩触发比例，必须在 `(0, 1]`，非法时回退 `0.8` |
| `GENERATION_MAX_TOOL_ROUNDS` | `12` | 单次生成最大工具轮次，最小为 `1` |
| `GENERATION_RETRY_INTERVAL_SECONDS` | `30` | 最终写库失败后的重试间隔，最小为 `1` |
| `GENERATION_RETRY_MAX` | `5` | 最终写库失败后的最大重试次数，最小为 `1` |

### 9.3 Tavily

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TAVILY_API_KEY` | 空 | 配置后启用 `tavily_search` 工具 |
| `TAVILY_BASE_URL` | `https://api.tavily.com` | Tavily API base URL |

### 9.4 Memory RAG

Memory RAG 是可选能力。只有以下配置完整时，服务才会启用向量索引写入和向量检索；否则退回 MySQL 检索。

必需完整配置：

```env
EMBEDDING_PROVIDER=openai
EMBEDDING_BASE_URL=
EMBEDDING_MODEL=
EMBEDDING_DIM=1024
QDRANT_URL=
QDRANT_COLLECTION=
RAG_SPLITTER_PROVIDER=external_api
RAG_SPLITTER_API_URL=
RAG_SPLITTER_API_SEGMENTS_PATH=chunks
```

可选配置：

```env
EMBEDDING_API_KEY=
QDRANT_API_KEY=
QDRANT_DISTANCE=Cosine
MEMORY_SIMILARITY_THRESHOLD=
RAG_SPLITTER_API_HEADERS_JSON={}
```

说明：

- `EMBEDDING_PROVIDER` 当前支持 `openai` 和 `dashscope`。
- `EMBEDDING_BASE_URL` 必须是最终 embedding 请求地址，不会自动追加 endpoint。
- `EMBEDDING_DIM` 会作为 Qdrant collection vector size。
- `dashscope` provider 会把 `EMBEDDING_DIM` 作为 `parameters.dimension` 传给上游。
- `openai` embedding provider 不传 `dimensions` 参数。
- `RAG_SPLITTER_PROVIDER` 当前只支持 `external_api`。
- 启用 RAG 后，服务启动时会创建 Qdrant collection，并创建 `memory_item_id`、`owner_user_id`、`scope`、`conversation_id`、`status`、`category` payload index。

### 9.5 Sandbox

沙箱默认关闭。`SANDBOX_ENABLED=true` 时，启动会检查 Docker；不可用则进程退出。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SANDBOX_ENABLED` | `false` | 是否启用 Docker 终端沙箱和 `terminal_*` 工具 |
| `SANDBOX_DATA_ROOT` | `./data/sandbox` | 会话工作区根目录 |
| `SANDBOX_IMAGE` | `tanhao2015/guchat-sandbox:bookworm-v1.1` | 终端容器镜像 |
| `SANDBOX_IDLE_TIMEOUT_SECONDS` | `1800` | 空闲回收时间，最小为 `1` |
| `SANDBOX_UPLOAD_MAX_BYTES` | `104857600` | 工作区单文件上传上限，最小为 `1` |

工作区路径为 `{SANDBOX_DATA_ROOT}/{user_id}/{conversation_id}/workspace`，容器内挂载为 `/workspace`。

### 9.6 MCP

`MCP_SERVERS` 是 JSON 数组字符串。每个 server 必须有唯一 `name`，并支持 `transport=http` 或 `transport=stdio`。

HTTP 示例：

```json
[
  {
    "name": "tavily",
    "transport": "http",
    "url": "https://mcp.tavily.com/mcp/",
    "auth_type": "query",
    "auth_field": "tavilyApiKey",
    "auth_key": "your-api-key"
  }
]
```

stdio 示例：

```json
[
  {
    "name": "github",
    "transport": "stdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-github"],
    "env": ["GITHUB_PERSONAL_ACCESS_TOKEN=your-token"]
  }
]
```

---

## 10. 联调示例

### 10.1 登录

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alex","password":"123456"}'
```

### 10.2 获取会话列表

```bash
curl http://localhost:8080/api/conversations \
  -H "Authorization: Bearer <access_token>"
```

### 10.3 分页获取消息

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

### 10.4 创建用户消息

```bash
curl -X POST "http://localhost:8080/api/conversations/<conversation_id>/messages" \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"content":"你好"}'
```

### 10.5 触发生成

```bash
curl -X POST "http://localhost:8080/api/conversations/<conversation_id>/messages/<message_id>/generation" \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"model_id":1,"tool_mode":"auto"}'
```

### 10.6 创建模型

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

### 10.7 订阅 SSE

```bash
curl -N "http://localhost:8080/api/conversations/<conversation_id>/messages/<assistant_message_id>/events" \
  -H "Authorization: Bearer <access_token>"
```

### 10.8 启动记忆重建

```bash
curl -X POST "http://localhost:8080/api/admin/memory/reindex" \
  -H "Authorization: Bearer <access_token>"
```

---
