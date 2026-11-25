// model.go Flow 引擎的模型目录。
//
// 表是从上游 flow2api 的 MODEL_CONFIG 逐条抄下来的：每个公开模型 ID 对应一个
// 上游模型键（图片的 model_name / 视频的 model_key）与宽高比枚举。
// 上游用的是位置协议，宽高比要变成枚举整数，所以目录里存枚举字符串。

package flow

// imageModel 是一条图片模型记录。
type imageModel struct {
	// ID 公开模型 ID。
	ID string
	// DisplayName 展示名。
	DisplayName string
	// ModelName 上游模型名（ogiZ0b 的 request[5]）。
	ModelName string
	// AspectRatio 上游宽高比枚举字符串。
	AspectRatio string
	// Upsample 放大目标（空表示不放大）。
	Upsample string
}

// videoModel 是一条视频模型记录。
type videoModel struct {
	// ID 公开模型 ID。
	ID string
	// DisplayName 展示名。
	DisplayName string
	// ModelKey 上游模型键（视频请求的 request[1]）。
	ModelKey string
	// AspectRatio 上游宽高比枚举字符串。
	AspectRatio string
	// Resolution 上游分辨率枚举字符串（空表示不指定）。
	Resolution string
	// VideoType 是生成模式：t2v / i2v / r2v / omni / extend。
	VideoType string
	// ReferenceKey 是 Omni 带参考图时使用的模型键。
	ReferenceKey string
	// MinImages / MaxImages 是参考图数量范围。
	MinImages int
	MaxImages int
	// ReferenceDuration 是 Omni 视频时长（秒）。
	ReferenceDuration int
}

