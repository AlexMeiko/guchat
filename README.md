# guchat

`guchat` 是一个使用 Go + Gin 实现的 AI 聊天后端服务，提供用户鉴权、会话管理、消息管理、模型配置、AI 流式生成、工具调用、记忆 RAG、上下文摘要压缩与 SSE 事件推送能力。项目目标是用清晰的分层结构实现一个可运行、可联调、便于维护的聊天后端。

## 功能特性

- 支持用户注册、登录、Token 刷新与登出
- 支持会话管理、消息管理和历史消息分页加载
- 支持 AI 回复生成、流式输出、生成中取消和生成任务恢复重试
- 支持上下文摘要压缩，长对话生成时会使用摘要锚点和必要原始消息构造上下文
- 支持 SSE 实时推送正文增量、推理增量、工具调用状态和生成状态
- 支持 OpenAI Chat Completions、Responses API 和本地 `fake` 生成器
- 支持模型配置管理，可动态启用或停用模型
- 支持内置工具调用、记忆工具和 MCP 工具扩展，MCP 可通过 HTTP 或 stdio 接入
- 支持私有记忆管理、基础记忆默认注入、MySQL 记忆检索和可选 Qdrant 向量检索
- 采用 Handler / Service / Repository 分层结构，便于维护和扩展

## 技术栈

- Go `1.25.0`
- Gin Web Framework
- MySQL
- sqlx + go-sql-driver/mysql
- golang-jwt/jwt
- godotenv

## 项目结构

```text
.
├── main.go                    # 服务启动与依赖组装
├── internal
│   ├── config                 # 环境变量配置加载
│   ├── db                     # MySQL 连接初始化
│   ├── entity                 # 数据库实体
│   ├── generator              # 生成器
│   ├── handler                # HTTP Handler
│   ├── memory                 # 记忆存储、切分、嵌入和向量索引
│   ├── middleware             # 鉴权中间件
│   ├── model                  # 请求/响应模型
│   ├── repository             # 数据访问层
│   ├── router                 # 路由注册
│   ├── service                # 业务逻辑层
│   ├── stream                 # SSE 运行时管理
│   └── tool                   # 内置工具与 MCP 工具接入
├── sql
│   └── schema.sql             # MySQL 表结构
├── docs
│   └── api.md                 # API 详细文档
└── .env.example               # 环境变量示例
```

## 快速开始

### 1. 准备环境

请先安装：

- Go `1.25.0+`
- MySQL `5.7+` 或兼容版本

说明：当前 Go 最低版本由依赖约束决定，`gin v1.12.0` 要求 Go `1.25.0`。数据库侧未使用 MySQL 8.0 专属语法，MySQL `5.7+` 即可满足当前表结构和查询需求。

### 2. 克隆并安装依赖

```bash
git clone https://github.com/AlexMeiko/guchat.git
cd guchat
go mod download
```

如果仓库目录名不同，请以实际目录为准。

### 3. 初始化数据库

创建数据库后导入表结构：

```sql
CREATE DATABASE guchat CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

```bash
mysql -u root -p guchat < sql/schema.sql
```

### 4. 配置环境变量

复制示例配置：

```bash
cp .env.example .env
```

Windows PowerShell：

```powershell
Copy-Item .env.example .env
```

编辑 `.env`：

```env
PORT=8080
DATABASE_URL=mysql://root:password@localhost:3306/guchat
JWT_SECRET=please-change-to-a-long-random-secret
JWT_ACCESS_TTL_SECONDS=3600
JWT_REFRESH_TTL_SECONDS=2592000
GENERATION_RETRY_INTERVAL_SECONDS=30
GENERATION_RETRY_MAX=5
CONTEXT_TOKEN_LIMIT=256000
CONTEXT_COMPRESS_RATIO=0.8
GENERATION_MAX_TOOL_ROUNDS=12
TAVILY_API_KEY=
TAVILY_BASE_URL=https://api.tavily.com
EMBEDDING_PROVIDER=openai
EMBEDDING_BASE_URL=
EMBEDDING_API_KEY=
EMBEDDING_MODEL=
EMBEDDING_DIM=0
QDRANT_URL=
QDRANT_API_KEY=
QDRANT_COLLECTION=
QDRANT_DISTANCE=Cosine
MEMORY_SIMILARITY_THRESHOLD=
RAG_SPLITTER_PROVIDER=external_api
RAG_SPLITTER_API_URL=
RAG_SPLITTER_API_HEADERS_JSON={}
RAG_SPLITTER_API_SEGMENTS_PATH=chunks
MCP_SERVERS=
```

`DATABASE_URL` 支持 `mysql://user:password@host:port/dbname` 格式，也支持 go-sql-driver/mysql 的原生 DSN 格式。

