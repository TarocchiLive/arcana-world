package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"arcana-world/internal/store"

	"golang.org/x/mod/semver"
)

const latestReleaseURL = "https://api.github.com/repos/TarocchiLive/arcana-world/releases/latest"
const releasePageURL = "https://github.com/TarocchiLive/arcana-world/releases"

func runUpdateCheck(ctx context.Context, out io.Writer, dir string, options store.Options) error {
	current := "v" + strings.TrimPrefix(version, "v")
	if !semver.IsValid(current) {
		return fmt.Errorf(i18n.T(i18n.CLIUpdateVersionInvalid), version)
	}
	// 只读配置获取代理，不打开存储、凭据后端或设备标识数据库。
	cfg, err := store.ReadConfig(dir)
	if err != nil {
		return errors.New(i18n.T(i18n.CLIUpdateConfigFailed))
	}
	proxy := cfg.Proxy
	if options.Overrides.Proxy != nil {
		proxy = *options.Overrides.Proxy
	}
	client, err := bili.New(proxy, "")
	if err != nil {
		return err
	}
	defer client.HTTP.CloseIdleConnections()
	// 固定查询官方仓库；不跟随重定向到其他主机，也不携带账号凭据。
	client.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "arcana-world/"+current)
	response, err := client.HTTP.Do(req)
	if err != nil {
		return updateNetworkError(ctx)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusTooManyRequests:
		return errors.New(i18n.T(i18n.CLIUpdateAccessLimited))
	case http.StatusNotFound:
		return errors.New(i18n.T(i18n.CLIUpdateNotFound))
	default:
		return fmt.Errorf(i18n.T(i18n.CLIUpdateHTTPFailed), response.StatusCode)
	}
	const maxReleaseSize = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseSize+1))
	if err != nil {
		return updateNetworkError(ctx)
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if len(body) > maxReleaseSize || json.Unmarshal(body, &release) != nil {
		return errors.New(i18n.T(i18n.CLIUpdateResponseInvalid))
	}
	latest := "v" + strings.TrimPrefix(release.Tag, "v")
	if !semver.IsValid(latest) || release.Draft || release.Prerelease || semver.Prerelease(latest) != "" {
		return errors.New(i18n.T(i18n.CLIUpdateResponseInvalid))
	}
	var message string
	switch semver.Compare(current, latest) {
	case -1:
		message = fmt.Sprintf(i18n.T(i18n.CLIUpdateAvailable), current, latest, releasePageURL+"/tag/"+url.PathEscape(release.Tag))
	case 0:
		message = fmt.Sprintf(i18n.T(i18n.CLIUpdateCurrent), current, latest)
	case 1:
		message = fmt.Sprintf(i18n.T(i18n.CLIUpdateAhead), current, latest)
	}
	if semver.Prerelease(current) != "" {
		message = i18n.T(i18n.CLIUpdatePrerelease) + "\n" + message
	}
	_, err = fmt.Fprintln(out, message)
	return err
}

func updateNetworkError(ctx context.Context) error {
	// 网络错误可能包含带密码的代理地址；仅输出可操作的分类提示。
	key := i18n.CLIUpdateNetworkFailed
	switch ctx.Err() {
	case context.Canceled:
		key = i18n.CLIUpdateCanceled
	case context.DeadlineExceeded:
		key = i18n.CLIUpdateTimeout
	}
	return errors.New(i18n.T(key))
}