// imageModels 是 Flow 支持的图片模型目录。
var imageModels = []imageModel{
	{ID: "gemini-3.1-flash-image-landscape", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE", Upsample: ""},
	{ID: "gemini-3.1-flash-image-landscape-2k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-image-landscape-4k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-image-portrait", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT", Upsample: ""},
	{ID: "gemini-3.1-flash-image-portrait-2k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-image-portrait-4k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-image-square", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_SQUARE", Upsample: ""},
	{ID: "gemini-3.1-flash-image-square-2k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_SQUARE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-image-square-4k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_SQUARE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-image-four-three", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE_FOUR_THREE", Upsample: ""},
	{ID: "gemini-3.1-flash-image-four-three-2k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE_FOUR_THREE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-image-four-three-4k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE_FOUR_THREE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-image-three-four", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT_THREE_FOUR", Upsample: ""},
	{ID: "gemini-3.1-flash-image-three-four-2k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT_THREE_FOUR", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-image-three-four-4k", DisplayName: "Nano Banana 2.1", ModelName: "BELUGA", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT_THREE_FOUR", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-lite-image-landscape", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE", Upsample: ""},
	{ID: "gemini-3.1-flash-lite-image-landscape-2k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-lite-image-landscape-4k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-lite-image-portrait", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT", Upsample: ""},
	{ID: "gemini-3.1-flash-lite-image-portrait-2k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-lite-image-portrait-4k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-lite-image-square", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_SQUARE", Upsample: ""},
	{ID: "gemini-3.1-flash-lite-image-square-2k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_SQUARE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-lite-image-square-4k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_SQUARE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-lite-image-four-three", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE_FOUR_THREE", Upsample: ""},
	{ID: "gemini-3.1-flash-lite-image-four-three-2k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE_FOUR_THREE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-lite-image-four-three-4k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE_FOUR_THREE", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
	{ID: "gemini-3.1-flash-lite-image-three-four", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT_THREE_FOUR", Upsample: ""},
	{ID: "gemini-3.1-flash-lite-image-three-four-2k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT_THREE_FOUR", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_2K"},
	{ID: "gemini-3.1-flash-lite-image-three-four-4k", DisplayName: "Nano Banana 2 Lite", ModelName: "HARBOR_SEAL", AspectRatio: "IMAGE_ASPECT_RATIO_PORTRAIT_THREE_FOUR", Upsample: "UPSAMPLE_IMAGE_RESOLUTION_4K"},
}

var videoModels = []videoModel{
	{ID: "veo-3.1-lite-4s-landscape", DisplayName: "Veo 3.1 - Lite · 4s · landscape", ModelKey: "veo_3_1_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite-4s-portrait", DisplayName: "Veo 3.1 - Lite · 4s · portrait", ModelKey: "veo_3_1_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite-6s-landscape", DisplayName: "Veo 3.1 - Lite · 6s · landscape", ModelKey: "veo_3_1_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite-6s-portrait", DisplayName: "Veo 3.1 - Lite · 6s · portrait", ModelKey: "veo_3_1_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite-8s-landscape", DisplayName: "Veo 3.1 - Lite · 8s · landscape", ModelKey: "veo_3_1_t2v", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite-8s-portrait", DisplayName: "Veo 3.1 - Lite · 8s · portrait", ModelKey: "veo_3_1_t2v", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast-4s-landscape", DisplayName: "Veo 3.1 - Fast · 4s · landscape", ModelKey: "veo_3_1_t2v_fast_4s_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast-4s-portrait", DisplayName: "Veo 3.1 - Fast · 4s · portrait", ModelKey: "veo_3_1_t2v_fast_4s_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast-6s-landscape", DisplayName: "Veo 3.1 - Fast · 6s · landscape", ModelKey: "veo_3_1_t2v_fast_6s_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast-6s-portrait", DisplayName: "Veo 3.1 - Fast · 6s · portrait", ModelKey: "veo_3_1_t2v_fast_6s_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast-8s-landscape", DisplayName: "Veo 3.1 - Fast · 8s · landscape", ModelKey: "veo_3_1_t2v_fast", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast-8s-portrait", DisplayName: "Veo 3.1 - Fast · 8s · portrait", ModelKey: "veo_3_1_t2v_fast_portrait", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-4s-landscape", DisplayName: "Veo 3.1 - Quality · 4s · landscape", ModelKey: "veo_3_1_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-4s-portrait", DisplayName: "Veo 3.1 - Quality · 4s · portrait", ModelKey: "veo_3_1_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-6s-landscape", DisplayName: "Veo 3.1 - Quality · 6s · landscape", ModelKey: "veo_3_1_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-6s-portrait", DisplayName: "Veo 3.1 - Quality · 6s · portrait", ModelKey: "veo_3_1_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-8s-landscape", DisplayName: "Veo 3.1 - Quality · 8s · landscape", ModelKey: "veo_3_1_t2v", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-8s-portrait", DisplayName: "Veo 3.1 - Quality · 8s · portrait", ModelKey: "veo_3_1_t2v_portrait", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "omni-1.1-flash-4s-landscape", DisplayName: "Omni 1.1 Flash · 4s · landscape", ModelKey: "abra_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "abra_r2v_4s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 4},
	{ID: "omni-1.1-flash-4s-portrait", DisplayName: "Omni 1.1 Flash · 4s · portrait", ModelKey: "abra_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "abra_r2v_4s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 4},
	{ID: "omni-1.1-flash-6s-landscape", DisplayName: "Omni 1.1 Flash · 6s · landscape", ModelKey: "abra_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "abra_r2v_6s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 6},
	{ID: "omni-1.1-flash-6s-portrait", DisplayName: "Omni 1.1 Flash · 6s · portrait", ModelKey: "abra_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "abra_r2v_6s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 6},
	{ID: "omni-1.1-flash-8s-landscape", DisplayName: "Omni 1.1 Flash · 8s · landscape", ModelKey: "abra_t2v_8s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "abra_r2v_8s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 8},
	{ID: "omni-1.1-flash-8s-portrait", DisplayName: "Omni 1.1 Flash · 8s · portrait", ModelKey: "abra_t2v_8s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "abra_r2v_8s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 8},
	{ID: "omni-1.1-flash-10s-landscape", DisplayName: "Omni 1.1 Flash · 10s · landscape", ModelKey: "abra_t2v_10s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "abra_r2v_10s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 10},
	{ID: "omni-1.1-flash-10s-portrait", DisplayName: "Omni 1.1 Flash · 10s · portrait", ModelKey: "abra_t2v_10s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "abra_r2v_10s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 10},
	{ID: "omni-1.2-flash-3s-landscape", DisplayName: "Omni 1.2 Flash · 3s · landscape", ModelKey: "macaron_cookie_t2v_3s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_3s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 3},
	{ID: "omni-1.2-flash-3s-portrait", DisplayName: "Omni 1.2 Flash · 3s · portrait", ModelKey: "macaron_cookie_t2v_3s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_3s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 3},
	{ID: "omni-1.2-flash-4s-landscape", DisplayName: "Omni 1.2 Flash · 4s · landscape", ModelKey: "macaron_cookie_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_4s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 4},
	{ID: "omni-1.2-flash-4s-portrait", DisplayName: "Omni 1.2 Flash · 4s · portrait", ModelKey: "macaron_cookie_t2v_4s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_4s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 4},
	{ID: "omni-1.2-flash-5s-landscape", DisplayName: "Omni 1.2 Flash · 5s · landscape", ModelKey: "macaron_cookie_t2v_5s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_5s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 5},
	{ID: "omni-1.2-flash-5s-portrait", DisplayName: "Omni 1.2 Flash · 5s · portrait", ModelKey: "macaron_cookie_t2v_5s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_5s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 5},
	{ID: "omni-1.2-flash-6s-landscape", DisplayName: "Omni 1.2 Flash · 6s · landscape", ModelKey: "macaron_cookie_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_6s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 6},
	{ID: "omni-1.2-flash-6s-portrait", DisplayName: "Omni 1.2 Flash · 6s · portrait", ModelKey: "macaron_cookie_t2v_6s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_6s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 6},
	{ID: "omni-1.2-flash-7s-landscape", DisplayName: "Omni 1.2 Flash · 7s · landscape", ModelKey: "macaron_cookie_t2v_7s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_7s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 7},
	{ID: "omni-1.2-flash-7s-portrait", DisplayName: "Omni 1.2 Flash · 7s · portrait", ModelKey: "macaron_cookie_t2v_7s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_7s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 7},
	{ID: "omni-1.2-flash-8s-landscape", DisplayName: "Omni 1.2 Flash · 8s · landscape", ModelKey: "macaron_cookie_t2v_8s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_8s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 8},
	{ID: "omni-1.2-flash-8s-portrait", DisplayName: "Omni 1.2 Flash · 8s · portrait", ModelKey: "macaron_cookie_t2v_8s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_8s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 8},
	{ID: "omni-1.2-flash-9s-landscape", DisplayName: "Omni 1.2 Flash · 9s · landscape", ModelKey: "macaron_cookie_t2v_9s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_9s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 9},
	{ID: "omni-1.2-flash-9s-portrait", DisplayName: "Omni 1.2 Flash · 9s · portrait", ModelKey: "macaron_cookie_t2v_9s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_9s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 9},
	{ID: "omni-1.2-flash-10s-landscape", DisplayName: "Omni 1.2 Flash · 10s · landscape", ModelKey: "macaron_cookie_t2v_10s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_10s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 10},
	{ID: "omni-1.2-flash-10s-portrait", DisplayName: "Omni 1.2 Flash · 10s · portrait", ModelKey: "macaron_cookie_t2v_10s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_10s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 10},
	{ID: "omni-1.2-flash-11s-landscape", DisplayName: "Omni 1.2 Flash · 11s · landscape", ModelKey: "macaron_cookie_t2v_11s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_11s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 11},
	{ID: "omni-1.2-flash-11s-portrait", DisplayName: "Omni 1.2 Flash · 11s · portrait", ModelKey: "macaron_cookie_t2v_11s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_11s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 11},
	{ID: "omni-1.2-flash-12s-landscape", DisplayName: "Omni 1.2 Flash · 12s · landscape", ModelKey: "macaron_cookie_t2v_12s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_12s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 12},
	{ID: "omni-1.2-flash-12s-portrait", DisplayName: "Omni 1.2 Flash · 12s · portrait", ModelKey: "macaron_cookie_t2v_12s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_12s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 12},
	{ID: "omni-1.2-flash-13s-landscape", DisplayName: "Omni 1.2 Flash · 13s · landscape", ModelKey: "macaron_cookie_t2v_13s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_13s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 13},
	{ID: "omni-1.2-flash-13s-portrait", DisplayName: "Omni 1.2 Flash · 13s · portrait", ModelKey: "macaron_cookie_t2v_13s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_13s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 13},
	{ID: "omni-1.2-flash-14s-landscape", DisplayName: "Omni 1.2 Flash · 14s · landscape", ModelKey: "macaron_cookie_t2v_14s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_14s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 14},
	{ID: "omni-1.2-flash-14s-portrait", DisplayName: "Omni 1.2 Flash · 14s · portrait", ModelKey: "macaron_cookie_t2v_14s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_14s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 14},
	{ID: "omni-1.2-flash-15s-landscape", DisplayName: "Omni 1.2 Flash · 15s · landscape", ModelKey: "macaron_cookie_t2v_15s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_15s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 15},
	{ID: "omni-1.2-flash-15s-portrait", DisplayName: "Omni 1.2 Flash · 15s · portrait", ModelKey: "macaron_cookie_t2v_15s", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ReferenceKey: "macaron_cookie_r2v_15s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 15},
	{ID: "omni-1.1-flash", DisplayName: "Omni 1.1 Flash · 8s · landscape", ModelKey: "abra_t2v_8s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "abra_r2v_8s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 15},
	{ID: "omni-1.2-flash", DisplayName: "Omni 1.2 Flash · 8s · landscape", ModelKey: "macaron_cookie_t2v_8s", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", ReferenceKey: "macaron_cookie_r2v_8s", VideoType: "omni", MinImages: 0, MaxImages: 3, ReferenceDuration: 15},
	{ID: "veo-3.1-lite-i2v-4s-landscape", DisplayName: "Veo 3.1 - Lite I2V · 4s · landscape", ModelKey: "veo_3_1_i2v_s_lite_4s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-lite-i2v-4s-portrait", DisplayName: "Veo 3.1 - Lite I2V · 4s · portrait", ModelKey: "veo_3_1_i2v_s_lite_4s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-lite-i2v-6s-landscape", DisplayName: "Veo 3.1 - Lite I2V · 6s · landscape", ModelKey: "veo_3_1_i2v_s_lite_6s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-lite-i2v-6s-portrait", DisplayName: "Veo 3.1 - Lite I2V · 6s · portrait", ModelKey: "veo_3_1_i2v_s_lite_6s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-fast-i2v-4s-landscape", DisplayName: "Veo 3.1 - Fast I2V · 4s · landscape", ModelKey: "veo_3_1_i2v_s_fast_4s_fl_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-fast-i2v-4s-portrait", DisplayName: "Veo 3.1 - Fast I2V · 4s · portrait", ModelKey: "veo_3_1_i2v_s_fast_4s_fl_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-fast-i2v-6s-landscape", DisplayName: "Veo 3.1 - Fast I2V · 6s · landscape", ModelKey: "veo_3_1_i2v_s_fast_6s_fl_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-fast-i2v-6s-portrait", DisplayName: "Veo 3.1 - Fast I2V · 6s · portrait", ModelKey: "veo_3_1_i2v_s_fast_6s_fl_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-quality-i2v-4s-landscape", DisplayName: "Veo 3.1 - Quality I2V · 4s · landscape", ModelKey: "veo_3_1_i2v_s_quality_4s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-quality-i2v-4s-portrait", DisplayName: "Veo 3.1 - Quality I2V · 4s · portrait", ModelKey: "veo_3_1_i2v_s_quality_4s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-quality-i2v-6s-landscape", DisplayName: "Veo 3.1 - Quality I2V · 6s · landscape", ModelKey: "veo_3_1_i2v_s_quality_6s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-quality-i2v-6s-portrait", DisplayName: "Veo 3.1 - Quality I2V · 6s · portrait", ModelKey: "veo_3_1_i2v_s_quality_6s_fl", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "i2v", MinImages: 1, MaxImages: 2},
	{ID: "veo-3.1-fast-r2v-8s-landscape", DisplayName: "Veo 3.1 - Fast R2V · 8s · landscape", ModelKey: "veo_3_1_r2v_fast_landscape", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "r2v", MinImages: 1, MaxImages: 3},
	{ID: "veo-3.1-fast-r2v-8s-portrait", DisplayName: "Veo 3.1 - Fast R2V · 8s · portrait", ModelKey: "veo_3_1_r2v_fast_portrait", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "r2v", MinImages: 1, MaxImages: 3},
	{ID: "veo-3.1-lite-r2v-8s-landscape", DisplayName: "Veo 3.1 - Lite R2V · 8s · landscape", ModelKey: "veo_3_1_r2v_lite", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "r2v", MinImages: 1, MaxImages: 3},
	{ID: "veo-3.1-lite-r2v-8s-portrait", DisplayName: "Veo 3.1 - Lite R2V · 8s · portrait", ModelKey: "veo_3_1_r2v_lite", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "r2v", MinImages: 1, MaxImages: 3},
	{ID: "veo-3.1-fast-extend-8s-landscape", DisplayName: "Veo 3.1 - Fast Extend · landscape", ModelKey: "veo_3_1_extend_fast_landscape_ultra_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "extend", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast-extend-8s-portrait", DisplayName: "Veo 3.1 - Fast Extend · portrait", ModelKey: "veo_3_1_extend_fast_portrait_ultra_relaxed", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "extend", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-extend-8s-landscape", DisplayName: "Veo 3.1 - Quality Extend · landscape", ModelKey: "veo_3_1_extend_landscape", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "extend", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality-extend-8s-portrait", DisplayName: "Veo 3.1 - Quality Extend · portrait", ModelKey: "veo_3_1_extend_portrait", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "extend", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite-extend-8s-landscape", DisplayName: "Veo 3.1 - Lite Extend · landscape", ModelKey: "veo_3_1_extension_lite", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "extend", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite-extend-8s-portrait", DisplayName: "Veo 3.1 - Lite Extend · portrait", ModelKey: "veo_3_1_extension_lite", AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", VideoType: "extend", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-lite", DisplayName: "Veo 3.1 - Lite · 8s · landscape", ModelKey: "veo_3_1_t2v", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-fast", DisplayName: "Veo 3.1 - Fast · 8s · landscape", ModelKey: "veo_3_1_t2v_fast", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
	{ID: "veo-3.1-quality", DisplayName: "Veo 3.1 - Quality · 8s · landscape", ModelKey: "veo_3_1_t2v", AspectRatio: "VIDEO_ASPECT_RATIO_LANDSCAPE", VideoType: "t2v", MinImages: 0, MaxImages: 0},
}