### 5. 启动服务

```bash
go run .
```

服务默认监听：

```text
http://localhost:8080
```

健康检查：

```bash
curl http://localhost:8080/health
```

## 接口概览

基础前缀为 `/api`，除注册、登录、刷新、登出和健康检查外，其余接口默认需要：

```http
Authorization: Bearer <access_token>
```

常用接口：

| 模块 | 方法与路径 | 说明 |
| --- | --- | --- |
| 健康检查 | `GET /health` | 检查服务状态 |
| 鉴权 | `POST /api/auth/register` | 注册用户 |
| 鉴权 | `POST /api/auth/login` | 登录并获取 Token |
| 鉴权 | `POST /api/auth/refresh` | 刷新 Access Token |
| 鉴权 | `POST /api/auth/logout` | 登出并撤销 Refresh Token |
| 用户 | `GET /api/me` | 获取当前用户 |
| 会话 | `GET /api/conversations` | 获取会话列表 |
| 会话 | `POST /api/conversations` | 创建会话 |
| 消息 | `GET /api/conversations/:conversation_id/messages` | 分页获取消息 |
| 消息 | `POST /api/conversations/:conversation_id/messages` | 创建消息 |
| 消息 | `GET /api/conversations/:conversation_id/messages/:message_id` | 获取单条消息 |
| 消息 | `PATCH /api/conversations/:conversation_id/messages/:message_id` | 更新消息正文 |
| 消息 | `DELETE /api/conversations/:conversation_id/messages/:message_id` | 删除消息 |
| 生成 | `POST /api/conversations/:conversation_id/messages/:message_id/generation` | 触发 AI 回复生成 |
| 生成 | `GET /api/conversations/:conversation_id/messages/:message_id/events` | 订阅 SSE 事件 |
| 记忆 | `GET /api/memory` | 获取当前用户可管理的私有记忆 |
| 记忆 | `PATCH /api/memory/:id/status` | 更新记忆状态 |
| 记忆 | `DELETE /api/memory/:id` | 删除记忆 |
| 记忆索引 | `GET /api/admin/memory/reindex` | 获取记忆重建任务状态，需登录 |
| 记忆索引 | `POST /api/admin/memory/reindex` | 启动 active 记忆全量重建，需登录 |
| 模型 | `GET /api/models` | 获取已启用模型 |
| 模型管理 | `GET /api/admin/models` | 获取全部模型，需 admin |
| 模型管理 | `POST /api/admin/models` | 创建模型，需 admin |
| 模型管理 | `GET /api/admin/models/:id` | 获取单个模型，需 admin |
| 模型管理 | `PATCH /api/admin/models/:id` | 更新模型，需 admin |
| 模型管理 | `DELETE /api/admin/models/:id` | 删除模型，需 admin |

完整接口说明见 `docs/api.md`。

## 模型配置

模型配置存储在 `models` 表中，可通过 `/api/admin/models` 管理。

当前支持的 `provider`：

- `openai`：调用 Chat Completions 风格接口
- `openai_responses`：调用 Responses 风格接口
- `fake`：本地假流式生成器，适合开发联调

创建模型示例：

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

`extra_body` 会合并进上游模型请求体，但不能覆盖项目保留字段，例如 `model`、`messages`、`input`、`stream` 等。具体规则见 `docs/api.md`。

## 工具与 MCP 配置

