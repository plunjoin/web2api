package geminiweb

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

func decodeRune(s string) (rune, int) {
	if s == "" {
		return utf8.RuneError, 0
	}
	return utf8.DecodeRuneInString(s)
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
