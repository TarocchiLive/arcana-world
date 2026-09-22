package tui

import (
	"strconv"
	"strings"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const initialSelectionWindow = 8

type selectionResult struct {
	Canceled bool
	Title    string
	Area     *domain.Area
}

type selectionItem struct {
	parent string
	area   *domain.Area
	recent bool
}

type roomSelection struct {
	theme     tuiTheme
	titleMode bool
	input     textinput.Model
	history   []string
	all       []domain.Area
	recent    []domain.Area
	currentID int64
	parent    string
	query     []rune
	items     []selectionItem
	selected  int
	window    int
}

func selectionText(s string) string {
	return strings.ReplaceAll(clean(s), "\n", " ")
}

func newTitleSelection(current string, recent []string) *roomSelection {
	in := textinput.New()
	in.SetVirtualCursor(false)
	in.Prompt = i18n.T(i18n.TUISelectionTitlePrompt)
	themeFor("", true).styleInput(&in)
	in.SetValue(selectionText(current))
	in.CursorEnd()
	in.Focus()
	s := &roomSelection{theme: themeFor("", true), titleMode: true, input: in, selected: -1, window: initialSelectionWindow}
	seen := make(map[string]bool)
	for _, title := range recent {
		title = selectionText(title)
		if strings.TrimSpace(title) != "" && !seen[title] {
			seen[title] = true
			s.history = append(s.history, title)
		}
	}
	return s
}

func newAreaSelection(all, recent []domain.Area, currentID int64) *roomSelection {
	s := &roomSelection{theme: themeFor("", true), currentID: currentID, window: initialSelectionWindow}
	canonical := make(map[int64]domain.Area, len(all))
	for _, area := range all {
		if area.ID <= 0 {
			continue
		}
		if _, exists := canonical[area.ID]; exists {
			continue
		}
		canonical[area.ID] = area
		s.all = append(s.all, area)
	}
	seen := make(map[int64]bool)
	for _, area := range recent {
		if current, ok := canonical[area.ID]; ok && !seen[area.ID] {
			s.recent = append(s.recent, current)
			seen[area.ID] = true
		}
	}
	s.rebuild()
	return s
}

func (s *roomSelection) Init() tea.Cmd {
	if s.titleMode {
		return textinput.Blink
	}
	return nil
}

func (s *roomSelection) rebuild() {
	s.items = nil
	s.selected = 0
	query := strings.ToLower(strings.TrimSpace(string(s.query)))
	if query != "" {
		for i := range s.all {
			a := &s.all[i]
			if strings.Contains(strings.ToLower(a.Parent+" / "+a.Name+" "+strconv.FormatInt(a.ID, 10)), query) {
				s.items = append(s.items, selectionItem{area: a})
			}
		}
		return
	}
	if s.parent != "" {
		for i := range s.all {
			if s.all[i].Parent == s.parent {
				s.items = append(s.items, selectionItem{area: &s.all[i]})
			}
		}
		return
	}
	for i := range s.recent {
		s.items = append(s.items, selectionItem{area: &s.recent[i], recent: true})
	}
	parents := make(map[string]bool)
	for _, a := range s.all {
		if !parents[a.Parent] {
			parents[a.Parent] = true
			s.items = append(s.items, selectionItem{parent: a.Parent})
		}
	}
}

func (s *roomSelection) move(key string, count, first int) {
	switch key {
	case "up":
		s.selected--
	case "down":
		s.selected++
	case "pgup":
		s.selected -= max(1, s.window)
	case "pgdown":
		s.selected += max(1, s.window)
	case "home":
		s.selected = first
	case "end":
		s.selected = count - 1
	}
	s.selected = max(first, min(s.selected, count-1))
}

func (s *roomSelection) Update(msg tea.Msg) (*selectionResult, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		if s.titleMode {
			s.selected = -1
		} else {
			s.query = append(s.query, []rune(selectionText(paste.Content))...)
			s.rebuild()
			return nil, nil
		}
	}
	key, isKey := msg.(tea.KeyPressMsg)
	if s.titleMode {
		if isKey {
			switch key.String() {
			case "esc":
				return &selectionResult{Canceled: true}, nil
			case "up", "down", "pgup", "pgdown":
				s.move(key.String(), len(s.history), -1)
				return nil, nil
			case "tab":
				if s.selected >= 0 {
					s.input.SetValue(s.history[s.selected])
					s.input.CursorEnd()
					s.selected = -1
				}
				return nil, nil
			case "enter":
				title := s.input.Value()
				if s.selected >= 0 {
					title = s.history[s.selected]
				}
				if strings.TrimSpace(title) != "" {
					return &selectionResult{Title: title}, nil
				}
				return nil, nil
			default:
				s.selected = -1
			}
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return nil, cmd
	}
	if !isKey {
		return nil, nil
	}
	switch key.String() {
	case "esc", "left":
		if len(s.query) > 0 || s.parent != "" {
			s.query = nil
			s.parent = ""
			s.rebuild()
			return nil, nil
		}
		return &selectionResult{Canceled: true}, nil
	case "ctrl+u":
		s.query = nil
		s.rebuild()
	case "up", "down", "pgup", "pgdown", "home", "end":
		s.move(key.String(), len(s.items), 0)
	case "enter", "right":
		if len(s.items) == 0 {
			return nil, nil
		}
		item := s.items[s.selected]
		if item.area != nil {
			if key.String() == "enter" {
				area := *item.area
				return &selectionResult{Area: &area}, nil
			}
		} else {
			s.parent = item.parent
			s.rebuild()
		}
	case "backspace", "ctrl+h":
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.rebuild()
		}
	default:
		if key.Text != "" {
			s.query = append(s.query, []rune(selectionText(key.Text))...)
			s.rebuild()
		}
	}
	return nil, nil
}

