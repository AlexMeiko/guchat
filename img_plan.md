# read_image 方案

引入 `read_image`，让模型读取当前会话工作区中的图片。像素不进库、不进 SSE；发给上游的图在组请求时按**当前模型**动态附上或改写工具视图。分析文本只在非视觉模型**当时调用**时生成，之后不再补。

本文是已确认方案，实现时按此执行。

---

## 1. 为什么不把图画进工具结果

当前链路不适合让工具直接返回 base64：

- `tool_calls.result_json` 是 `MEDIUMTEXT`，会进入历史重放、摘要和 token 粗估
- SSE `tool_call.updated` 会把 `result` 推给前端
- 像素若写进 `result_json`，库、SSE、摘要、token 粗估都会被撑爆；发给上游的图只在组请求时附上

因此：工具只落小 JSON 引用；缩放后的副本另存到会话 `images/` 目录。组请求时读副本附上，不把像素写入 `result_json`。

---

## 2. 存什么、发什么

库 / SSE / 前端始终是引用，**不随模型切换改写**。

库 / SSE 仍只存小 JSON，**不把 base64 写入 `result_json`**。发给模型用的副本另外落在会话目录、工作区旁边：

```text
{SANDBOX_DATA_ROOT}/{user_id}/{conversation_id}/workspace/   # 用户文件，可删
{SANDBOX_DATA_ROOT}/{user_id}/{conversation_id}/images/      # read_image 缩放后的副本，用户 API 不可见
```

删除会话时 `DeleteConversation` 本来就会 `RemoveAll` 整个 `{user_id}/{conversation_id}`，副本一并清掉。

`result_json`（库 / SSE / 前端）给后端自己用，例如：

```json
{
  "ok": true,
  "path": "/workspace/chart.png",
  "mime_type": "image/png",
  "width": 3840,
  "height": 2160,
  "size_bytes": 12345,
  "store_key": "<sha256>",
  "stored_mime_type": "image/jpeg",
  "stored_width": 2000,
  "stored_height": 1125,
  "intent": "这次要从图里得到什么，必填",
  "analysis": "仅非视觉调用当时写出时才有"
}
```

发给**模型**的文本视图更瘦，第一版不加路径、尺寸、坐标比例：

| 情况 | 文本 |
| --- | --- |
| 视觉，且副本可用 | `{"ok": true}` + 附图 |
| 非视觉，且已有 analysis | `{"ok": true, "analysis": "……"}` |
| 非视觉，且没有 analysis | `{"ok": false, "error": "current model does not support vision"}` |
| 失败（坏文件 / 副本也没了等） | `{"ok": false, "error": "……"}` |

`path` / `intent` 已在工具参数里，不必再回给模型。`store_key` / 尺寸只留在 `result_json`，组请求时剥掉。视图不回写库。

`analysis` 是 `read_image` 独有的持久化字段，放在 `ReadImageResult` 中，不提升为通用 `ToolResult` 字段。`ToolReadImage` 和 `ReadImageResult` 属于 `internal/tool`，与 `terminal_exec` 等工具一样，不要放到 `service`。

通用工具结果只增加一个不参与 JSON 序列化的临时图片附件，这是给模型看的运输结构，任何工具都可以用，不是 `read_image` 专属：

