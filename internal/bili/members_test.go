package bili

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arcana-world/internal/domain"
)

func TestRoomFleetMergeRankAndLimit(t *testing.T) {
	rows := make([]string, 0, 23)
	for rank := 25; rank >= 3; rank-- {
		rows = append(rows, fmt.Sprintf(`{"uid":%d,"username":"member%d","rank":%d,"guard_level":3}`, rank, rank, rank))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"code":0,"data":{"info":{"num":"25"},"top3":[{"uid":"3","username":"third","rank":"3","guard_level":"1","medal_info":{"medal_name":"牌","medal_level":"27"}},{"uid":1,"username":"first","rank":1,"guard_level":2,"medal_info":{"medal_name":"牌","medal_level":12}}],"list":[%s]}}`, strings.Join(rows, ","))
	}))
	defer server.Close()
	client, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP, client.LiveBase = server.Client(), server.URL
	client.SetAccount(domain.Account{Cookies: map[string]string{"DedeUserID": "1"}})
	result, err := client.RoomFleet(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 25 || len(result.Members) != 20 {
		t.Fatalf("total or capped list incorrect: %+v", result)
	}
	for index, item := range result.Members {
		wantRank := int64(index + 2)
		if index == 0 {
			wantRank = 1
		}
		if item.UID != wantRank || item.Rank != wantRank {
			t.Fatalf("member %d lost rank or was duplicated: %+v", index, item)
		}
	}
	first, third := result.Members[0], result.Members[1]
	if first.GuardLevel != 2 || first.MedalLevel != 12 || third.GuardLevel != 1 || third.MedalLevel != 27 || third.MedalName != "牌" || third.Name != "third" {
		t.Fatalf("numeric decoding or overlapping top3 precedence incorrect: first=%+v third=%+v", first, third)
	}
}

func TestRoomFleetEmptyAndErrors(t *testing.T) {
	body := `{"code":0,"data":{"info":{"num":0},"top3":[],"list":[]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	client, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP, client.LiveBase = server.Client(), server.URL
	client.SetAccount(domain.Account{UID: "1"})
	result, err := client.RoomFleet(context.Background(), 2)
	if err != nil || result.Total != 0 || len(result.Members) != 0 {
		t.Fatalf("explicit empty room: result=%+v err=%v", result, err)
	}
	for _, response := range []string{
		`{"code":-400,"message":"bad request"}`,
		`{"code":0,"data":{}}`,
		`{"code":0,"data":{"info":{"num":2},"top3":[],"list":[]}}`,
		`{"code":0,"data":{"info":{"num":1},"top3":[{"uid":"invalid","username":"member","rank":1,"guard_level":3}]}}`,
	} {
		body = response
		if _, err := client.RoomFleet(context.Background(), 2); err == nil {
			t.Fatalf("invalid response became a successful list: %s", response)
		}
	}
}

func TestRoomMembersAndFleetSeparateRosters(t *testing.T) {
	online := `{"code":0,"data":{"onlineNum":"3238","OnlineRankItem":[{"uid":"12","name":"ordinary","userRank":"2","score":"45","guard_level":"0","medalInfo":{"medalName":"牌","level":"6"}},{"uid":99,"name":"神秘人","userRank":1,"score":90,"guard_level":0,"is_mystery":true,"uinfo":{"origin_info":{"uid":99,"name":"private"}}},{"uid":12,"name":"duplicate","userRank":3,"score":40,"guard_level":0}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xlive/general-interface/v1/rank/getOnlineGoldRank":
			query := r.URL.Query()
			if query.Get("roomId") != "2" || query.Get("ruid") != "1" || query.Get("page") != "1" || query.Get("pageSize") != "20" {
				t.Errorf("incorrect online ranking query: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, online)
		case "/xlive/app-room/v2/guardTab/topList":
			fmt.Fprint(w, `{"code":0,"data":{"info":{"num":1},"top3":[],"list":[{"uid":23,"username":"fleet-only","rank":1,"guard_level":3}]}}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP, client.LiveBase = server.Client(), server.URL
	client.SetAccount(domain.Account{UID: "1"})
	members, err := client.RoomMembers(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if members.Total != 3238 || len(members.Members) != 2 {
		t.Fatalf("online total or deduplication incorrect: %+v", members)
	}
	anonymous, ordinary := members.Members[0], members.Members[1]
	if !anonymous.Mystery || anonymous.UID != 0 || anonymous.Name != "神秘人" || anonymous.Rank != 1 {
		t.Fatalf("anonymous identity disclosed or rank lost: %+v", anonymous)
	}
	if ordinary.UID != 12 || ordinary.GuardLevel != 0 || ordinary.Score != 45 || ordinary.Rank != 2 || ordinary.MedalName != "牌" || ordinary.MedalLevel != 6 {
		t.Fatalf("ordinary online member lost: %+v", ordinary)
	}
	fleet, err := client.RoomFleet(context.Background(), 2)
	if err != nil || fleet.Total != 1 || len(fleet.Members) != 1 || fleet.Members[0].UID != 23 || fleet.Members[0].GuardLevel != 3 {
		t.Fatalf("fleet mixed with online ranking: result=%+v err=%v", fleet, err)
	}
	online = `{"code":0,"data":{"onlineNum":3238,"OnlineRankItem":[]}}`
	members, err = client.RoomMembers(context.Background(), 2)
	if err != nil || members.Total != 3238 || len(members.Members) != 0 {
		t.Fatalf("empty ranking with online viewers rejected: result=%+v err=%v", members, err)
	}
	for _, response := range []string{
		`{"code":-400,"message":"bad request"}`,
		`{"code":0,"data":{}}`,
		`{"code":0,"data":{"onlineNum":1}}`,
		`{"code":0,"data":{"onlineNum":1,"OnlineRankItem":null}}`,
		`{"code":0,"data":{"onlineNum":1,"OnlineRankItem":[{"uid":"invalid","name":"member","userRank":1}]}}`,
	} {
		online = response
		if _, err := client.RoomMembers(context.Background(), 2); err == nil {
			t.Fatalf("invalid online response became a successful list: %s", response)
		}
	}
}
