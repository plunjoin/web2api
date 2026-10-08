# Veo 视频任务

视频接口采用长任务模式：创建任务、轮询状态、下载内容。以下三条标准路由要求引擎 B 启用且处于 `native` 模式；`upstream` 模式不会将这三条路由转发到上游。

使用前调用 `GET /v1/models`，确认实际 Veo 模型 ID，并确认账户具备生成资格和可用额度。当前只支持文本生成视频，每次一个结果；参考图、首尾帧和负向提示词等扩展字段尚未映射。

## 创建任务

`POST /v1/videos` 返回 `202 Accepted`：

```bash
curl http://localhost:8800/v1/videos \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "veo-3.1-fast-generate-preview",
    "prompt": "一只橘猫在雨后的东京街头奔跑",
    "seconds": 8,
    "aspect_ratio": "16:9",
    "size": "4k"
  }'
```

### 请求参数

| 参数 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `model` | 是 | 无 | 支持视频长任务的实际 Veo 模型 ID |
| `prompt` | 是 | 无 | 视频内容描述，不能全为空白 |
| `seconds` | 否 | `4` | 时长（秒），整数或整数文本，如 `8` 或 `"8"`；常见 4/6/8，实际以模型目录为准 |
| `duration_seconds` | 否 | 无 | `seconds` 的整数别名，非零时优先；不要同时指定两者 |
| `aspect_ratio` | 否 | `16:9` | 常见 `16:9`、`9:16`，实际以模型目录为准 |
| `size` | 否 | 无 | 生成分辨率。取值 `360p`、`720p`、`1080p`、`4k`，`4k` 的 k 小写。网关把这个字符串原样交给上游。4K 用这个字段，不要用 `resolution` |
| `resolution` | 否 | `720p` | 不是本路由的 4K 字段。若另外传了合法分辨率，它会覆盖 `size` |

服务默认值不代表所有模型都接受对应组合。`veo-3.1-fast-generate-preview` 上，`size: "4k"` 配默认 4 秒会被上游拒绝，原文是 `4k is not supported for a duration of 4 seconds.` 同一模型改成 `seconds: 8` 后创建返回 HTTP 202，成片为 h264、3840×2160、时长 8.0 秒。`veo-3.1-generate-preview` 未验证：当时没有符合条件的 AI Studio 账户。

`veo-3.1-fast-generate-preview` 的 4K 请求体：

```json
{
  "model": "veo-3.1-fast-generate-preview",
  "prompt": "a red circle moving once",
  "size": "4k",
  "seconds": 8
}
```

创建成功返回 `202`。响应中的 `id` 用于后续查询，此时还没有视频文件：

```json
{
  "id": "video_abc123",
  "object": "video",
  "status": "in_progress",
  "model": "veo-3.1-fast-generate-preview",
  "created_at": 1760000000,
  "seconds": "8",
  "size": "1920x1080"
}
```

## 查询和下载

将示例 `video_abc123` 替换为创建响应中返回的真实 `id`。

```bash
curl http://localhost:8800/v1/videos/video_abc123 \
  -H 'Authorization: Bearer sk-xxxxxxxx'

curl http://localhost:8800/v1/videos/video_abc123/content \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -o output.mp4
```

| `status` | 下一步 |
| --- | --- |
| `in_progress` | 等待数秒后继续查询 |
| `completed` | 使用 `output.url` 或 `/v1/videos/{id}/content` 下载 MP4，仍需 API Key |
| `failed` | 已结束但没有输出文件，停止轮询；当前响应不附带上游详细失败原因 |

完成的查询响应示例：

```json
{
  "id": "video_abc123",
  "object": "video",
  "status": "completed",
  "model": "veo-3.1-fast-generate-preview",
  "created_at": 1760000000,
  "seconds": "8",
  "size": "1920x1080",
  "output": {
    "file_id": "file_example",
    "mime_type": "video/mp4",
    "url": "/v1/videos/video_abc123/content"
  }
}
```

下载成功返回视频二进制。未完成或没有输出文件时返回 `409`；无效的生成参数返回 `400`；上游协议调用失败返回 `502`；上游限流、配额不足或可用账户均在冷却时返回 `429`，错误类型为 `rate_limit_error`。

## 配额错误排查

如果创建任务返回 `HTTP 429`、协议错误码 `8`，并提示 `You exceeded your current quota`，说明 Google 上游拒绝了这次生成请求。该提示本身无法区分分钟限流、每日额度耗尽或计费资格问题；需要结合上游配额明细判断。

先使用引擎B实际选中的 Google 账户，在 AI Studio 网页中选择相同的 Veo 模型和参数创建视频。若网页也失败，检查该账户的视频生成资格、关联项目的计费状态，以及 [Google 配额页面](https://ai.dev/rate-limit) 中对应模型的限制和用量。只有确认是可恢复的限流或额度重置，等待后重试才有意义。

账户池遇到上游 `429` 会记录冷却并尝试其他符合条件的账户。若没有可用账户，请恢复上游额度或配置具备视频生成资格且有剩余额度的账户。模型出现在目录中、聊天可用或账户显示 Pro/Ultra，均不能保证这次 Veo 生成有可用额度。

请求中的 `seconds: 8`、`aspect_ratio: "16:9"`、`size: "4k"` 会按原值编码到视频协议。`4k` 是字符串，不是枚举数字。支持范围由上游实时模型目录校验。降低分辨率或缩短时长不能保证消除配额错误。

## 与 Gemini Interactions 视频的区别

本页描述 AI Studio 的 Veo 长任务 RPC，不是 Google Interactions 的 Gemini Omni 视频协议。虽然部分参数表达相似，不能互换请求或任务 ID。

| 官方 Interactions 参数 / 流程 | 本页 Veo 接口 |
| --- | --- |
| input 的 text / image / video 等 Content 块 | 只有 prompt 文本；参考图、首尾帧、编辑视频输入未接入 |
| generation_config.video_config.task：text_to_video / image_to_video / reference_to_video / edit / extend | 没有此配置对象，当前只开放文生视频 |
| response_format.video 的 aspect_ratio / resolution | 比例仍用顶层 `aspect_ratio`。分辨率用顶层 `size`（如 `4k`），不是官方 `response_format`，也不是聊天接口的 `resolution` |
| response_format.video.duration：如 "8s" | seconds：8 或 "8"；duration_seconds 为整数别名 |
| response_format.video.delivery / gcs_uri | 未开放内联 / URI 选择或 GCS 输出；统一从网关视频内容路由下载 |
| background、store、webhook_config、continuation_token | 没有这些官方生命周期参数；Veo 创建返回 202 的独立任务 |
| Interactions GET / cancel / DELETE / last_event_id | Veo 提供查询和下载；没有对应的交互取消、删除或 SSE 恢复 |

Omni 的输入限制、参考标签、任务类型和输出示例见[中文参考详解](/api/gemini-reference)，完整字段见[官方对照](/api/gemini-schema)。官方指南中的 3–10 秒或 360p 等取值不能直接套到 Veo；实际 Veo 范围以账号实时目录为准。
