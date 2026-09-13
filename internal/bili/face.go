// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
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

// ResolveFace returns Bilibili's real verification page. It never marks a user
// verified: after completing the page the user must explicitly retry Start.
// V1 already carries its QR URL. V2 registers and decodes the risk challenge,
// then constructs exactly the doubly-quoted token link used by StartLive.
func (c *Client) ResolveFace(ctx context.Context, challenge *domain.FaceChallenge) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if challenge == nil {
		return "", errors.New("缺少身份验证请求")
	}
	if challenge.URL != "" {
		if !validLink(challenge.URL) {
			return "", errors.New("身份验证链接不是受信任的 B 站 HTTPS 地址")
		}
		return challenge.URL, nil
	}
	if challenge.Voucher == "" {
		return "", errors.New("缺少身份验证凭证")
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
	// captcha_codec.__risk_captcha_enc__ also computes an AES content field;
	// FaceCaptchaWorker discards that field. Only its fresh obfuscated token is
	// sent to the realname page, so computing the unused ciphertext is omitted.
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
	invalid := errors.New("身份验证响应无法解码，请重新开播获取新的验证请求")
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
		return "", errors.New("B 站要求暂不支持的验证类型，请使用官方直播姬完成验证")
	}
	return decoded.Token, nil
}
func obfuscateToken(token string) (string, error) {
	var random [9]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", errors.New("生成验证令牌随机数失败")
	}
	raw := make([]byte, len(token)+9)
	raw[0] = random[0]
	for i := range len(token) {
		raw[i+1] = token[i] ^ random[0] ^ random[1+i%8]
	}
	copy(raw[len(token)+1:], random[1:])
	return base64.StdEncoding.EncodeToString(raw), nil
}
