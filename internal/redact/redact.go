// Package redact 在文本返回 API 客户端前屏蔽敏感片段。
// 日志仍应保留原文，便于排查问题。
package redact

import "regexp"

const (
	emailPlaceholder  = "[REDACTED_EMAIL]"
	bearerPlaceholder = "[REDACTED_BEARER_TOKEN]"
	apiKeyPlaceholder = "[REDACTED_API_KEY]"
)

var (
	emailPattern  = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	bearerPattern = regexp.MustCompile(`(?i)\b(Bearer[ \t]+)[A-Za-z0-9._~+/=-]+`)
	apiKeyPattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])(sk[-_][A-Za-z0-9_-]{3,})`)
)

// Text 用稳定占位符替换文本中的邮箱、Bearer 令牌和 sk 风格 API Key。
func Text(text string) string {
	if text == "" {
		return text
	}
	text = emailPattern.ReplaceAllString(text, emailPlaceholder)
	text = bearerPattern.ReplaceAllString(text, `${1}`+bearerPlaceholder)
	return apiKeyPattern.ReplaceAllString(text, `${1}`+apiKeyPlaceholder)
}

// Error 返回 err 脱敏后的消息；err 为 nil 时返回空字符串。
func Error(err error) string {
	if err == nil {
		return ""
	}
	return Text(err.Error())
}
