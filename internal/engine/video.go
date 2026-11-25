// video.go 长任务视频的跨引擎公共类型。
//
// 视频是「提交 → 轮询 → 下载」三段式，AI Studio 原生 Veo 与 Google Flow
// 两套协议的请求/响应结构完全不同，但网关层只关心这四件事：
//
//   - 提交任务（拿到一个 operation ID）
//   - 轮询任务状态（完成 / 失败 / 进行中）
//   - 下载成品文件
//   - 记住这个 operation 属于哪个账号
//
// 所以把「网关看得见」的那部分抽到这里：网关与 provider.Manager 只依赖
// engine 包的类型，具体引擎（aistudio / flow）自己实现 VideoEngine 接口。
package engine

import (
	"context"
	"io"
	"time"
)

// VideoFile 是视频任务的成品文件引用。
type VideoFile struct {
	// ID 文件 ID（引擎内部标识，用于下载）。
	ID string `json:"id"`
	// Name 文件名。
	Name string `json:"name,omitempty"`
	// MIME MIME 类型。
	MIME string `json:"mime,omitempty"`
}

// VideoRequest 是一次长任务视频请求。
type VideoRequest struct {
	// Model 模型 ID（不带 models/ 前缀）。
	Model string
	// Prompt 提示词。
	Prompt string
	// Count 结果数量；当前上游一般只支持 1。
	Count int
	// AspectRatio 画幅比例，如 16:9 / 9:16 / 1:1。
	AspectRatio string
	// DurationSeconds 时长（秒）。
	DurationSeconds int
	// Resolution 分辨率：360p / 720p / 1080p / 4k。
	Resolution string
	// Size 输出尺寸（部分引擎用「宽x高」表达分辨率）。
	Size string
	// AccountID 指定账号（空则由引擎自行选号）。
	AccountID string
	// ImageURLs 参考图（文生图/图生视频的首帧）。
	ImageURLs []string
	// RecoverWAARuntime 由引擎A的 WAA 插件使用，Flow 忽略。
	RecoverWAARuntime func(context.Context, string, error) (bool, error)
}

// VideoOperation 是一次长任务视频的状态快照。
type VideoOperation struct {
	// ID operation ID（轮询用）。
	ID string
	// Done 任务是否已结束（成功或失败）。
	Done bool
	// File 成品文件；Done 且 File 非空表示成功。
	File *VideoFile
	// Model 模型 ID。
	Model string
	// Seconds 时长文案。
	Seconds string
	// Size 输出尺寸文案。
	Size string
	// CreatedAt 任务创建时间。
	CreatedAt time.Time
}

// MediaStream 是下载得到的媒体流。
type MediaStream struct {
	// Body 响应体，调用方负责关闭。
	Body io.ReadCloser
	// MIME MIME 类型。
	MIME string
	// Name 文件名。
	Name string
	// Size 字节数（0 表示未知）。
	Size int64
}

// VideoEngine 由能承接长任务视频的引擎实现。
//
// 方法名刻意与各引擎内部已有的同名方法错开（引擎内部叫 GenerateVideo /
// GetGenerateVideoOperation），这样适配层不用改动引擎内部的既有代码。
//
// 轮询与下载必须能定位到「当初提交任务的那个账号」：引擎要么自己记住
// operation → 账号的映射，要么用账号无关的凭证完成后续请求。
type VideoEngine interface {
	// SubmitVideo 提交一个视频生成任务，返回 operation ID。
	SubmitVideo(ctx context.Context, request VideoRequest) (VideoOperation, error)
	// PollVideo 查询任务状态。
	PollVideo(ctx context.Context, operationID string) (VideoOperation, error)
	// FetchVideoFile 下载成品文件。
	FetchVideoFile(ctx context.Context, fileID string) (MediaStream, error)
}
