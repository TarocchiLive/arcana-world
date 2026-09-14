// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"arcana-world/internal/coverimage"
	"arcana-world/internal/domain"
)

func preparedCover(t *testing.T) *coverimage.Prepared {
	t.Helper()
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(path, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := coverimage.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCoverUploadWireAndPublication(t *testing.T) {
	p := preparedCover(t)
	for _, failPublish := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != http.MethodPost {
				t.Error("expected POST")
			}
			if calls == 1 {
				if r.URL.Path != "/x/upload/web/image" || r.URL.Query().Get("csrf") != "csrf-secret" {
					t.Error("wrong upload endpoint/query")
				}
				if err := r.ParseMultipartForm(coverimage.MaxFileSize); err != nil {
					t.Fatal(err)
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("bucket") != "live" || r.FormValue("dir") != "new_room_cover" {
					t.Error("wrong bucket/directory")
				}
				file, header, err := r.FormFile("file")
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				raw, err := io.ReadAll(file)
				if err != nil {
					t.Fatal(err)
				}
				if header.Filename != "blob" || header.Header.Get("Content-Type") != "image/png" || !bytes.Equal(raw, p.PNG) {
					t.Error("uploaded bytes differ from confirmed preview")
				}
				img, err := png.Decode(bytes.NewReader(raw))
				if err != nil || img.Bounds() != image.Rect(0, 0, 704, 396) {
					t.Error("upload not normalized to 704x396 PNG")
				}
				io.WriteString(w, `{"code":0,"data":{"location":"https://i0.hdslb.com/cover.png"}}`)
				return
			}
			if r.URL.Path != preLiveInfo {
				t.Error("wrong publication endpoint")
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{"csrf": "csrf-secret", "csrf_token": "csrf-secret", "platform": "pc_link", "mobi_app": "pc_link", "build": "1", "cover": "https://i0.hdslb.com/cover.png", "coverVertical": "", "liveDirectionType": "1", "visit_id": ""} {
				if !r.PostForm.Has(key) || r.PostForm.Get(key) != want {
					t.Errorf("wrong publication field %s", key)
				}
			}
			if failPublish {
				io.WriteString(w, `{"code":-617}`)
			} else {
				io.WriteString(w, `{"code":0,"data":{"audit_info":{"audit_title_status":1,"audit_title_reason":"pending"}}}`)
			}
		}))
		client, err := New("direct")
		if err != nil {
			t.Fatal(err)
		}
		client.APIBase, client.LiveBase = server.URL, server.URL
		client.SetAccount(domain.Account{Cookies: map[string]string{"SESSDATA": "session-secret", "bili_jct": "csrf-secret"}})
		result, err := client.UploadCover(context.Background(), p)
		server.Close()
		if calls != 2 || result.URL != "https://i0.hdslb.com/cover.png" {
			t.Fatalf("publication result: %+v, calls=%d", result, calls)
		}
		if failPublish {
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Code != -617 {
				t.Fatalf("lost publication failure: %v", err)
			}
		} else if err != nil {
			t.Fatalf("successful publication failed: %v", err)
		}
	}
}

type coverTransport func(*http.Request) (*http.Response, error)

func (f coverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCoverRejectsInvalidBeforeNetwork(t *testing.T) {
	client := &Client{HTTP: &http.Client{Transport: coverTransport(func(*http.Request) (*http.Response, error) { t.Fatal("invalid input reached network"); return nil, nil })}}
	client.APIBase, client.LiveBase = "https://api.bilibili.com", "https://api.live.bilibili.com"
	client.SetAccount(domain.Account{Cookies: map[string]string{"SESSDATA": "session-secret", "bili_jct": "csrf-secret"}})
	var wrongSize bytes.Buffer
	if err := png.Encode(&wrongSize, image.NewNRGBA(image.Rect(0, 0, 160, 90))); err != nil {
		t.Fatal(err)
	}
	p := preparedCover(t)
	for _, bad := range []*coverimage.Prepared{nil, {PNG: make([]byte, coverimage.MaxFileSize+1)}, {PNG: wrongSize.Bytes()}, {PNG: p.PNG[:len(p.PNG)/2]}} {
		if _, err := client.UploadCover(context.Background(), bad); err == nil {
			t.Fatal("accepted invalid PNG")
		}
	}
	for _, raw := range []string{"https://evil.test/a", "https://i0.hdslb.com.evil.test/a", "https://user:secret@i0.hdslb.com/a", "https://i0.hdslb.com:8443/a", "https://127.0.0.1/a", "file:///tmp/a", "https://i0.hdslb.com:/a"} {
		if _, err := client.FetchCover(context.Background(), raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestFetchCoverAnonymousAndRedirects(t *testing.T) {
	p := preparedCover(t)
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://i0.hdslb.com/")
	jar.SetCookies(u, []*http.Cookie{{Name: "SESSDATA", Value: "jar-secret"}})
	for _, target := range []string{"https://i1.biliimg.com/image", "https://evil.test/image", "http://i1.biliimg.com/image"} {
		calls := 0
		client := &Client{HTTP: &http.Client{Jar: jar, Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("buvid") != "" || r.URL.Scheme != "https" {
				t.Error("download leaked credentials or used HTTP")
			}
			if calls == 1 {
				return &http.Response{StatusCode: 302, Header: http.Header{"Location": {target}}, Body: io.NopCloser(bytes.NewReader(nil)), Request: r}, nil
			}
			if r.URL.Host != "i1.biliimg.com" {
				t.Fatal("untrusted redirect followed")
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(p.PNG)), Request: r}, nil
		})}}
		client.SetAccount(domain.Account{Cookies: map[string]string{"SESSDATA": "account-secret"}})
		_, err := client.FetchCover(context.Background(), "http://i0.hdslb.com/image")
		if target == "https://i1.biliimg.com/image" {
			if err != nil || calls != 2 {
				t.Fatalf("safe download failed: %v", err)
			}
		} else if err == nil || calls != 1 {
			t.Fatal("unsafe redirect accepted")
		}
	}
}

func TestCoverRejectionKeepsReasonWithoutCredentialEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"code":-617,"message":"服务端拒绝测试 session-secret \u001b[31m"}`)
	}))
	defer server.Close()
	client, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	client.APIBase = server.URL
	client.SetAccount(domain.Account{Cookies: map[string]string{"SESSDATA": "session-secret", "bili_jct": "csrf-secret"}})
	_, err = client.UploadCover(context.Background(), preparedCover(t))
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Code != -617 {
		t.Fatalf("lost API rejection: %v", err)
	}
	if !strings.Contains(err.Error(), "服务端拒绝测试") || strings.Contains(err.Error(), "session-secret") || strings.ContainsRune(err.Error(), '\x1b') {
		t.Fatalf("unsafe or missing diagnostic: %v", err)
	}
}
