package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"arcana-world/internal/bili"
	"arcana-world/internal/danmaku"
	"arcana-world/internal/store"
)

// Session 固定启动账号；方法由传输层串行调用，不提供账号或配置写入入口。
type Session struct {
	uid            string
	version        string
	authentication string
	client         *bili.Client
	history        *danmaku.History
	listener       interface {
		Snapshot() danmaku.Snapshot
		Close() error
	}
}

// NewSession 最多用十秒验证启动账号；失败后仅保留状态查询，重启才会重新验证。
func NewSession(ctx context.Context, s *store.Store, version string) (*Session, error) {
	history, err := danmaku.Open(s.Dir())
	if err != nil {
		return nil, errors.New("bot history unavailable; check exclusive data-directory access")
	}
	cfg := s.Config()
	listener := danmaku.NewListener(ctx, history, s.DeviceID())
	session := &Session{uid: cfg.ActiveUID, version: version, authentication: "unauthenticated", history: history, listener: listener}
	listener.Configure(nil, cfg.Proxy, !cfg.DanmakuDisabled)
	if cfg.ActiveUID == "" {
		return session, nil
	}
	account, err := s.Load(cfg.ActiveUID)
	if err != nil || account.UID != cfg.ActiveUID || (account.Cookies["DedeUserID"] != "" && account.Cookies["DedeUserID"] != cfg.ActiveUID) || account.Cookies["SESSDATA"] == "" || account.Cookies["bili_jct"] == "" {
		return session, nil
	}
	client, err := bili.New(cfg.Proxy, s.DeviceID())
	if err != nil {
		session.authentication = "not_ready"
		return session, nil
	}
	session.client = client
	client.SetAccount(account)
	validationCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	info, err := client.Validate(validationCtx)
	cancel()
	if err != nil {
		var validation *bili.ValidationError
		if !errors.As(err, &validation) || !validation.Permanent {
			session.authentication = "not_ready"
		}
		return session, nil
	}
	if info.UID != cfg.ActiveUID {
		return session, nil
	}
	session.authentication = "authenticated"
	listener.Configure(&account, cfg.Proxy, !cfg.DanmakuDisabled)
	return session, nil
}

// Close 必须先等监听器停止写入，再关闭历史库。
func (s *Session) Close() error {
	listenerErr := s.listener.Close()
	historyErr := s.history.Close()
	if s.client != nil {
		s.client.HTTP.CloseIdleConnections()
	}
	if listenerErr != nil || historyErr != nil {
		return errors.New("bot shutdown failed")
	}
	return nil
}

type statusResult struct {
	ProtocolVersion string        `json:"protocol_version"`
	ProductVersion  string        `json:"product_version"`
	UID             string        `json:"uid"`
	Authentication  string        `json:"authentication"`
	ListenerPhase   danmaku.Phase `json:"listener_phase"`
	RoomID          string        `json:"room_id"`
	Methods         []string      `json:"methods"`
}

