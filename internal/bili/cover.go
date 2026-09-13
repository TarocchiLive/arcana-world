// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
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
	"time"
	"unicode"

	"arcana-world/internal/coverimage"
)

type CoverUpdate struct {
	URL    string
	Status string
	Reason string
}

// UploadCover publishes exactly the prepared PNG shown in the confirmation UI.
// A nonempty result URL alongside an error means upload succeeded but publication
// did not complete; callers must not present that as a successful room update.
func (c *Client) UploadCover(ctx context.Context, prepared *coverimage.Prepared) (CoverUpdate, error) {
	var result CoverUpdate
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if prepared == nil || len(prepared.PNG) == 0 || len(prepared.PNG) > coverimage.MaxFileSize {
		return result, errors.New("封面 PNG 无效或超过 20 MiB")
	}
	config, err := png.DecodeConfig(bytes.NewReader(prepared.PNG))
	if err != nil || config.Width != coverimage.Width || config.Height != coverimage.Height {
		return result, errors.New("封面必须为 704×396 PNG，请重新准备图片")
	}
	// Header-only validation accepts truncated/corrupted bodies. Decode the
	// bounded, fixed-size PNG before any authenticated network operation.
	if _, err = png.Decode(bytes.NewReader(prepared.PNG)); err != nil {
		return result, errors.New("封面 PNG 数据损坏")
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
	if err = c.coverFailure("封面上传", e); err != nil {
		return result, err
	}
	var uploaded struct {
		Location string `json:"location"`
	}
	if err = decode(e.Data, &uploaded); err != nil {
		return result, err
	}
	if uploaded.Location == "" {
		return result, errors.New("封面上传响应缺少图片地址")
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
		err = c.coverFailure("封面发布", e)
	}
	if err != nil {
		return result, fmt.Errorf("图片已上传，但房间封面发布失败：%w", err)
	}
	if len(e.Data) != 0 && string(e.Data) != "null" {
		var data struct {
			Audit *struct {
				Status json.RawMessage `json:"audit_title_status"`
				Reason string          `json:"audit_title_reason"`
			} `json:"audit_info"`
		}
		if err = decode(e.Data, &data); err != nil {
			return result, fmt.Errorf("封面发布已响应成功，但审核信息无法解析：%w", err)
		}
		if data.Audit != nil {
			result.Reason = data.Audit.Reason
			status := data.Audit.Status
			if len(status) != 0 && string(status) != "null" {
				if json.Unmarshal(status, &result.Status) != nil {
					var number json.Number
					if json.Unmarshal(status, &number) != nil {
						return result, errors.New("封面发布已响应成功，但审核状态格式无效")
					}
					result.Status = number.String()
				}
			}
		}
	}
	switch result.Status {
	case "0":
		result.Status = "审核中"
	case "1":
		result.Status = "审核通过"
	case "-1":
		result.Status = "审核未通过"
	case "":
		result.Status = "审核状态待刷新"
	}
	return result, nil
}

func coverURL(raw string, upgrade bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" || u.Fragment != "" {
		return nil, errors.New("封面地址无效")
	}
	if upgrade && u.Scheme == "http" {
		u.Scheme = "https"
	}
	host := strings.ToLower(u.Hostname())
	trusted := host == "hdslb.com" || strings.HasSuffix(host, ".hdslb.com") || host == "biliimg.com" || strings.HasSuffix(host, ".biliimg.com")
	if u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") || strings.HasSuffix(u.Host, ":") || net.ParseIP(host) != nil || !trusted {
		return nil, errors.New("封面仅支持官方 B 站图片 CDN 的 HTTPS 地址")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || label[0] == '-' || label[len(label)-1] == '-' {
			return nil, errors.New("封面域名无效")
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return nil, errors.New("封面域名无效")
			}
		}
	}
	return u, nil
}

// FetchCover never attaches account cookies, a cookie jar, or API authentication.
func (c *Client) FetchCover(ctx context.Context, rawURL string) (image.Image, error) {
	u, err := coverURL(rawURL, true)
	if err != nil {
		return nil, err
	}
	if c.HTTP == nil {
		return nil, errors.New("HTTP 客户端未配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("无法创建封面下载请求")
	}
	client := *c.HTTP
	client.Jar = nil
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("封面重定向次数过多")
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
		return nil, errors.New("封面下载失败（网络错误或不安全的重定向）")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("封面下载 HTTP 状态 %d", response.StatusCode)
	}
	if response.ContentLength > coverimage.MaxFileSize {
		return nil, errors.New("封面文件超过 20 MiB")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, coverimage.MaxFileSize+1))
	if err != nil {
		return nil, errors.New("读取封面失败")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
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

// Preserve the server's actual rejection reason without exposing credential echoes.
func (c *Client) coverFailure(operation string, response envelope) error {
	err := response.check(operation)
	if err == nil {
		return nil
	}
	message := response.Message
	for _, value := range c.account.Cookies {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[已隐藏]")
		}
	}
	var safe strings.Builder
	count := 0
	for _, r := range message {
		if unicode.IsControl(r) {
			continue
		}
		if count == 200 {
			safe.WriteString("…")
			break
		}
		safe.WriteRune(r)
		count++
	}
	if safe.Len() == 0 {
		return err
	}
	return fmt.Errorf("%w：%s", err, safe.String())
}
