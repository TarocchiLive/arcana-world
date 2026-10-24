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
	"image/jpeg"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
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

// UploadCover 发布确认界面中展示的已压缩 JPEG。
// 若返回的 URL 非空且伴随错误，说明上传成功，但发布尚未完成；
// 调用方不得将这种结果展示为直播间更新成功。
func (c *Client) UploadCover(ctx context.Context, prepared *coverimage.Prepared) (CoverUpdate, error) {
	var result CoverUpdate
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if prepared == nil || len(prepared.JPEG) == 0 || len(prepared.JPEG) > coverimage.MaxUploadSize {
		logger.Error("cover image size invalid", "bytes", coverBytes(prepared), "limit", coverimage.MaxUploadSize)
		return result, errors.New(i18n.T(i18n.BiliCoverImageInvalid))
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(prepared.JPEG))
	if err != nil || config.Width != coverimage.Width || config.Height != coverimage.Height {
		logger.Error("cover jpeg dimensions invalid", "bytes", len(prepared.JPEG), "width", config.Width, "height", config.Height, "err", err)
		return result, errors.New(i18n.T(i18n.BiliCoverDimensionsInvalid))
	}
	// 仅校验头部会放过截断或损坏的数据。在任何需要认证的网络操作前，
	// 先完整解码大小受限且尺寸固定的 JPEG。
	if _, err = jpeg.Decode(bytes.NewReader(prepared.JPEG)); err != nil {
		logger.Error("cover jpeg corrupted", "bytes", len(prepared.JPEG), "err", err)
		return result, errors.New(i18n.T(i18n.BiliCoverImageCorrupted))
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
	header.Set("Content-Disposition", `form-data; name="file"; filename="cover.jpg"`)
	header.Set("Content-Type", "image/jpeg")
	part, err := writer.CreatePart(header)
	if err != nil {
		return result, err
	}
	if _, err = part.Write(prepared.JPEG); err != nil {
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
		logger.Error("cover upload failed", "bytes", len(prepared.JPEG), "err", err)
		return result, err
	}
	if err = c.coverFailure(i18n.T(i18n.BiliCoverUpload), e); err != nil {
		return result, err
	}
	var uploaded struct {
		Location string `json:"location"`
	}
	if err = decode("/x/upload/web/image", e.Data, &uploaded); err != nil {
		return result, err
	}
	if uploaded.Location == "" {
		logger.Error("cover upload response has no location")
		return result, errors.New(i18n.T(i18n.BiliCoverUploadUrlMissing))
	}
	logger.Info("cover uploaded", "bytes", len(prepared.JPEG))
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
		logger.Error("cover publication failed", "uploaded", true, "err", err)
		return result, fmt.Errorf(i18n.T(i18n.BiliCoverPublicationFailed), err)
	}
	if len(e.Data) != 0 && string(e.Data) != "null" {
		var data struct {
			Audit *struct {
				Status json.RawMessage `json:"audit_title_status"`
				Reason string          `json:"audit_title_reason"`
			} `json:"audit_info"`
		}
		if err = decode(preLiveInfo, e.Data, &data); err != nil {
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
		logger.Warn("cover url rejected", "err", err)
		return nil, err
	}
	if c.HTTP == nil {
		return nil, errors.New(i18n.T(i18n.BiliHttpClientMissing))
	}
	start := time.Now()
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
			logger.Warn("cover download interrupted", "host", u.Hostname(), "path", u.Path, "err", ctx.Err())
			return nil, ctx.Err()
		}
		logger.Error("cover download failed", "host", u.Hostname(), "path", u.Path, "elapsed", time.Since(start), "err", netCause(err))
		return nil, errors.New(i18n.T(i18n.BiliCoverDownloadFailed))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		logger.Error("cover download failed", "host", u.Hostname(), "path", u.Path, "status", response.StatusCode)
		return nil, fmt.Errorf(i18n.T(i18n.BiliCoverHttpStatus), response.StatusCode)
	}
	if response.ContentLength > coverimage.MaxFileSize {
		logger.Error("cover download too large", "host", u.Hostname(), "path", u.Path, "content_length", response.ContentLength, "limit", coverimage.MaxFileSize)
		return nil, errors.New(i18n.T(i18n.BiliCoverFileTooLarge))
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, coverimage.MaxFileSize+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		logger.Error("cover download read failed", "host", u.Hostname(), "path", u.Path, "err", err)
		return nil, errors.New(i18n.T(i18n.BiliCoverReadFailed))
	}
	img, err := coverimage.Decode(raw)
	if err != nil {
		logger.Error("cover image decode failed", "host", u.Hostname(), "path", u.Path, "bytes", len(raw), "err", err)
		return nil, err
	}
	logger.Debug("cover fetched", "host", u.Hostname(), "path", u.Path, "bytes", len(raw), "elapsed", time.Since(start))
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
	logCheck(operation, response, err)
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

func coverBytes(prepared *coverimage.Prepared) int {
	if prepared == nil {
		return 0
	}
	return len(prepared.JPEG)
}
