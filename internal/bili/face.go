// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"arcana-world/internal/i18n"

	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"arcana-world/internal/domain"
)

// ResolveFace 返回 B 站的真实验证页面，不会将用户标记为已验证。
// 用户完成页面验证后，必须明确重试开播操作。
// V1 已携带二维码 URL；V2 会注册并解码风控质询，
// 随后严格按照 StartLive 的方式构建经过两次 URL 转义的令牌链接。
func (c *Client) ResolveFace(ctx context.Context, challenge *domain.FaceChallenge) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if challenge == nil {
		return "", errors.New(i18n.T(i18n.BiliIdentityChallengeMissing))
	}
	if challenge.URL != "" {
		if !validLink(challenge.URL) {
			return "", errors.New(i18n.T(i18n.BiliIdentityLinkUntrusted))
		}
		return challenge.URL, nil
	}
	if challenge.Voucher == "" {
		return "", errors.New(i18n.T(i18n.BiliIdentityCredentialMissing))
	}
	p, err := c.csrf(values("v_voucher", challenge.Voucher, "dm_track", "[]"))
	if err != nil {
		return "", err
	}
	p.Del("csrf_token")
	var d struct {
		Content string `json:"content"`
	}
	if err = c.call(ctx, http.MethodPost, c.APIBase, "/x/gaia-vgate/v2/register", nil, p, false, true, &d); err != nil {
		return "", err
	}
	token, err := decodeRisk(challenge.Voucher, d.Content)
	if err != nil {
		return "", err
	}
	// captcha_codec.__risk_captcha_enc__ 还会计算 AES 内容字段，
	// 但 FaceCaptchaWorker 会丢弃它。实际发送到实名页面的只有新生成的
	// 混淆令牌，因此这里省略了无用密文的计算。
	pollToken, err := obfuscateToken(token)
	if err != nil {
		return "", err
	}
	linkToken, err := obfuscateToken(token)
	if err != nil {
		return "", err
	}
	challenge.Voucher = pollToken
	challenge.URL = "https://www.bilibili.com/h5/risk-control/realname?t=" + quote(quote(linkToken))
	return challenge.URL, nil
}
func decodeRisk(voucher, content string) (string, error) {
	invalid := errors.New(i18n.T(i18n.BiliIdentityResponseDecodeFailed))
	raw, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return "", invalid
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[1] == "" {
		return "", invalid
	}
	index, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return "", invalid
	}
	start, end := -1, -1
	for bit := range 32 {
		if index&(1<<bit) != 0 {
			if start < 0 {
				start = bit
			} else {
				end = bit
				break
			}
		}
	}
	if start < 0 || end < 0 {
		return "", invalid
	}
	if len(voucher) > 8 {
		voucher = voucher[8:]
	}
	if start > len(voucher) {
		start = len(voucher)
	}
	if end > len(voucher) {
		end = len(voucher)
	}
	key := sha256.Sum256([]byte(voucher[start:end] + parts[0]))
	encrypted, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", invalid
	}
	for i := range encrypted {
		encrypted[i] ^= key[i%16]
	}
	var decoded struct {
		Token string `json:"token"`
		Type  string `json:"type"`
	}
	if json.Unmarshal(encrypted, &decoded) != nil || decoded.Token == "" {
		return "", invalid
	}
	if decoded.Type != "realname" {
		return "", errors.New(i18n.T(i18n.BiliIdentityTypeUnsupported))
	}
	return decoded.Token, nil
}
func obfuscateToken(token string) (string, error) {
	var random [9]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", errors.New(i18n.T(i18n.BiliIdentityRandomnessFailed))
	}
	raw := make([]byte, len(token)+9)
	raw[0] = random[0]
	for i := range len(token) {
		raw[i+1] = token[i] ^ random[0] ^ random[1+i%8]
	}
	copy(raw[len(token)+1:], random[1:])
	return base64.StdEncoding.EncodeToString(raw), nil
}
