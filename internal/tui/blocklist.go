package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	tea "charm.land/bubbletea/v2"
)

// 屏蔽词弹层用 editKind 记录层级：分类列表、某分类的词列表、向某分类添加词的表单；Esc 逐级返回。
const (
	blocklistListsKind = "blocklist"
	blocklistWordsKind = "blocklist:"
	blocklistAddKind   = "blocklist-add:"
	blocklistAddItem   = "+add"
)

type blocklistSpec struct {
	id          string
	label, help i18n.Key
}

var blocklistLists = [...]blocklistSpec{
	{"global", i18n.BlocklistGlobal, i18n.BlocklistGlobalHelp},
	{"tts", i18n.BlocklistTTS, i18n.BlocklistTTSHelp},
}

// blocklistIndex 返回分类在 blocklistLists 中的位置；未知分类返回 -1。
func blocklistIndex(id string) int {
	return slices.IndexFunc(blocklistLists[:], func(list blocklistSpec) bool { return list.id == id })
}

func blocklistInfo(id string) (label, help i18n.Key, ok bool) {
	if i := blocklistIndex(id); i >= 0 {
		return blocklistLists[i].label, blocklistLists[i].help, true
	}
	return "", "", false
}

// blocklistPicker 判断 editKind 是否为屏蔽词选择器；id 为空表示分类列表，否则为该分类的词列表。
func blocklistPicker(kind string) (id string, ok bool) {
	if kind == blocklistListsKind {
		return "", true
	}
	return strings.CutPrefix(kind, blocklistWordsKind)
}

func (m *Model) blocklistMenuLabel() string {
	b := m.config.Blocklist
	return fmt.Sprintf(i18n.T(i18n.BlocklistMenu), len(b.Global)+len(b.TTS))
}

// showBlocklist 打开分类列表（id 为空）或某个分类的词列表，并保留可用的选中位置。
func (m *Model) showBlocklist(id string) tea.Cmd {
	selected := m.selected
	m.input.SetValue("")
	m.input.Blur()
	m.choices = nil
	b := m.config.Blocklist
	if id == "" {
		for _, list := range blocklistLists {
			m.choices = append(m.choices, choice{fmt.Sprintf(i18n.T(i18n.BlocklistCount), i18n.T(list.label), len(*b.List(list.id))), list.id})
		}
		cmd := m.pick(blocklistListsKind, i18n.T(i18n.BlocklistTitle))
		m.selected = min(selected, len(m.choices)-1)
		return cmd
	}
	label, _, ok := blocklistInfo(id)
	if !ok {
		return nil
	}
	m.choices = append(m.choices, choice{i18n.T(i18n.BlocklistAdd), blocklistAddItem})
	for _, word := range *b.List(id) {
		m.choices = append(m.choices, choice{clean(word), word})
	}
	cmd := m.pick(blocklistWordsKind+id, i18n.T(label))
	m.selected = min(selected, len(m.choices)-1)
	return cmd
}

// blocklistHint 返回选择器下方的说明：分类列表说明匹配规则，词列表说明生效范围。
func (m *Model) blocklistHint() string {
	id, _ := blocklistPicker(m.editKind)
	if id == "" {
		return i18n.T(i18n.BlocklistRules)
	}
	_, help, _ := blocklistInfo(id)
	return i18n.T(help)
}

func (m *Model) chooseBlocklist(value string) tea.Cmd {
	id, _ := blocklistPicker(m.editKind)
	if id == "" {
		m.selected = 0
		return m.showBlocklist(value)
	}
	if value == blocklistAddItem {
		label, _, _ := blocklistInfo(id)
		return m.form(blocklistAddKind+id, fmt.Sprintf(i18n.T(i18n.BlocklistAddPrompt), i18n.T(label)), "", false)
	}
	b := m.config.Blocklist.Clone()
	list := b.List(id)
	if list == nil {
		return nil
	}
	*list = slices.DeleteFunc(*list, func(word string) bool { return word == value })
	return m.saveBlocklist(b, fmt.Sprintf(i18n.T(i18n.BlocklistRemoved), clean(value)))
}