// resize 负责编辑器尺寸，渲染过程不得改变横向滚动偏移。
func (s *roomSelection) resize(width, height int) {
	width = max(1, width)
	if s.titleMode {
		s.input.Prompt = ansi.Truncate(i18n.T(i18n.TUISelectionTitlePrompt), max(0, width-2), "")
		resizeTextInput(&s.input, max(1, width-ansi.StringWidth(s.input.Prompt)-1))
	}
	_, _, s.window = s.layout(width, height)
}

func (s *roomSelection) layout(width, height int) (header, footer, window int) {
	width, height = max(1, width), max(1, height)
	header = 2
	if height < 4 {
		header = 1
		if height == 1 && s.selected >= 0 {
			header = 0
		}
	}
	controls := i18n.T(i18n.TUISelectionCategoryControls)
	if s.titleMode {
		controls = i18n.T(i18n.TUISelectionTitleControls)
	}
	footer = min(strings.Count(s.theme.hintText(controls, width), "\n")+1, max(0, height-header-1))
	return header, footer, max(1, height-header-footer)
}

func (s *roomSelection) View(width, height int) string {
	width, height = max(1, width), max(1, height)
	header, footer, window := s.layout(width, height)
	var heading, editor, controls, empty string
	var labels []string
	if s.titleMode {
		heading = i18n.T(i18n.TUISelectionTitleHeading)
		editor = s.input.View()
		controls = i18n.T(i18n.TUISelectionTitleControls)
		empty = i18n.T(i18n.TUISelectionTitleEmpty)
		labels = s.history
	} else {
		location := i18n.T(i18n.TUISelectionCategoryRoot)
		if s.parent != "" {
			location = s.parent
		}
		if len(s.query) > 0 {
			location = i18n.T(i18n.TUISelectionCategorySearch)
		}
		heading = i18n.T(i18n.TUISelectionCategoryHeading) + selectionText(location)
		editor = s.searchView(width)
		controls = i18n.T(i18n.TUISelectionCategoryControls)
		empty = i18n.T(i18n.TUISelectionCategoryEmpty)
		for _, item := range s.items {
			label := selectionText(item.parent) + " →"
			if item.area != nil {
				a := item.area
				label = selectionText(a.Parent+" / "+a.Name) + " #" + strconv.FormatInt(a.ID, 10)
				if item.recent {
					label = i18n.T(i18n.TUISelectionRecentPrefix) + label
				}
				if a.ID == s.currentID {
					label += i18n.T(i18n.TUISelectionCurrentSuffix)
				}
			}
			labels = append(labels, label)
		}
	}
	var lines []string
	if header == 2 {
		lines = append(lines, s.theme.sectionTitle(heading, width))
	}
	if header > 0 {
		lines = append(lines, ansi.Truncate(editor, width, ""))
	}
	start := max(0, s.selected-window+1)
	end := min(len(labels), start+window)
	for i := start; i < end; i++ {
		lines = append(lines, s.theme.listRow(labels[i], width, i == s.selected))
	}
	if len(labels) == 0 {
		lines = append(lines, s.theme.hintText(empty, width))
	}
	if footer > 0 {
		help := strings.Split(s.theme.hintText(controls, width), "\n")
		lines = append(lines, help[:min(footer, len(help))]...)
	}
	// 窄终端中的空状态换行后可能超过可用高度。
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	return strings.Join(lines[:min(height, len(lines))], "\n")
}

// 在可见搜索文字后预留一个显示格，供终端光标定位。
func (s *roomSelection) searchView(width int) string {
	prompt := ansi.Truncate(i18n.T(i18n.TUISelectionSearchPrompt), max(0, width-1), "")
	query := selectionText(string(s.query))
	available := max(0, width-ansi.StringWidth(prompt)-1)
	if cells := ansi.StringWidth(query); cells > available {
		query = ansi.TruncateLeft(query, cells-available, "")
	}
	return s.theme.accent.Render(prompt) + s.theme.textStyle.Render(query)
}

func (s *roomSelection) cursor(width, height int) *tea.Cursor {
	header, _, _ := s.layout(width, height)
	if header == 0 {
		return nil
	}
	var c *tea.Cursor
	if s.titleMode {
		c = textInputCursor(s.input)
	} else {
		c = tea.NewCursor(ansi.StringWidth(s.searchView(width)), 0)
		c.Color = s.theme.accentColor
		c.Blink = true
	}
	if c != nil {
		c.Y += header - 1
	}
	return c
}
