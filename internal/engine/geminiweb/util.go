package geminiweb

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// randID 生成随机十六进制 ID（大写，用于请求 UUID 等）。
func randID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}

// newUUID 生成大写 UUID。网页端要求请求头和 f.req[59] 使用同一份，
// 模型头末尾还要带上客户端会话 ID。
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return strings.ToUpper(s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32])
}

func decodeRune(s string) (rune, int) {
	if s == "" {
		return utf8.RuneError, 0
	}
	return utf8.DecodeRuneInString(s)
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
