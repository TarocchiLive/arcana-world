// Package webui 提供可选的本机浏览器控制面板。
package webui

import "context"

type Command struct {
	Page     string `json:"page"`
	Action   string `json:"action"`
	Value    string `json:"value"`
	Revision uint64 `json:"revision"`
}

type Snapshot struct {
	Revision        uint64        `json:"revision"`
	Language        string        `json:"language"`
	Page            string        `json:"page"`
	Busy            bool          `json:"busy"`
	Status          string        `json:"status"`
	Warning         bool          `json:"warning"`
	Account         string        `json:"account"`
	AccountUID      string        `json:"accountUID"`
	Room            *Room         `json:"room,omitempty"`
	OBS             bool          `json:"obs"`
	OBSConnecting   bool          `json:"obsConnecting"`
	StreamAvailable bool          `json:"streamAvailable"`
	ChatConnected   bool          `json:"chatConnected"`
	Audience        string        `json:"audience"`
	Overlay         bool          `json:"overlay"`
	TTS             bool          `json:"tts"`
	Fields          []Field       `json:"fields"`
	Accounts        []Account     `json:"accounts"`
	Help            []HelpArticle `json:"help"`
	Messages        []Message     `json:"messages"`
	Logs            []string      `json:"logs"`
	Interaction     *Interaction  `json:"interaction,omitempty"`
}

type Room struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Area         string `json:"area"`
	Live         bool   `json:"live"`
	Cover        string `json:"cover"`
	Announcement string `json:"announcement"`
	CoverStatus  string `json:"coverStatus"`
}

type Option struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Selected bool   `json:"selected,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type Field struct {
	ID       string   `json:"id"`
	Group    string   `json:"group"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Value    string   `json:"value"`
	Options  []Option `json:"options,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
	Min      float64  `json:"min,omitempty"`
	Max      float64  `json:"max,omitempty"`
	Step     float64  `json:"step,omitempty"`
}

type Account struct {
	UID    string `json:"uid"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type HelpArticle struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

type Message struct {
	ID         string `json:"id"`
	Time       string `json:"time"`
	Name       string `json:"name"`
	Text       string `json:"text"`
	Kind       string `json:"kind"`
	UID        string `json:"uid,omitempty"`
	MedalName  string `json:"medalName,omitempty"`
	MedalLevel int64  `json:"medalLevel,omitempty"`
	Mystery    bool   `json:"mystery,omitempty"`
	Deleted    bool   `json:"deleted,omitempty"`
}

type Interaction struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Text     string   `json:"text"`
	Value    string   `json:"value"`
	Secret   bool     `json:"secret"`
	Choices  []Option `json:"choices"`
	Image    string   `json:"image,omitempty"`
	URL      string   `json:"url,omitempty"`
	Target   string   `json:"target"`
	Revision uint64   `json:"revision"`
	Back     bool     `json:"back,omitempty"`
	Members  *Members `json:"members,omitempty"`
}

// Members 保留上次成功名单及加载状态，供浏览器增量更新。
type Members struct {
	Total    int64    `json:"total"`
	Loaded   bool     `json:"loaded"`
	Loading  bool     `json:"loading"`
	Previous bool     `json:"previous"`
	Entries  []Member `json:"entries"`
}

type Member struct {
	Rank       int64  `json:"rank"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
	Role       string `json:"role"`
	Score      string `json:"score"`
	MedalName  string `json:"medalName"`
	MedalLevel int64  `json:"medalLevel"`
	Mystery    bool   `json:"mystery"`
}

// Request 将请求交给已有的单写入者事件循环；HTTP 层不读取业务状态。
type Request func(context.Context, string, *Command) (Snapshot, error)
