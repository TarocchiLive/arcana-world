package models

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const downloadStallTimeout = 60 * time.Second

func downloadClient(proxy string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	switch proxy {
	case "":
	case "direct":
		transport.Proxy = nil
	default:
		u, err := url.Parse(proxy)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return nil, ErrDownload
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return ErrDownload
		}
		return nil
	}}, nil
}

// openRegular 在开始写入前核对文件身份，避免截断链接指向的现有资源。
func openRegular(r *os.Root, name string) (*os.File, error) {
	if err := regular(r, name); err != nil {
		return nil, err
	}
	f, err := r.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	info, err := r.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, actual) {
		f.Close()
		return nil, ErrUnsafePath
	}
	return f, nil
}

func download(ctx context.Context, r *os.Root, proxy string, spec Spec, progress func(Progress)) error {
	client, err := downloadClient(proxy)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	part := spec.Name + spec.extension + ".part"
	file, err := openRegular(r, part)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > spec.ArchiveSize {
		if err = file.Truncate(0); err != nil {
			return err
		}
	}
	// 只有一次尝试没有任何进展才计入失败，慢速网络可以分多次续传完成。
	for failures := 0; ; {
		if err = ctx.Err(); err != nil {
			return err
		}
		info, err = file.Stat()
		if err != nil {
			return err
		}
		if info.Size() == spec.ArchiveSize {
			break
		}
		notify(progress, spec, "downloading", info.Size())
		retry, fetchErr := fetch(ctx, client, file, spec, progress)
		if fetchErr == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		grown, statErr := file.Stat()
		if statErr != nil {
			return statErr
		}
		if grown.Size() > info.Size() {
			failures = 0
		} else {
			failures++
		}
		if !retry || failures >= 3 {
			return fetchErr
		}
		timer := time.NewTimer(time.Duration(max(failures, 1)) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	notify(progress, spec, "verifying", spec.ArchiveSize)
	hash, err := digest(ctx, file)
	if err != nil {
		return err
	}
	if hash != spec.archiveSHA256 {
		file.Close()
		_ = r.Remove(part)
		return ErrIntegrity
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	archive := spec.Name + spec.extension
	if _, err = r.Lstat(archive); err == nil {
		return ErrBusy
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return r.Rename(part, archive)
}

func fetch(ctx context.Context, client *http.Client, file *os.File, spec Spec, progress func(Progress)) (bool, error) {
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	offset := info.Size()
	// 以长时间无数据代替整体超时，大文件在慢速网络下也能持续下载。
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stall := time.AfterFunc(downloadStallTimeout, cancel)
	defer stall.Stop()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, spec.ArchiveURL, nil)
	if err != nil {
		return false, ErrDownload
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if spec.Name == "melo-zh-en" {
		req.Header.Set("Referer", "https://live.bilibili.com/")
	}
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := client.Do(req)
	if err != nil {
		return true, ErrDownload
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		return true, ErrDownload
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return false, ErrDownload
	}
	switch response.StatusCode {
	case http.StatusOK:
		// 服务器忽略 Range 时从零开始，绝不能把完整响应追加到部分文件。
		offset = 0
		if err = file.Truncate(0); err != nil {
			return false, err
		}
	case http.StatusPartialContent:
		expected := fmt.Sprintf("bytes %d-%d/%d", offset, spec.ArchiveSize-1, spec.ArchiveSize)
		if response.Header.Get("Content-Range") != expected {
			return false, ErrDownload
		}
	default:
		return false, ErrDownload
	}
	if response.ContentLength >= 0 && response.ContentLength != spec.ArchiveSize-offset {
		return false, ErrDownload
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return false, err
	}
	notify(progress, spec, "downloading", offset)
	buffer := make([]byte, 64<<10)
	last := time.Now()
	for {
		if err = requestCtx.Err(); err != nil {
			return true, ErrDownload
		}
		n, readErr := response.Body.Read(buffer)
		if int64(n) > spec.ArchiveSize-offset {
			return false, ErrIntegrity
		}
		if n > 0 {
			written, writeErr := file.Write(buffer[:n])
			offset += int64(written)
			if writeErr != nil {
				return false, writeErr
			}
			if written != n {
				return false, io.ErrShortWrite
			}
			stall.Reset(downloadStallTimeout)
			if time.Since(last) >= 100*time.Millisecond {
				notify(progress, spec, "downloading", offset)
				last = time.Now()
			}
		}
		if readErr != nil {
			notify(progress, spec, "downloading", offset)
			if readErr == io.EOF && offset == spec.ArchiveSize {
				return false, nil
			}
			return true, ErrDownload
		}
	}
}