当前内置工具包括：

- `get_current_time`：获取指定 IANA 时区的当前时间
- `read_web_page`：读取公开网页正文
- `search_memory`：搜索当前用户、当前会话和公共范围内的 active 记忆或知识
- `add_memory`：写入当前用户的 user / conversation scope 记忆
- `disable_memory`：禁用当前用户自己的 user / conversation scope 记忆
- `tavily_search`：配置 `TAVILY_API_KEY` 后启用 Tavily 搜索

生成接口的 `tool_mode` 支持 `auto` 和 `none`。`tool_mode=auto` 时，后端会向模型提供当前可用的内置工具和 MCP 工具；`tool_mode=none` 时，本次生成不提供工具。

外部 MCP 服务通过环境变量 `MCP_SERVERS` 配置。它是一个 JSON 数组，每个元素表示一个 MCP 服务；`name` 会作为工具名前缀，例如 MCP 服务返回 `tavily_extract` 时，最终暴露为 `tavily.tavily_extract`。

HTTP MCP 示例：

```env
MCP_SERVERS=[{"name":"tavily","transport":"http","url":"https://mcp.tavily.com/mcp/","auth_type":"query","auth_field":"tavilyApiKey","auth_key":"your-api-key"}]
```

stdio MCP 示例：

```env
MCP_SERVERS=[{"name":"github","transport":"stdio","command":"npx","args":["-y","@modelcontextprotocol/server-github"],"env":["GITHUB_PERSONAL_ACCESS_TOKEN=your-token"]}]
```

字段说明：

- `transport`：支持 `http`、`stdio`
- `url`：HTTP MCP endpoint，`transport=http` 时必填
- `auth_type`：HTTP 认证方式，支持 `none`、`query`、`header`
- `auth_field` / `auth_key`：HTTP 认证字段名和密钥
- `command` / `args`：stdio MCP 启动命令和参数，`transport=stdio` 时 `command` 必填
- `env`：传给 stdio MCP 子进程的环境变量，格式为 `KEY=value`

如果 `.env` 中需要把 `MCP_SERVERS` 写成多行 JSON，请用引号包住完整值。

## 记忆与 RAG 配置

`/api/memory` 用于管理当前用户自己的 `user` / `conversation` scope 私有记忆；不返回 `global` 记忆，也不提供公共创建接口。对话生成时，后端会默认注入少量 active、未过期、scope=user 的基础记忆，分类包括 `constraint`、`negative_preference`、`user_profile`、`preference`。

生成过程中，模型可通过内置工具按需检索或写入记忆：

- `search_memory`：检索当前用户、当前会话和公共范围内的 active 记忆或知识
- `add_memory`：保存当前用户的 user / conversation scope 记忆
- `disable_memory`：禁用当前用户自己的私有记忆

记忆检索默认可使用 MySQL。记忆向量检索是可选能力，完整配置 embedding、Qdrant 和外部切分 API 后，服务会写入和检索记忆向量索引；未完整配置时会回退到 MySQL 检索。

已存在 active 记忆可通过 `POST /api/admin/memory/reindex` 启动异步全量重建，通过 `GET /api/admin/memory/reindex` 查询任务状态。当前这两个接口要求已登录。

`EMBEDDING_BASE_URL` 必须填写最终请求地址。示例：

> OpenAI-compatible provider：`https://example.com/v1/embeddings`
>
> DashScope provider：`https://<workspace-id>.cn-beijing.maas.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding`

`EMBEDDING_DIM` 会作为 Qdrant collection 的向量维度。DashScope provider 会把该值作为 `parameters.dimension` 传给上游；OpenAI-compatible provider 暂不传 `dimensions` 参数，只校验返回向量维度应与 Qdrant collection 配置一致。

外部切分 API 当前使用 `RAG_SPLITTER_PROVIDER=external_api`。guchat 会把待入库文本以 `POST` 请求发送到 `RAG_SPLITTER_API_URL`，请求体固定为：

```json
{
  "text": "待切分文本"
}
```

