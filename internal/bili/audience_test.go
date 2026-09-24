package bili

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"arcana-world/internal/domain"
)

func TestAudienceDoesNotSubstituteOtherMetrics(t *testing.T) {
	body := `{"online":999,"watched_show":{"num":888}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"code":0,"data":%s}`, body)
	}))
	defer server.Close()
	client, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP, client.LiveBase = server.Client(), server.URL
	client.SetAccount(domain.Account{UID: "1"})
	if _, err := client.RoomAudience(context.Background(), 2); err == nil {
		t.Fatal("missing audience was reported as zero or another metric")
	}
	body = `{"onlineNum":0}`
	if count, err := client.RoomAudience(context.Background(), 2); err != nil || count != 0 {
		t.Fatalf("explicit zero: count=%d err=%v", count, err)
	}
}

func TestAudienceRequestKeepsCapturedCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ruid"); got != "1" {
			t.Errorf("audience used another account UID: %q", got)
		}
		cookie, err := r.Cookie("SESSDATA")
		if err != nil || cookie.Value != "original" {
			t.Errorf("audience used refreshed credentials: cookie=%v err=%v", cookie, err)
		}
		fmt.Fprint(w, `{"code":0,"data":{"onlineNum":7}}`)
	}))
	defer server.Close()
	client, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP, client.LiveBase = server.Client(), server.URL
	client.SetAccount(domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "original"}})
	request := client.AudienceRequest(2)
	// 覆盖票据原地刷新和整个账号替换两种身份变更。
	client.account.Cookies["SESSDATA"] = "refreshed"
	client.SetAccount(domain.Account{UID: "3", Cookies: map[string]string{"SESSDATA": "replacement"}})
	if count, err := request(context.Background()); err != nil || count != 7 {
		t.Fatalf("captured audience request: count=%d err=%v", count, err)
	}
}
