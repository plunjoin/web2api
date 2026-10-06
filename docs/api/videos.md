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
    "resolution": "1080p"
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
| `resolution` | 否 | `720p` | 实际生成分辨率；常见 `720p`、`1080p`、`4k`，以模型和上游支持为准 |
| `size` | 否 | 自动推导 | 仅改变返回的尺寸标签，不控制生成分辨率，通常省略 |

服务默认值不代表所有模型都接受对应组合；高分辨率与时长的组合也受上游限制。示例 `8 秒 / 1080p` 需所选模型支持。

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

请求中的 `seconds: 8`、`aspect_ratio: "16:9"`、`resolution: "1080p"` 会按原值编码到视频协议，支持范围由上游实时模型目录校验。降低分辨率或缩短时长不能保证消除配额错误。
