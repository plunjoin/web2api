// payload.go Flow 前端的 RPC 参数构造。
//
// 上游用的是「位置参数」协议：每个 RPC 的参数是一个定长数组，槽位顺序
// 固定，没有命名键。改错槽位顺序不会报错，只会拿到莫名其妙的结果，
// 所以下面的构造都按槽位注释写死。
package flow

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// 视频宽高比（上游枚举）。
const (
	aspectVideoPortrait  = 1
	aspectVideoSquare    = 0
	aspectVideoLandscape = 2
)

// 图片宽高比（上游枚举）。
const (
	aspectImageSquare    = 1
	aspectImagePortrait  = 2
	aspectImageLandscape = 3
	aspectImageThreeFour = 4
	aspectImageFourThree = 5
)

// 视频分辨率（上游枚举）。
const (
	resolution360p  = 1
	resolution720p  = 2
	resolution1080p = 3
	resolution4K    = 4
)

// upperUUID 生成一个大写 UUID4。
func upperUUID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败时用时间戳兜底：只是「请求批次 ID」，不必是密码学强度。
		now := uint64(time.Now().UnixNano())
		for i := range buf {
			buf[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	hexed := hex.EncodeToString(buf[:])
	return strings.ToUpper(hexed[0:8]) + "-" + strings.ToUpper(hexed[8:12]) + "-" +
		strings.ToUpper(hexed[12:16]) + "-" + strings.ToUpper(hexed[16:20]) + "-" +
		strings.ToUpper(hexed[20:32])
}

// randomSeed 生成 1..2147483647 的随机种子。
func randomSeed() int {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 1
	}
	value := int(buf[0])<<24 | int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])
	if value < 0 {
		value = -value
	}
	return value%2147483646 + 1
}

// projectContext 构造 clientContext（11 个槽位）。
//
// 槽位 10 是 [reCAPTCHA token, 1]——token 就放在这里。它在整个协议里
// 只有这一个位置：图片请求里出现两次（request[7] 与 argument[3]），
// 视频请求里出现一次（argument[1]）。
func projectContext(projectID, recaptchaToken string) []any {
	return []any{
		nil,                      // 0
		22,                       // 1 固定常量
		nil,                      // 2
		nil,                      // 3
		nil,                      // 4
		projectID,                // 5 项目 ID
		nil,                      // 6
		nil,                      // 7
		nil,                      // 8
		nil,                      // 9
		[]any{recaptchaToken, 1}, // 10 reCAPTCHA token
	}
}

// promptValue 构造提示词（三层嵌套）。
func promptValue(prompt string) []any {
	return []any{[]any{[]any{prompt}}}
}

// promptMessage 构造视频请求里的提示词槽。
func promptMessage(prompt string) []any {
	return []any{nil, nil, promptValue(prompt)}
}

// generationMetadata 构造视频请求里的 metadata 槽。
func generationMetadata(sessionID, requestID string) []any {
	return []any{nil, nil, nil, nil, sessionID, requestID}
}

// mediaReference 构造媒体引用槽。
func mediaReference(mediaID string) []any {
	return []any{nil, strings.TrimSpace(mediaID)}
}

// imageGenerationArgument 构造文生图（ogiZ0b）参数。
func imageGenerationArgument(projectID, prompt, modelName string, aspectRatio int, recaptchaToken, sessionID string, referenceMediaIDs []string) []any {
	context := projectContext(projectID, recaptchaToken)
	request := make([]any, 14)
	request[0] = nil                 // 0 未用
	request[1] = nil                 // 1 未用
	request[2] = nil                 // 2 参考图列表
	request[3] = randomSeed()        // 3 种子
	request[4] = aspectRatio         // 4 宽高比枚举
	request[5] = modelName           // 5 模型名
	request[6] = nil                 // 6 未用
	request[7] = context             // 7 clientContext
	request[8] = promptValue(prompt) // 8 提示词
	request[9] = nil                 // 9 必须为 nil（塞设置对象会被拒）
	request[10] = nil                // 10 未用
	request[11] = nil                // 11 未用
	request[12] = sessionID          // 12 会话 ID
	request[13] = upperUUID()        // 13 请求 UUID
	if len(referenceMediaIDs) > 0 {
		refs := make([]any, 0, len(referenceMediaIDs))
		for _, mediaID := range referenceMediaIDs {
			refs = append(refs, []any{mediaID, nil, nil, nil, 1})
		}
		request[2] = refs
		request = append(request, 4) // 14 有参考图时才有这个槽
	}
	return []any{
		nil,              // 0
		[]any{request},   // 1
		1,                // 2
		context,          // 3
		[]any{sessionID}, // 4
	}
}

// videoArgument 构造视频类 RPC 的外层参数（三槽位）。
func videoArgument(request []any, projectID, recaptchaToken string) []any {
	return []any{
		[]any{request}, // 0
		projectContext(projectID, recaptchaToken), // 1
		[]any{upperUUID(), 2},                     // 2
	}
}

// videoTextRequest 构造文生视频（YhhmEf）的 request。
func videoTextRequest(prompt, modelKey string, aspectRatio int, sessionID string, resolution *int) []any {
	request := make([]any, 8)
	request[0] = promptMessage(prompt)                      // 0 提示词
	request[1] = modelKey                                   // 1 模型键
	request[2] = aspectRatio                                // 2 宽高比枚举
	request[3] = nil                                        // 3 未用
	request[4] = generationMetadata(sessionID, upperUUID()) // 4 metadata
	request[5] = nil                                        // 5 未用
	request[6] = nil                                        // 6 output spec
	request[7] = resolution                                 // 7 分辨率
	return request
}