```go
// internal/tool
type ReadImageResult struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	// path、MIME、尺寸、store_key 等引用字段
	Intent   string `json:"intent,omitempty"`
	Analysis string `json:"analysis,omitempty"`
}

// internal/service — 通用 DTO，不是某个工具的 schema
type GenerateImage struct {
	MIMEType string
	Bytes    []byte
}

type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result"`
	Image      *GenerateImage  `json:"-"`
}
```

`ToolResult.Image` 只允许出现在发起上游请求前创建的内存副本中。数据库和 SSE 使用的原始 `ToolResult.Result` 仍是完整引用 JSON，`Image` 必须为 `nil`。

四层数据不要混：

| 层 | 是什么 | 有没有像素 |
| --- | --- | --- |
| 原始 `ToolResult.Result` / `result_json` | 小 JSON 引用（path、尺寸、`store_key`、可选 analysis） | 无 |
| `images/{store_key}`（落盘副本） | 缩放后的图片字节，发给模型的像素来源 | 有，磁盘文件 |
| 投影后的 `ToolResult`（仅内存） | 瘦 `Result` + 可选 `Image`；从原始工具交换记录复制产生 | 视觉模型时有 |
| 上游 HTTP 请求体 | 生成器把 `Image` 编成 Chat Completions / Responses 所需的 data URL | 视觉模型时有 |

落库 / SSE 只碰第一层（完整 `result_json`）。组请求时由**上下文组装器**复制工具交换记录：若结果 JSON 带 `store_key` 或 `analysis`，按第 4 节换成瘦 JSON，视觉模型再读盘填入 `Image`；没有这些字段则原样通过。原始记录不变。生成器只负责协议。粗估用**投影后的瘦文本** + 每张图定额，**禁止**对副本字节、data URL 或整份 `result_json` 做 `len/3`。

主生成和摘要共用第 4 节过滤器：当前模型能看图则由组装器凭 `store_key` 读 `images/` 附图（读盘在组装器，不在生成器，也不在 `GenerationService` 循环里），不能看图则只发文本（有 analysis 就带）。刚 `read_image` 就被压缩时，摘要模型必须能看见图，否则画面事实进不了 `workingSummary`。摘要的**输出**仍是纯文本；压缩完成后这些图不再进入后续主上下文。

不依赖工作区原图是否还在，一律读 `images/{store_key}`。

### 谁负责组模型视图

`CallTool` 只负责执行并得到要存下来的引用。同一条 `result_json` 会在历史重放、当轮下一跳、摘要里按**当前模型**反复翻译，不能把模型视图写回库，也不能让 `CallTool` 兼做这件事。

不要把翻译写进 `GenerationService.Process`，也不要给 `ToolProvider` 增加 `ProjectForModel`，也不要再加 `ToolResultViewer`。`Process` 已经同时在跑任务生命周期、落库、SSE、工具循环和拼上下文；工具侧只应继续提供 `ListTools` / `CallTool`。`ImageProvider` 不负责组模型视图。

从生成服务抽出 `ContextAssembler`，专门把已有材料变成模型看见的 `[]GenerateMessage`。它不执行工具、不写库、不碰上游协议。压缩和工具循环中间夹着 IO，所以组装器不是一次 `Assemble()`，而是一组步骤：`BindHistory`、`View` / `ViewResult`、`Prefix`、`Round`、`Estimate`。

`GenerationService` 继续决定何时 View、何时估 token、何时摘要、何时 `CallTool`。它不引用 `ToolReadImage` 或完整 `ReadImageResult`。

组装器按通用图片引用投影，不认工具名：结果 JSON 里若有 `store_key` 或 `analysis`，按第 4 节改写；否则原样返回。读盘通过注入的 `OpenStoredImage`（`WorkspaceManager` 已有），不 import `internal/tool`。完整 `ReadImageResult`（path、宽高等）仍只属于 `internal/tool`，给库 / SSE 用。

请求级 `store_key` 读盘缓存跟一次 `Process` 走（`assembler.Session(user)`）。不同用户 / 会话的缓存不混用。

压缩循环（调摘要模型、写回摘要）先留在 `Process`。工作区清单、skill、工具循环内 `workingSummary` 等各自做的时候再往组装器加。

---

## 3. 调用当时（CallTool）

由**当前**这次生成使用的模型决定，只发生在工具执行时。

### 3.1 视觉模型调用

- 读工作区原文件、校验 MIME / 大小、解码拿宽高
- 缩放并编码后写入 `images/{store_key}`（这是之后发给模型的那份）
- `intent` 必填；发给模型的视图不带回，组请求时 arguments 里已有
- `result_json` 落引用 + `store_key` + 原始/缩放尺寸 + `intent`（给库 / SSE，不进模型视图）；不写 base64、不写 `analysis`

### 3.2 非视觉模型调用

- 同样读原文件、校验、写入 `images/{store_key}`、落引用
- `intent` 必填；空则工具失败，不读文件。fallback 用主模型给出的目的提问，不再套默认句
- `ImageProvider.readImage` 通过 `UserContext.AnalyzeImage` 调用 fallback 视觉模型（带上图和 intent）
- `analysis` 以及 `intent` 写入该条 `result_json`
- **当轮**把分析文本还给这个非视觉模型
- 未配置 fallback：不分析；当轮返回「当前模型不支持视觉」；引用仍保存，以后换视觉模型还能附图

`internal/imageproc` 不生成 `analysis`。它只做确定性的格式检测、解码、缩放、哈希和重新编码，不依赖模型配置、生成器或网络。fallback 选择、请求取消和错误策略属于工具执行编排，放在 `ImageProvider` 及其注入的 `VisionAnalyzer` 中。

参数示例：

```json
{
  "path": "/workspace/chart.png",
  "intent": "读出柱状图里 2023 年的销售额"
}
```

`read_image` 对非视觉模型也要出现在工具列表中（否则无法先读、再换视觉模型看图）。「不支持」只作用于发给 LLM 的视图，或当轮无法分析时的返回。

Fallback 请求本身不带任何工具，避免递归。Fallback 模型必须 `supports_vision=true` 且已启用。

---

## 4. 之后（换模型 / 新的一次生成）

分析只在非视觉模型当时调用时生成；之后一律走过滤器，**不再调用视觉模型补分析**。

| 当前模型 | 发给模型的文本 | 附图 | analysis |
| --- | --- | --- | --- |
| 视觉 | `{"ok": true}` | 附上 | 有也不发 |
| 非视觉，且已有 analysis | `{"ok": true, "analysis": "……"}` | 不附 | 发已有的，不新建 |
| 非视觉，且没有 analysis | `{"ok": false, "error": "current model does not support vision"}` | 不附 | 不新建 |

主生成、当轮 tool 返回后的下一轮、以后的历史重放、摘要，都用这张表。不要把 `result_json` 原样塞给模型。

会话可以换模型，但每次 `POST .../generation` 只带一个 `model_id`。历史 tool 结果与当时用的模型解耦；每次用**当前**模型重新解释。

典型路径：

1. 非视觉调用 `read_image` → 当轮拿到 `analysis` 并落库
2. 再切到视觉 → 同一条历史变成 `ok: true` 并附图，不发 `analysis`
3. 再切回非视觉 → 有 `analysis` 则 `ok: true` 并发分析，不附图；没有则「不支持」
4. 视觉调用的 `read_image` 后来切到非视觉、且从未写过 `analysis` → 「不支持」，不补分析

---

## 5. 缩放与文件边界

发给模型的是副本，不是工作区原文件。

- 最长边 ≤ 2000（`2000×2000` 可以；更大的按比例缩小）
- 不放大
- **一律重新编码**，小图不原样透传（小图不重编码属于可选优化，先不做）
- 输出格式：原图是 JPEG → JPEG；PNG / WebP → PNG（Go 没有 WebP encoder）
- 这些尺寸和 `store_key` 只写入 `result_json`，不放进发给模型的文本；第一版不做坐标映射 hint
- CallTool 时做完缩放并落副本；同一轮生成里同一 `store_key` 只读盘一次

暂缓（第一版不做）：

- EXIF Orientation 摆正（手机竖拍可能是横的，后续补）
- APNG：先不拦，`png.Decode` 只出默认静帧，动画区分以后做

路径：

- 只读当前会话工作区
- **只接受 `/workspace/...` 开头的容器路径**（含子目录）；HTTP 下载/删除也改为只传 `path` 参数
- 防 `..`、防 symlink 逃逸（中间目录段也检查）

格式：PNG / JPEG / WebP，按 magic bytes 识别，不只看扩展名。GIF 不支持（首帧/动画语义不清，要看先转 PNG/JPEG）。

大小：

- `READ_IMAGE_MAX_BYTES` 限制原文件，默认 10MB，超限工具失败
- 输出不再设编码体积上限：最长边 2000 内 JPEG q90 / PNG 最坏体积可接受，质量阶梯、降尺寸循环已去掉

工作区原图被删：

- 只要 `images/{store_key}` 还在，视觉仍附图，非视觉仍按过滤器发 analysis / 不支持
- 副本也不在了（例如删了整个会话）：不能附图；有 `analysis` 的非视觉仍可发分析文本；否则说明图片已不可用。不让整次生成失败

`store_key` 建议用原文件内容 SHA-256，同一张图多次 `read_image` 可共用副本。

---

## 6. 发给上游时怎么附

一次 `read_image` 只对应一张图，图跟在**该条 tool 消息**里，不插额外 user，也不把多张图合成一条消息。

Chat Completions（已在 `deepseek-v4-flash-vision-exp` 兼容口验证）：

- 视觉：`role=tool` 的 `content` 为数组

```json
{
  "role": "tool",
  "tool_call_id": "call_1",
  "content": [
    { "type": "text", "text": "{\"ok\":true}" },
    {
      "type": "image_url",
      "image_url": { "url": "data:image/png;base64,..." }
    }
  ]
}
```

- 非视觉：`content` 仍是字符串，即上表那份瘦视图
- 不使用「tool 文本 + 额外 user 带图」，也不把 `image_url` 写成裸 data URL 字符串（实现固定用 `{ "url": "data:..." }`）

Responses API：

- 视觉：每个 `function_call_output.output` 为 `input_text` + `input_image`
- 非视觉：只有文本

摘要请求：

- 当前模型支持视觉：对被压缩的那一段里的 `read_image` 附图（和主生成同一套 `tool.content` / `input_image`）
- 当前模型不支持视觉：不附图；有 `analysis` 则带上，否则 `ok: false`
- `generateSummary` 仍把 `Tools` 交给生成器，但摘要 prompt 已禁止调工具；摘要与主生成走同一套组装器 `View`，不各自实现读盘
- 一次摘要里图很多时，粗估按张数累加；若摘要请求自身会超 `CONTEXT_TOKEN_LIMIT`，优先保留较新的图，更早的只留元数据 / analysis。第一版可以先全附，超限再截。

token 粗估：工具文本按发给模型的瘦视图计，图片按张数定额（例如每张 ~1200 token），不把 base64 当文本。主生成和摘要都用这套粗估。

---

## 7. 模型与配置

`models` 表增加 `supports_vision`。Admin 创建 / 更新 / 详情带上该字段；现有库补 `ALTER`。公开的启用模型列表也可返回该字段，方便前端切换模型时展示。

环境变量：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `VISION_FALLBACK_MODEL_ID` | 空 | 非视觉模型调用 `read_image` 时使用的视觉模型 `models.id`，必须已启用且 `supports_vision=true` |
| `READ_IMAGE_MAX_BYTES` | `10485760` | 原文件上限 |
| `READ_IMAGE_MAX_EDGE` | `2000` | 发给模型的副本最长边 |

工作区始终存在，不依赖 `SANDBOX_ENABLED`。因此 `read_image` 默认可用。

用户消息暂不支持直接贴图，那是另一条能力。消息表仍是纯文本。

---

## 8. 实现要点

1. `models.supports_vision`；admin 与公开模型接口；`sql/schema.sql` 和现有库 `ALTER`
2. 配置项：`VISION_FALLBACK_MODEL_ID`、`READ_IMAGE_MAX_BYTES`、`READ_IMAGE_MAX_EDGE`
3. `WorkspaceManager`：安全打开工作区相对路径（含子目录、防逃逸）；另增会话级 `images/` 读写（不出现在工作区文件 API 里）
4. 图片编解码 / 等比例缩放（独立小模块 `internal/imageproc`）；CallTool 时写入 `images/{store_key}`，组请求时读副本（EXIF 暂缓，见第 5 节）
5. 内置工具 `read_image`：校验、落副本和引用；非视觉时由 `ImageProvider` 通过 `UserContext.AnalyzeImage`（生成服务按 `VISION_FALLBACK_MODEL_ID` 惰性构造的 `AnalyzeImageFunc`）按 intent 调 fallback，把结果写入 `ReadImageResult.Analysis`
6. `ContextAssembler` 在组请求前复制工具交换记录，按通用图片引用（`store_key` / `analysis`）和第 4 节规则投影：把副本中的 `ToolResult.Result` 替换为瘦 JSON；当前模型能看图时凭 `store_key` 读 `images/`，填入临时的 `ToolResult.Image`。**生成器不读盘、不做策略**，只负责协议；`GenerationService` 不引用工具名；`ImageProvider` 不组视图；原始工具结果不修改、不回写。不要给 `ToolProvider` 加 `ProjectForModel`，也不要加 `ToolResultViewer`
7. Chat Completions：视觉时 `tool.content` 为 `[text, image_url]`；Responses：视觉时 `function_call_output` 带 `input_image`。主生成和摘要在当前模型能看图时都这样附；不插额外 user。`GenerateMessage.Image` 让普通 user 消息也能带图（`[text, image_url]` / `[input_text, input_image]`，空正文时只发图片段），目前只有 fallback 描述请求使用
8. token 粗估按图片张数定额
9. README / `docs/api.md` / `.env.example`

过滤器作用范围：

- **当轮**（同一条 `Process` 里工具刚返回）：CallTool 的完整引用用于落库 / SSE；下一轮组请求由组装器 `ViewResult` 按第 4 节表生成瘦视图并按需读副本。
- **历史重放与摘要**：复用同一套 `View`，不回写库，不重新调用 fallback。
- 不引入单独的 `ModelView` 字段。历史记录在进入生成上下文时投影一次；当轮新工具结果先落库 / SSE，再投影一个副本追加到内存上下文。

## 9. 已实现的现状与偏离记录

进度核对：2026-09-10，基线 HEAD `72e488e`，工作区另有未提交改动；以下以当前源码为准。同日确认：模型视图由 `ContextAssembler` 按通用图片引用装载，不引入 `ToolResultViewer`，工具 schema 不进 `service`。

已完成：

- `supports_vision` 已贯通 entity、仓储、服务、admin/公开模型接口和 `sql/schema.sql`；`.env.example`、`docs/api.md` 及三个读图配置项也已补齐。现有数据库是否执行过对应 `ALTER` 仍未核实。
- 下载 / 删除和 `read_image` 只接受 `/workspace/...` 路径并支持子目录；Linux 使用 `O_NOFOLLOW` 打开文件，逐段 symlink 检查已实现，但并发替换目录的竞态仍未完全消除。
- 会话级 `images/` 存储、`OpenStoredImage`/`WriteStoredImage` 和 JPEG/PNG/WebP 检测、解码、最长边缩放、再编码已实现；副本按原始字节 SHA-256 寻址，随会话目录删除。
- `ImageProvider` 已注册且不依赖 `SANDBOX_ENABLED`；`GenerationService` 已把当前模型的 `SupportsVision` 传入 `ListTools` 和 `CallTool`。`read_image` 对视觉和非视觉模型都会出现在工具列表中。
- 通用运输结构已有：`GenerateImage`、标记为 `json:"-"` 的 `ToolResult.Image`，以及 `GenerateMessage.Image`（仅 fallback 请求使用）。
- `ToolReadImage` / `ReadImageResult` 已在 `internal/tool`。
- `ContextAssembler` 已抽出；`Process` 调度 `BindHistory` / `View` / `Prefix` / `Round`。组请求时按 `store_key` / `analysis` 投影，视觉模型读 `images/` 填 `Image`；落库 / SSE 仍用原始引用。
- Chat Completions 的 `[text, image_url]`、Responses 的 `input_image`，以及 token 按张定额（1200）已接上。摘要与主生成共用 `View`。
- 非视觉模型的 fallback 已接通：`GenerationService` 按 `VISION_FALLBACK_MODEL_ID` 惰性构造 `AnalyzeImageFunc` 放进 `UserContext.AnalyzeImage`；`ImageProvider.readImage` 在 `!SupportsVision && AnalyzeImage != nil` 时用**缩放后的副本**和必填 `intent` 调用，成功则写入 `ReadImageResult.Analysis`。fallback 失败只写日志、不写进结果 JSON，也不阻断工具结果落库；`ctx` 取消时向上返回取消错误。

当前 `read_image` 工具的实际返回策略：

- 成功时只返回引用 JSON（`ok`、`path`、原图/副本 MIME、尺寸、大小、`store_key`、必填 `intent`），图片字节只写入会话 `images/`，不写入工具结果 JSON、数据库字段或 SSE。发给模型的瘦视图不含 `intent`。
- 失败时返回 `{"ok":false,"error":"..."}`。非视觉模型且配置了 fallback 时，工具会调用 fallback 并把描述写入 `analysis`；fallback 自身失败只写日志，结果 JSON 仍是成功引用，也不带 error。
- 发给模型的视图由组装器按当前模型过滤：视觉发 `{"ok":true}` 加副本图片；非视觉无 analysis 发“不支持视觉”；有 analysis 则只发分析文本。

尚未完成：

- 副本临时文件写完后原子发布、路径检查与读写之间的完整竞态防护、解码前像素数上限。
- 端到端和单元测试；当前 `go test ./...` 通过，但仓库没有 `*_test.go`。用真实视觉模型验收「能看图」仍待做。

偏离初稿：

- `ToolResult.Parts` 和单独的 `ModelView` **都不做**。`analysis` 保存在 `ReadImageResult` 的原始 JSON 中。
- 不给 `ToolProvider` 加 `ProjectForModel`，也不加 `ToolResultViewer`。模型视图由 `ContextAssembler` 认 `store_key` / `analysis` 并读盘完成；`ImageProvider` 只负责 CallTool。生成器只消费文本与图片字节，不识别 `store_key`。
- `ToolReadImage` / `ReadImageResult` 属于 `internal/tool`，不属于 `service`。组装器只用图片引用字段子集，不解完整工具结果类型。
- fallback 的注入形态从“构造 `ImageProvider` 时注入 `VisionAnalyzer` 接口”改为“`UserContext.AnalyzeImage` 函数字段”，由 `GenerationService` 惰性构造（`newVisionAnalyzer`）。好处：`ImageProvider` 构造签名不变、main 里不用重排依赖、`tool` 不依赖 `ModelService`/`GeneratorFactory`、未读图的请求零开销；代价是 `UserContext` 从纯数据变成带能力的数据（4 个 provider 里只有 `ImageProvider` 用得上）。
- 只接受 `/workspace/...` 开头的路径（初稿的 `foo.png` 相对形式放弃）
- GIF 不支持；APNG 暂不拦截（以后做）
- EXIF 未做（以后做）
- 输出编码体积上限/质量阶梯去掉；JPEG 输出质量 q90；JPEG→JPEG，PNG/WebP→PNG

已知修复项：

- 非 Linux 编译失败：`open_regular_other.go` 的 `createExclusiveNoFollow` 中 `info` 未使用；前置 `Lstat` 本身也可移除。
- 路径检查与实际读 / 删之间存在竞态；使用受工作区目录句柄约束的操作，并保留普通文件限制。
- `path.Clean("/" + p)` 会把越界输入重映射回工作区（如 `/workspace/../x` 被解析成 `x` 而非拒绝）。需要按路径段检查。~~`/..` 前缀判断误拒 `..notes.txt`~~（已随子目录列表/上传改动删除该判断，2026-09-11）。
- 副本文件在写完前就可见，已有空文件会被当成成功缓存；需要临时文件写完后原子发布。
- 原图只限制压缩后的字节数，尚无解码前像素数上限；完整解码后再缩放无法限制解码内存。

后续编码顺序：

1. [x] 工具入口上下文：生成服务已把 `SupportsVision: modelConfig.SupportsVision` 传入 `ListTools` / `CallTool`，`ctx` 也已传入 `readImage`；工具仍对非视觉模型开放。
2. [x] 通用运输结构：`GenerateImage` 与 `ToolResult.Image` 在 `service`；`ToolReadImage` / `ReadImageResult` 在 `internal/tool`。
3. [x] 抽出 `ContextAssembler` 并接通生成链路：历史 / 当轮 / 摘要走组装器 `View`（按 `store_key` / `analysis` 投影并读盘）；Chat Completions / Responses 序列化临时 `Image`；token 估算使用瘦文本 + 图片定额。压缩循环仍在 `Process`。
4. [x] 最后接 fallback：`ImageProvider.readImage` 只在非视觉模型实际调用时通过 `UserContext.AnalyzeImage` 执行；fallback 模型必须启用且支持视觉。保存 `ReadImageResult.Analysis`，后续重放只读已有结果，不补分析。`internal/imageproc` 保持纯图片处理。

第 4 项落地情况：

- 校验（已启用 + `supports_vision=true`）在**每次调用分析时**进行，不在启动或请求建立阶段做，因此配置改了立刻生效，也不会因为环境变量写错而让进程起不来。
- `VISION_FALLBACK_MODEL_ID == 0` 时 `AnalyzeImage` 为 nil，`ImageProvider` 不启用分析；非视觉模型仍走“不支持视觉”视图。
- fallback 请求不带工具，且用缩放后的副本、按必填 `intent` 提问。
- 分析失败不写进结果 JSON（否则同一条记录换视觉模型重放时会显示为失败而不是附图），只打日志；请求取消沿 `ctx` 向上返回。

第 3 项验收点：

- 相同引用交给视觉模型：生成视图为 `{"ok":true}` + 一张图；交给无 analysis 的非视觉模型：无图且明确不支持。
- 已有 analysis 的非视觉视图直接返回 analysis；原图删除不影响副本读取；副本缺失转为工具视图失败，当前模型可用的 analysis 仍能保留。
- 原始引用 JSON 不变；落库 / SSE 不含图片字节或 data URL。
- 同一次生成内，同一副本最多读取一次；不同用户 / 会话的缓存不混用。
- `GenerationService` 不引用 `ToolReadImage` 或 `ReadImageResult`。组装器只偷看 `store_key` / `analysis` / `stored_mime_type`，不解完整工具结果类型。

检查结果：Linux `go test ./...` 通过，但目前没有 `*_test.go`；Windows sandbox 交叉编译仍因 `open_regular_other.go` 的未使用变量失败。编译通过不代表读图端到端链路已完成。

不要把像素写入 `result_json`，也不要在 SSE 里推像素。
