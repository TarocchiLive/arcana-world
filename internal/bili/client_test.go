package bili

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"arcana-world/internal/domain"
)

func TestSignatureMatchesPythonURLSerialization(t *testing.T) {
	// 参考值由 Python hashlib.md5(urllib.parse.urlencode(sorted(...))+secret) 生成。
	p := signed(values("csrf", "中文 ~ a+b/", "ts", "1700000000"))
	if p.Get("sign") != "c6fd15ffa2548cfbbde055923539ae7d" {
		t.Fatal("signature incompatible with LiveHime Python serialization")
	}
}
func TestQRIsAnonymousAndAccountSwitchDropsOldCookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/passport-login/web/qrcode/generate":
			if r.Header.Get("Cookie") != "" {
				t.Error("QR acquisition sent stored account cookies")
			}
			fmt.Fprint(w, `{"code":0,"data":{"qrcode_key":"key","url":"https://passport.bilibili.com/login?key=test"}}`)
		case "/x/passport-login/web/qrcode/poll":
			if r.Header.Get("Cookie") != "" {
				t.Error("QR polling sent stored account cookies")
			}
			fmt.Fprint(w, `{"code":0,"data":{"code":86101}}`)
		case "/x/web-interface/nav":
			if _, err := r.Cookie("previous_only"); err == nil {
				t.Error("old account cookie reached the new account")
			}
			if _, err := r.Cookie("jar_only"); err == nil {
				t.Error("injected cookie jar contaminated account")
			}
			cookie, err := r.Cookie("SESSDATA")
			if err != nil || cookie.Value != "second" {
				t.Error("new account session was not used")
			}
			fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":2,"uname":"second"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP = server.Client()
	c.APIBase = server.URL
	c.PassportBase = server.URL
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(server.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "jar_only", Value: "secret"}})
	c.HTTP.Jar = jar
	first := domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "first", "previous_only": "private"}}
	c.SetAccount(first)
	ctx := context.Background()
	qr, err := c.GenerateQR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.PollQR(ctx, qr.Key); err != nil {
		t.Fatal(err)
	}
	second := domain.Account{UID: "2", Cookies: map[string]string{"SESSDATA": "second"}}
	c.SetAccount(second)
	second.Cookies["SESSDATA"] = "caller mutation"
	if _, err = c.Validate(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestRedirectCannotForwardAuthenticatedPOST(t *testing.T) {
	var received atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Store(true) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	c, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP = origin.Client()
	c.LiveBase = origin.URL
	c.SetAccount(domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "secret", "bili_jct": "csrf"}})
	if err = c.SetTitle(context.Background(), 1, "private title"); err == nil {
		t.Fatal("redirect accepted")
	}
	if received.Load() {
		t.Fatal("authenticated body was forwarded to redirect destination")
	}
}