// videoReferencesRequest 构造参考图视频（MZZa6b）的 request。
func videoReferencesRequest(prompt, modelKey string, aspectRatio int, sessionID string, mediaIDs []string, resolution *int) []any {
	refs := make([]any, 0, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		refs = append(refs, mediaReference(mediaID))
	}
	request := make([]any, 12)
	request[0] = promptMessage(prompt)                      // 0
	request[1] = refs                                       // 1 参考图列表
	request[2] = modelKey                                   // 2
	request[3] = aspectRatio                                // 3
	request[4] = nil                                        // 4
	request[5] = generationMetadata(sessionID, upperUUID()) // 5 metadata
	request[6] = nil                                        // 6
	request[7] = nil                                        // 7
	request[8] = nil                                        // 8 output spec
	request[9] = nil                                        // 9
	request[10] = nil                                       // 10
	request[11] = resolution                                // 11 分辨率
	return request
}

// videoStartEndRequest 构造首尾帧视频（nprQif）的 request。
func videoStartEndRequest(prompt, modelKey string, aspectRatio int, sessionID, startMediaID, endMediaID string) []any {
	request := make([]any, 9)
	request[0] = promptMessage(prompt)                      // 0
	request[1] = modelKey                                   // 1
	request[2] = aspectRatio                                // 2
	request[3] = nil                                        // 3
	request[4] = mediaReference(startMediaID)               // 4 首帧
	request[5] = mediaReference(endMediaID)                 // 5 尾帧
	request[6] = generationMetadata(sessionID, upperUUID()) // 6 metadata
	request[7] = nil                                        // 7
	request[8] = nil                                        // 8 output spec
	return request
}

// videoStartRequest 构造首帧视频（eb1hJf）的 request。
func videoStartRequest(prompt, modelKey string, aspectRatio int, sessionID, startMediaID string, resolution *int) []any {
	request := make([]any, 9)
	request[0] = promptMessage(prompt)                      // 0
	request[1] = modelKey                                   // 1
	request[2] = aspectRatio                                // 2
	request[3] = nil                                        // 3
	request[4] = mediaReference(startMediaID)               // 4 首帧
	request[5] = generationMetadata(sessionID, upperUUID()) // 5 metadata
	request[6] = nil                                        // 6
	request[7] = nil                                        // 7
	request[8] = resolution                                 // 8 分辨率
	return request
}

// videoStatusArgument 构造视频状态轮询（jwpduf）参数。
func videoStatusArgument(operationIDs []string) []any {
	ids := make([]any, 0, len(operationIDs))
	for _, id := range operationIDs {
		ids = append(ids, []any{id})
	}
	return []any{nil, nil, ids}
}

// createProjectArgument 构造建项目（jHPbke）参数。
func createProjectArgument(title string) []any {
	return []any{"projects/*", []any{nil, []any{title}}, []any{nil, 22}}
}

// uploadImageArgument 构造上传参考图（maseQ）参数。
//
// 第 3 槽必须是整数 1 而不是布尔 true——布尔会被上游以 code=[3] 拒掉。
func uploadImageArgument(projectID, base64Image, mimeType, filename, recaptchaToken string) []any {
	return []any{
		projectContext(projectID, recaptchaToken), // 0 clientContext
		base64Image, // 1 图片 base64
		mimeType,    // 2 MIME
		1,           // 3 固定整数
		nil,         // 4
		nil,         // 5
		nil,         // 6
		nil,         // 7
		filename,    // 8 文件名
		nil,         // 9
		upperUUID(), // 10
		upperUUID(), // 11
	}
}

// videoAspectRatio 把用户输入的画幅映射成上游枚举。
func videoAspectRatio(aspect string) int {
	normalized := strings.ToUpper(strings.TrimSpace(aspect))
	switch {
	case strings.Contains(normalized, "PORTRAIT"), strings.Contains(normalized, "9:16"):
		return aspectVideoPortrait
	case strings.Contains(normalized, "SQUARE"), strings.Contains(normalized, "1:1"):
		return aspectVideoSquare
	default:
		return aspectVideoLandscape
	}
}

// imageAspectRatio 把用户输入的画幅映射成上游枚举。
func imageAspectRatio(aspect string) (int, error) {
	normalized := strings.ToUpper(strings.TrimSpace(aspect))
	normalized = strings.ReplaceAll(normalized, "：", ":")
	normalized = strings.ReplaceAll(normalized, "/", ":")
	normalized = strings.ReplaceAll(normalized, " ", "")
	switch normalized {
	case "16:9", "LANDSCAPE", "16:9:":
		return aspectImageLandscape, nil
	case "9:16", "PORTRAIT":
		return aspectImagePortrait, nil
	case "1:1", "SQUARE":
		return aspectImageSquare, nil
	case "4:3", "FOUR_THREE", "FOUR-THREE", "LANDSCAPE_FOUR_THREE":
		return aspectImageFourThree, nil
	case "3:4", "THREE_FOUR", "THREE-FOUR", "PORTRAIT_THREE_FOUR":
		return aspectImageThreeFour, nil
	}
	return 0, fmt.Errorf("不支持的图片宽高比: %s", aspect)
}

// videoResolution 把用户输入的分辨率映射成上游枚举；空表示不指定。
func videoResolution(resolution string) *int {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "360p":
		value := resolution360p
		return &value
	case "720p":
		value := resolution720p
		return &value
	case "1080p":
		value := resolution1080p
		return &value
	case "4k":
		value := resolution4K
		return &value
	}
	return nil
}