服务端响应需要是 JSON，guchat 会从 `RAG_SPLITTER_API_SEGMENTS_PATH` 指定的字段路径读取分段数组；默认路径是 `chunks`，表示读取响应里的 `chunks` 字段。如果响应结构是 `{"data":{"chunks":[]}}`，则应配置为 `data.chunks`。分段数组可以是字符串数组，也可以是对象数组：

```json
{
  "chunks": [
    "第一段内容",
    {
      "text": "第二段内容",
      "start": 10,
      "end": 20
    }
  ]
}
```

对象形式中 `text` 必填；`start` 和 `end` 可选，但如果出现必须同时出现，并表示原文中的非负整数偏移。

如果切分服务需要鉴权或自定义请求头，可通过 `RAG_SPLITTER_API_HEADERS_JSON` 配置 JSON 对象，里面的键值会作为 HTTP Header 发送给切分服务。

语义切分服务可使用我的另一个项目 [GuChunk-v1](https://modelscope.cn/models/tanhao2015/GuChunk-v1) 部署后接入；它是基于 Char CNN/TCN + BiLSTM 的轻量中文 RAG 语义分块模型，用于长文档知识库切片。只要返回分段数组路径与 `RAG_SPLITTER_API_SEGMENTS_PATH` 配置一致即可。

## 上下文压缩

生成前会以源消息为截止点构造上下文。服务会使用最近的摘要锚点和必要原始消息；当上下文估算 token 数达到 `CONTEXT_TOKEN_LIMIT * CONTEXT_COMPRESS_RATIO` 时，会对较早历史写入摘要 checkpoint。

相关配置：

- `CONTEXT_TOKEN_LIMIT`：上下文 token 上限，默认 `32000`
- `CONTEXT_COMPRESS_RATIO`：摘要压缩触发比例，默认 `0.8`，有效范围 `(0, 1]`
- `GENERATION_MAX_TOOL_ROUNDS`：单次生成最大工具调用轮次，默认 `12`

## SSE 事件

生成接口会创建 assistant 消息，客户端可订阅该消息的事件流：

```bash
curl -N "http://localhost:8080/api/conversations/<conversation_id>/messages/<assistant_message_id>/events" \
  -H "Authorization: Bearer <access_token>"
```

主要事件：

- `message.snapshot`：当前消息快照
- `message.delta`：正文增量
- `message.reasoning_delta`：推理内容增量
- `tool_call.created`：工具调用创建
- `tool_call.updated`：工具调用状态更新
- `message.completed`：生成完成
- `message.failed`：生成失败

## 构建与部署

直接构建当前平台：

```bash
go build .
```

指定输出文件名：

```bash
go build -o guchat .
```

交叉编译示例：

```bash
GOOS=linux GOARCH=amd64 go build -o guchat .
GOOS=windows GOARCH=amd64 go build -o guchat.exe .
```

部署时请同时准备：

- 可执行文件
- `.env` 配置文件
- 已执行 `sql/schema.sql` 初始化的 MySQL 数据库

## 开发说明

- Handler 层负责请求解析和响应输出，核心业务放在 Service 层
- Repository 层负责数据库访问，避免业务逻辑散落在 SQL 调用处
- 涉及消息顺序、生成状态、重试恢复等逻辑时，优先保持 SQL 与事务语义清晰
- API 行为以当前实现和 `docs/api.md` 为准

## 常见问题

### 启动时报 `JWT_SECRET is required`

请检查 `.env` 是否存在，并确认 `JWT_SECRET` 已配置为非空字符串。

### 启动时报 `DATABASE_URL is required`

请检查 `.env` 中的 `DATABASE_URL`，并确认数据库已创建、账号密码正确。

### 数据库时间字段解析异常

项目连接 MySQL 时会启用 `parseTime=true`，如果使用原生 DSN，请确保包含等价配置。

### 管理模型接口返回 `403`

`/api/admin/models` 相关接口需要 `admin` 角色用户访问。

## 相关文档

- API 文档：`docs/api.md`
- 数据库表结构：`sql/schema.sql`
- 环境变量示例：`.env.example`
