// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"arcana-world/internal/i18n"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"unicode"

	"arcana-world/internal/coverimage"
)

const maxCoverRedirects = 5
const maxCoverRejectionRunes = 200

type CoverUpdate struct {
	URL    string
	Status string
	Reason string
}

// UploadCover 发布确认界面中展示的原始已准备 PNG。
// 若返回的 URL 非空且伴随错误，说明上传成功，但发布尚未完成；
// 调用方不得将这种结果展示为直播间更新成功。
func (c *Client) UploadCover(ctx context.Context, prepared *coverimage.Prepared) (CoverUpdate, error) {
	var result CoverUpdate
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if prepared == nil || len(prepared.PNG) == 0 || len(prepared.PNG) > coverimage.MaxFileSize {
		return result, errors.New(i18n.T(i18n.BiliCoverPngInvalid))
	}
	config, err := png.DecodeConfig(bytes.NewReader(prepared.PNG))
	if err != nil || config.Width != coverimage.Width || config.Height != coverimage.Height {
		return result, errors.New(i18n.T(i18n.BiliCoverDimensionsInvalid))
	}
	// 仅校验头部会放过截断或损坏的数据。在任何需要认证的网络操作前，
	// 先完整解码大小受限且尺寸固定的 PNG。
	if _, err = png.Decode(bytes.NewReader(prepared.PNG)); err != nil {
		return result, errors.New(i18n.T(i18n.BiliCoverPngCorrupted))
	}
	p, err := c.csrf(values())
	if err != nil {
		return result, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err = writer.WriteField("bucket", "live"); err != nil {
		return result, err
	}
	if err = writer.WriteField("dir", "new_room_cover"); err != nil {
		return result, err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="blob"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		return result, err
	}
	if _, err = part.Write(prepared.PNG); err != nil {
		return result, err
	}
	if err = writer.Close(); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	e, _, err := c.request(ctx, http.MethodPost, c.APIBase, "/x/upload/web/image", values("csrf", p.Get("csrf")), &body, writer.FormDataContentType(), false, true)
	if err != nil {
		return result, err
	}
	if err = c.coverFailure(i18n.T(i18n.BiliCoverUpload), e); err != nil {
		return result, err
	}
	var uploaded struct {
		Location string `json:"location"`
	}
	if err = decode(e.Data, &uploaded); err != nil {
		return result, err
	}
	if uploaded.Location == "" {
		return result, errors.New(i18n.T(i18n.BiliCoverUploadUrlMissing))
	}
	result.URL = uploaded.Location
	p.Set("platform", "pc_link")
	p.Set("mobi_app", "pc_link")
	p.Set("build", "1")
	p.Set("cover", result.URL)
	p.Set("coverVertical", "")
	p.Set("liveDirectionType", "1")
	p.Set("visit_id", "")
	e, _, err = c.request(ctx, http.MethodPost, c.LiveBase, preLiveInfo, nil, strings.NewReader(encode(p)), "application/x-www-form-urlencoded", false, true)
	if err == nil {
		err = c.coverFailure(i18n.T(i18n.BiliCoverPublication), e)
	}
	if err != nil {
		return result, fmt.Errorf(i18n.T(i18n.BiliCoverPublicationFailed), err)
	}
	if len(e.Data) != 0 && string(e.Data) != "null" {
		var data struct {
			Audit *struct {
				Status json.RawMessage `json:"audit_title_status"`
				Reason string          `json:"audit_title_reason"`
			} `json:"audit_info"`
		}
		if err = decode(e.Data, &data); err != nil {
			return result, fmt.Errorf(i18n.T(i18n.BiliCoverReviewDecodeFailed), err)
		}
		if data.Audit != nil {
			result.Reason = data.Audit.Reason
			status := data.Audit.Status
			if len(status) != 0 && string(status) != "null" {
				if json.Unmarshal(status, &result.Status) != nil {
					var number json.Number
					if json.Unmarshal(status, &number) != nil {
						return result, errors.New(i18n.T(i18n.BiliCoverReviewStatusInvalid))
					}
					result.Status = number.String()
				}
			}
		}
	}
	switch result.Status {
	case "0":
		result.Status = i18n.T(i18n.BiliCoverUnderReview)
	case "1":
		result.Status = i18n.T(i18n.BiliCoverApproved)
	case "-1":
		result.Status = i18n.T(i18n.BiliCoverRejected)
	case "":
		result.Status = i18n.T(i18n.BiliCoverReviewRefresh)
	}
	return result, nil
}

func coverURL(raw string, upgrade bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" || u.Fragment != "" {
		return nil, errors.New(i18n.T(i18n.BiliCoverUrlInvalid))
	}
	if upgrade && u.Scheme == "http" {
		u.Scheme = "https"
	}
	host := strings.ToLower(u.Hostname())
	trusted := host == "hdslb.com" || strings.HasSuffix(host, ".hdslb.com") || host == "biliimg.com" || strings.HasSuffix(host, ".biliimg.com")
	if u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") || strings.HasSuffix(u.Host, ":") || net.ParseIP(host) != nil || !trusted {
		return nil, errors.New(i18n.T(i18n.BiliCoverCdnRequired))
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || label[0] == '-' || label[len(label)-1] == '-' {
			return nil, errors.New(i18n.T(i18n.BiliCoverDomainInvalid))
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return nil, errors.New(i18n.T(i18n.BiliCoverDomainInvalid))
			}
		}
	}
	return u, nil
}

// FetchCover 不附加账号 Cookie、Cookie 容器或 API 身份认证信息。
func (c *Client) FetchCover(ctx context.Context, rawURL string) (image.Image, error) {
	u, err := coverURL(rawURL, true)
	if err != nil {
		return nil, err
	}
	if c.HTTP == nil {
		return nil, errors.New(i18n.T(i18n.BiliHttpClientMissing))
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New(i18n.T(i18n.BiliCoverRequestFailed))
	}
	client := *c.HTTP
	client.Jar = nil
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= maxCoverRedirects {
			return errors.New(i18n.T(i18n.BiliCoverRedirectLimit))
		}
		if _, err := coverURL(next.URL.String(), false); err != nil {
			return err
		}
		next.Header = make(http.Header)
		return nil
	}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New(i18n.T(i18n.BiliCoverDownloadFailed))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf(i18n.T(i18n.BiliCoverHttpStatus), response.StatusCode)
	}
	if response.ContentLength > coverimage.MaxFileSize {
		return nil, errors.New(i18n.T(i18n.BiliCoverFileTooLarge))
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, coverimage.MaxFileSize+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, errors.New(i18n.T(i18n.BiliCoverReadFailed))
	}
	img, err := coverimage.Decode(raw)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return img, nil
}

// 保留服务器实际拒绝原因，同时避免泄露响应中回显的凭据。
func (c *Client) coverFailure(operation string, response envelope) error {
	err := response.check(operation)
	if err == nil {
		return nil
	}
	message := response.Message
	for _, value := range c.account.Cookies {
		if value != "" {
			message = strings.ReplaceAll(message, value, i18n.T(i18n.BiliCredentialRedacted))
		}
	}
	var safe strings.Builder
	count := 0
	for _, r := range message {
		if unicode.IsControl(r) {
			continue
		}
		if count == maxCoverRejectionRunes {
			safe.WriteString("…")
			break
		}
		safe.WriteRune(r)
		count++
	}
	if safe.Len() == 0 {
		return err
	}
	return fmt.Errorf(i18n.T(i18n.BiliCoverRejectionDetail), err, safe.String())
}
