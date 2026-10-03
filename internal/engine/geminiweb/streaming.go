package geminiweb

import (
	"strings"
)

// StreamingFrameParser 增量解析 Google 的「长度前缀」流式帧。
// 帧格式：<长度数字>\n<JSON>\n，长度按 UTF-16 code units 计（与 JS 字符串语义一致）。
// 参考 gemini_webapi.utils.parsing.StreamingFrameParser。
type StreamingFrameParser struct {
	buffer        string
	expectedUnits int
	payloadStart  int
	scannedChars  int
	scannedUnits  int
	prefixChecked bool
}

// NewStreamingFrameParser 创建解析器。
func NewStreamingFrameParser() *StreamingFrameParser {
	return &StreamingFrameParser{expectedUnits: -1}
}

// Feed 输入新文本，返回本轮完成的完整 JSON 帧。
func (p *StreamingFrameParser) Feed(content string) []any {
	if content != "" {
		p.buffer += content
	}
	p.stripPrefixOnce()

	var frames []any
	for {
		if p.expectedUnits < 0 && !p.readLengthMarker() {
			break
		}
		p.scanAvailablePayload()
		if p.scannedUnits < p.expectedUnits {
			break
		}
		endPos := p.payloadStart + p.scannedChars
		chunk := p.buffer[p.payloadStart:endPos]
		p.buffer = p.buffer[endPos:]
		p.resetFrameState()

		if strings.TrimSpace(chunk) == "" {
			continue
		}
		var parsed any
		if err := jsonUnmarshal([]byte(chunk), &parsed); err != nil {
			continue
		}
		switch t := parsed.(type) {
		case []any:
			frames = append(frames, t...)
		default:
			frames = append(frames, parsed)
		}
	}
	return frames
}

// Flush 解码结束后处理遗留数据。
func (p *StreamingFrameParser) Flush() []any {
	return p.Feed("")
}

// parseResponseFrames 一次性解析完整响应文本（带 )]}' 前缀与长度帧格式）。
// 帧解析失败时回退为整体 JSON / NDJSON 解析。
func parseResponseFrames(text string) []any {
	content := strings.TrimPrefix(text, ")]}'")
	content = strings.TrimLeft(content, " \t\n\r")

	parser := NewStreamingFrameParser()
	frames := parser.Feed(content)
	frames = append(frames, parser.Flush()...)
	if len(frames) > 0 {
		return frames
	}

	// 回退 1：整体 JSON
	var parsed any
	if err := jsonUnmarshal([]byte(strings.TrimSpace(content)), &parsed); err == nil {
		if list, ok := parsed.([]any); ok {
			return list
		}
		return []any{parsed}
	}
	// 回退 2：NDJSON
	var collected []any
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item any
		if err := jsonUnmarshal([]byte(line), &item); err != nil {
			continue
		}
		if list, ok := item.([]any); ok {
			collected = append(collected, list...)
		} else {
			collected = append(collected, item)
		}
	}
	return collected
}

// parseResponseFramesFromBody 供外部使用的便捷入口。
func parseResponseFramesFromBody(body []byte) []any {
	return parseResponseFrames(string(body))
}

func (p *StreamingFrameParser) resetFrameState() {
	p.expectedUnits = -1
	p.payloadStart = 0
	p.scannedChars = 0
	p.scannedUnits = 0
}

func (p *StreamingFrameParser) stripPrefixOnce() {
	if p.prefixChecked {
		return
	}
	prefix := ")]}'"
	if len(p.buffer) < len(prefix) && strings.HasPrefix(prefix, p.buffer) {
		return
	}
	if strings.HasPrefix(p.buffer, prefix) {
		p.buffer = strings.TrimLeft(p.buffer[len(prefix):], " \t\n\r")
	}
	p.prefixChecked = true
}

func (p *StreamingFrameParser) readLengthMarker() bool {
	consumed := 0
	total := len(p.buffer)
	for consumed < total && (p.buffer[consumed] == ' ' || p.buffer[consumed] == '\t' || p.buffer[consumed] == '\n' || p.buffer[consumed] == '\r') {
		consumed++
	}
	if consumed > 0 {
		p.buffer = p.buffer[consumed:]
		total = len(p.buffer)
	}
	if total == 0 {
		return false
	}
	// 长度标记：一行数字
	idx := 0
	for idx < total && p.buffer[idx] >= '0' && p.buffer[idx] <= '9' {
		idx++
	}
	if idx == 0 {
		return false
	}
	n := 0
	for i := 0; i < idx; i++ {
		n = n*10 + int(p.buffer[i]-'0')
	}
	p.expectedUnits = n
	p.payloadStart = idx
	p.scannedChars = 0
	p.scannedUnits = 0
	return true
}

func (p *StreamingFrameParser) scanAvailablePayload() {
	if p.expectedUnits < 0 {
		return
	}
	idx := p.payloadStart + p.scannedChars
	limit := len(p.buffer)
	for p.scannedUnits < p.expectedUnits && idx < limit {
		r, size := decodeRune(p.buffer[idx:])
		unit := 1
		if r > 0xFFFF {
			unit = 2
		}
		if p.scannedUnits+unit > p.expectedUnits {
			break
		}
		p.scannedUnits += unit
		p.scannedChars += size
		idx += size
	}
}
