package model

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID 生成 n 字节的随机十六进制字符串（用于 chatcmpl 等 ID）。
func NewID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte("fallback"))
	}
	return hex.EncodeToString(b)
}

// randHex 兼容别名。
func randHex(n int) string { return NewID(n) }