func (m *Model) submitBlockWords(id, value string) tea.Cmd {
	b := m.config.Blocklist.Clone()
	list := b.List(id)
	if list == nil {
		return nil
	}
	// 先单独规范化本次输入，区分“没有有效词”和“全部重复”。
	words, err := presentation.NormalizeBlockWords([]string{value})
	if err != nil {
		m.warnStatus(blocklistError(err))
		return nil
	}
	if len(words) == 0 {
		m.warnStatus(i18n.T(i18n.BlocklistEmpty))
		return nil
	}
	before := len(*list)
	*list = append(*list, words...)
	normalized, err := b.Normalize()
	if err != nil {
		m.warnStatus(blocklistError(err))
		return nil
	}
	added := len(*normalized.List(id)) - before
	m.selected = 0
	m.showBlocklist(id)
	if added == 0 {
		m.log(i18n.T(i18n.BlocklistDuplicate))
		return nil
	}
	return m.saveBlocklist(normalized, fmt.Sprintf(i18n.T(i18n.BlocklistAdded), added))
}

// setBlocklistWords 保存网页提交的某分类完整词列表（JSON 字符串数组）；id 已由网页字段白名单校验。
func (m *Model) setBlocklistWords(id, value string) (tea.Cmd, error) {
	var words []string
	if err := json.Unmarshal([]byte(value), &words); err != nil {
		return nil, errors.New(i18n.T(i18n.WebRequestInvalid))
	}
	b := m.config.Blocklist.Clone()
	*b.List(id) = words
	b, err := b.Normalize()
	if err != nil {
		return nil, errors.New(blocklistError(err))
	}
	if b.Equal(m.config.Blocklist) {
		return nil, nil
	}
	return m.saveBlocklist(b, i18n.T(i18n.TUILogSettingsSaved)), nil
}

// blocklistBack 处理 Esc：表单回到词列表，词列表回到分类列表；分类列表交给通用关闭逻辑。
func (m *Model) blocklistBack() (tea.Cmd, bool) {
	if id, ok := strings.CutPrefix(m.editKind, blocklistAddKind); ok && m.mode == "form" {
		m.selected = 0
		return m.showBlocklist(id), true
	}
	if id, ok := strings.CutPrefix(m.editKind, blocklistWordsKind); ok && m.mode == "pick" {
		// 回到分类列表时选中刚离开的分类。
		m.selected = max(0, blocklistIndex(id))
		return m.showBlocklist(""), true
	}
	return nil, false
}

func blocklistError(err error) string {
	switch err {
	case presentation.ErrBlockWordTooLong:
		return fmt.Sprintf(i18n.T(i18n.BlocklistTooLong), presentation.MaxBlockWordRunes)
	case presentation.ErrTooManyBlockWords:
		return fmt.Sprintf(i18n.T(i18n.BlocklistTooMany), presentation.MaxBlockWords)
	}
	return clean(err.Error())
}

// saveBlocklist 保存已规范化的屏蔽词，成功后刷新仍打开的屏蔽词列表。
func (m *Model) saveBlocklist(b presentation.Blocklist, notice string) tea.Cmd {
	op := operation[struct{}]{label: i18n.TUIOperationSaveSettings, handle: func(m *Model, _ struct{}, err error, label i18n.Key) tea.Cmd {
		if cmd, failed := m.resultError(label, err); failed {
			return cmd
		}
		if m.websocket != nil {
			m.setChatBroadcast(m.websocket.Publish)
		}
		// 仅刷新仍打开的屏蔽词列表，不重新进入已关闭的弹层。
		if id, ok := blocklistPicker(m.editKind); ok && m.mode == "pick" {
			m.showBlocklist(id)
		}
		m.log(notice)
		return m.finishResult()
	}}
	return work(m, op, func(ctx context.Context) (struct{}, error) {
		if err := m.session.Lock(ctx); err != nil {
			return struct{}{}, err
		}
		defer m.session.Unlock()
		// 基于最新快照保存，避免覆盖其他设置变更。
		cfg := m.store.Config()
		cfg.Blocklist = b
		return struct{}{}, m.store.SaveConfig(cfg)
	})
}
