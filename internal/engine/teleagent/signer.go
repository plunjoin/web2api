// signer.go 实现 TeleAgent 聊天网关的两步 HMAC-SHA256 请求签名。
// 算法逆向自桌面端内核 GatewaySigner.Sign（见 docs/guide/teleagent-auth.md
// 与插件 docs/PROTOCOL.md）。
package teleagent

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hmacHex(key, msg string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// computeSignature 计算两步 HMAC 签名（纯函数，便于单测）。
//
// 第 1 步：key = JWT 第三段（token.split(".")[2]），
//
//	msg = "superagent-auth-v1/{token}/{installId}/{ts}/{nonce}"，k1 = hex(HMAC-SHA256(key, msg))
//
// 第 2 步：key = k1（当作 64 字符十六进制文本），
//
//	msg = "superagent-auth-v1\n{METHOD}\n{pathname}\n{ts}\n{nonce}\n{appVersion}\n{sha256hex(body)}"
//	sig = hex(HMAC-SHA256(key, msg))
func computeSignature(token, installID, appVersion, method, pathname, ts, nonce, body string) (k1, signature string) {
	// token 形如 `<tenant>:<JWT>`，切分后 [2] 仍是 JWT 的签名段。
	parts := strings.Split(token, ".")
	jwtThird := ""
	if len(parts) >= 3 {
		jwtThird = parts[2]
	}
	k1 = hmacHex(jwtThird, "superagent-auth-v1/"+token+"/"+installID+"/"+ts+"/"+nonce)
	signature = hmacHex(k1, strings.Join([]string{
		"superagent-auth-v1", method, pathname, ts, nonce, appVersion, sha256hex(body),
	}, "\n"))
	return k1, signature
}

// authHeaders 组装一次聊天请求的签名头。每次请求都要新生成 ts + nonce。
type authHeaders struct {
	ts      string
	nonce   string
	headers map[string]string
}

func newNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// 与插件 crypto.randomUUID() 风格一致的 UUID 形态。
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func newSessionID() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return "ses_" + hex.EncodeToString(b)
}

// newDeviceID 生成设备指纹（桌面端为 MAC 派生，浏览器登录无 MAC，用 12 位 hex 兜底）。
func newDeviceID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newInstallID 生成安装指纹（与桌面端 device-meta.json 的 installId 同为 UUID 形态）。
func newInstallID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func newMessageID() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

// buildAuthHeaders 组装签名头。body 是要发出去的完整请求体文本。
func buildAuthHeaders(c Credentials, method, pathname, body string) authHeaders {
	appVersion := c.AppVersion
	if appVersion == "" {
		appVersion = DefaultAppVersion
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := newNonce()
	_, sig := computeSignature(c.Token, c.InstallID, appVersion, method, pathname, ts, nonce, body)

	sid := newSessionID()
	h := map[string]string{
		"Content-Type":               "application/json",
		"Accept":                     "application/json",
		"X-Token":                    c.Token,
		"User-Agent":                 "super-agent/1.0",
		"X-App-Version":              appVersion,
		"X-Route-Target":             RouteTarget,
		"X-Superagent-Device-Id":     c.DeviceID,
		"X-Superagent-Install-Id":    c.InstallID,
		"X-Superagent-Timestamp":     ts,
		"X-Superagent-Nonce":         nonce,
		"X-Superagent-Sign-Version":  "v1",
		"X-Superagent-Signature":     sig,
		"X-Teleai-Run-Unit-Type":     "agent_loop",
		"X-Session-Id":               sid,
		"X-Message-Id":               newMessageID(),
		"X-Teleai-Root-Session-Id":   sid,
		"Cache-Control":              "no-cache",
	}
	if c.APIKey != "" {
		h["Authorization"] = "Bearer " + c.APIKey
	} else {
		h["Authorization"] = "Bearer " + DefaultAPIKey
	}
	return authHeaders{ts: ts, nonce: nonce, headers: h}
}

// parseUpstreamError 把上游 OpenAI 兼容错误体解析成可读信息，保留厂商原话。
func parseUpstreamError(data []byte) string {
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		return "TeleAgent 上游返回了错误"
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}
