# Veo 视频任务

视频接口采用长任务模式：创建任务、轮询状态、下载内容。

## 创建任务

`POST /v1/videos` 返回 `202 Accepted`：

```bash
curl http://localhost:8800/v1/videos \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "veo-3.1-fast-generate-preview",
    "prompt": "一只橘猫在雨后的东京街头奔跑",
    "seconds": 4,
    "aspect_ratio": "16:9",
    "resolution": "720p"
  }'
```

响应中的 `id` 用于后续查询：

```json
{
  "id": "video_abc123",
  "object": "video",
  "status": "in_progress",
  "model": "veo-3.1-fast-generate-preview",
  "created_at": 1760000000,
  "seconds": "4"
}
```

## 查询和下载

```bash
curl http://localhost:8800/v1/videos/video_abc123 \
  -H 'Authorization: Bearer sk-xxxxxxxx'

curl http://localhost:8800/v1/videos/video_abc123/content \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -o output.mp4
```

任务状态为 `completed` 后才能下载。生成中下载会返回 `409`；上游失败会返回包含错误信息的 `failed` 任务或 `502`。
