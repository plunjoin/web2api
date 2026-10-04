# 多模态透传

在引擎 B 的 `upstream` 模式下，以下路径会将请求透传到上游 AI Studio：

| 路径 | 用途 |
| --- | --- |
| `POST /v1/images/*` | 图片生成或编辑 |
| `POST /v1/videos/*` | 上游视频协议 |
| `POST /v1/audio/*` | 音频相关请求 |
| `POST /v1/files/*` | 文件上传和处理 |
| `POST /v1/embeddings/*` | Embedding 请求 |

透传请求仍然使用 `Authorization: Bearer sk-...`。JSON 请求使用 `Content-Type: application/json`，二进制内容使用 `application/octet-stream`。路径和请求体字段由对应上游协议决定，使用前请确认配置中的 `mode: upstream`。

普通聊天和 Veo 长任务优先使用标准化的 `/v1/chat/completions` 与 `/v1/videos`，这样可以获得稳定的 OpenAI 风格响应。