type publicEvent struct {
	Sequence   string    `json:"sequence"`
	RoomID     string    `json:"room_id"`
	Kind       string    `json:"kind"`
	UID        string    `json:"uid"`
	User       string    `json:"user"`
	Text       string    `json:"text,omitempty"`
	Gift       string    `json:"gift,omitempty"`
	Count      int64     `json:"count,omitempty"`
	Amount     int64     `json:"amount,omitempty"`
	CoinType   string    `json:"coin_type,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
	Deleted    bool      `json:"deleted"`
}

type eventsResult struct {
	RoomID     string        `json:"room_id"`
	Events     []publicEvent `json:"events"`
	NextBefore string        `json:"next_before"`
}

// decodeParams 只接受对象和已声明字段，避免把指定其他账号或房间的请求当作成功。
func decodeParams(raw json.RawMessage, optional bool, allowed ...string) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, optional
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil, false
	}
	for key := range values {
		known := false
		for _, name := range allowed {
			if key == name {
				known = true
				break
			}
		}
		if !known {
			return nil, false
		}
	}
	return values, true
}

func (s *Session) currentRoom() (int64, *Error) {
	if s.authentication == "unauthenticated" {
		return 0, &Error{Code: -32001, Message: "unauthenticated"}
	}
	if s.authentication != "authenticated" {
		return 0, &Error{Code: -32002, Message: "not_ready"}
	}
	snapshot := s.listener.Snapshot()
	if snapshot.AccountUID != s.uid || snapshot.RoomID <= 0 || snapshot.Phase == danmaku.PhaseDisabled || snapshot.Phase == danmaku.PhaseWaiting {
		return 0, &Error{Code: -32002, Message: "not_ready"}
	}
	return snapshot.RoomID, nil
}

func (s *Session) Call(ctx context.Context, method string, params json.RawMessage) (any, *Error) {
	invalid := &Error{Code: -32602, Message: "Invalid params"}
	switch method {
	case "get_status":
		if _, ok := decodeParams(params, true); !ok {
			return nil, invalid
		}
		snapshot := s.listener.Snapshot()
		room := int64(0)
		if snapshot.AccountUID == s.uid && s.authentication == "authenticated" {
			room = snapshot.RoomID
		}
		return statusResult{"2.0", s.version, s.uid, s.authentication, snapshot.Phase, strconv.FormatInt(room, 10), []string{"get_status", "get_recent_events", "send_message"}}, nil
	case "get_recent_events":
		args, ok := decodeParams(params, false, "limit", "before")
		if !ok {
			return nil, invalid
		}
		limit := 20
		if raw := args["limit"]; len(raw) != 0 && (bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &limit) != nil) {
			return nil, invalid
		}
		if limit < 1 || limit > 100 {
			return nil, invalid
		}
		var before uint64
		if raw := args["before"]; len(raw) != 0 {
			var cursor string
			if json.Unmarshal(raw, &cursor) != nil || cursor == "" {
				return nil, invalid
			}
			for _, ch := range cursor {
				if ch < '0' || ch > '9' {
					return nil, invalid
				}
			}
			var err error
			before, err = strconv.ParseUint(cursor, 10, 64)
			if err != nil || before == 0 {
				return nil, invalid
			}
		}
		room, rpcErr := s.currentRoom()
		if rpcErr != nil {
			return nil, rpcErr
		}
		events, err := s.history.Page(room, before, limit)
		if err != nil {
			return nil, &Error{Code: -32004, Message: "history_unavailable"}
		}
		result := eventsResult{RoomID: strconv.FormatInt(room, 10), Events: make([]publicEvent, 0, len(events))}
		for _, event := range events {
			item := publicEvent{Sequence: strconv.FormatUint(event.Sequence, 10), RoomID: result.RoomID, Kind: event.Kind, UID: event.UID, User: event.User, ReceivedAt: event.Time, Deleted: event.Deleted}
			if !event.Deleted {
				item.Text, item.Gift, item.Count, item.Amount, item.CoinType = event.Text, event.Gift, event.Count, event.Amount, event.CoinType
			}
			result.Events = append(result.Events, item)
		}
		if len(events) == limit {
			result.NextBefore = strconv.FormatUint(events[len(events)-1].Sequence, 10)
		}
		return result, nil
	case "send_message":
		args, ok := decodeParams(params, false, "text")
		var text string
		if !ok || json.Unmarshal(args["text"], &text) != nil {
			return nil, invalid
		}
		text = strings.TrimSpace(text)
		if text == "" || utf8.RuneCountInString(text) > 40 {
			return nil, invalid
		}
		room, rpcErr := s.currentRoom()
		if rpcErr != nil {
			return nil, rpcErr
		}
		if err := ctx.Err(); err != nil {
			return nil, &Error{Code: -32002, Message: "not_ready"}
		}
		if err := s.client.SendDanmaku(ctx, room, text); err != nil {
			var network net.Error
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
				return nil, &Error{Code: -32003, Message: "send_failed: delivery_unknown; do not automatically retry"}
			}
			return nil, &Error{Code: -32003, Message: "send_failed"}
		}
		return struct {
			Sent   bool   `json:"sent"`
			RoomID string `json:"room_id"`
		}{true, strconv.FormatInt(room, 10)}, nil
	default:
		return nil, &Error{Code: -32601, Message: "Method not found"}
	}
}

var _ Handler = (*Session)(nil)
