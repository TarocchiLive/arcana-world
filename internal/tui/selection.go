package tui

import (
	"strconv"
	"strings"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	in.Prompt = i18n.T(i18n.TUISelectionTitlePrompt)
	in.SetValue(selectionText(current))
	in.CursorEnd()
	in.Focus()
	s := &roomSelection{titleMode: true, input: in, selected: -1, window: initialSelectionWindow}
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
	s := &roomSelection{currentID: currentID, window: initialSelectionWindow}
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
	key, isKey := msg.(tea.KeyMsg)
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
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			text := string(key.Runes)
			if key.Type == tea.KeySpace {
				text = " "
			}
			s.query = append(s.query, []rune(selectionText(text))...)
			s.rebuild()
		}
	}
	return nil, nil
}

func (s *roomSelection) View(width, height int) string {
	width, height = max(1, width), max(1, height)
	var lines []string
	var labels []string
	if s.titleMode {
		s.input.Width = max(1, width-ansi.StringWidth(s.input.Prompt)-1)
		lines = append(lines, accent.Render(i18n.T(i18n.TUISelectionTitleHeading)), s.input.View(), muted.Render(i18n.T(i18n.TUISelectionTitleControls)))
		labels = s.history
		if len(labels) == 0 {
			lines = append(lines, muted.Render(i18n.T(i18n.TUISelectionTitleEmpty)))
		}
	} else {
		location := i18n.T(i18n.TUISelectionCategoryRoot)
		if s.parent != "" {
			location = s.parent
		}
		if len(s.query) > 0 {
			location = i18n.T(i18n.TUISelectionCategorySearch)
		}
		lines = append(lines, accent.Render(i18n.T(i18n.TUISelectionCategoryHeading)+selectionText(location)), i18n.T(i18n.TUISelectionSearchPrompt)+selectionText(string(s.query)), muted.Render(i18n.T(i18n.TUISelectionCategoryControls)))
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
		if len(labels) == 0 {
			lines = append(lines, muted.Render(i18n.T(i18n.TUISelectionCategoryEmpty)))
		}
	}
	// 在较矮终端中保持编辑器或搜索框及一个可选行可见。
	if height < 4 {
		if height == 1 && len(labels) > 0 && s.selected >= 0 {
			lines = nil
		} else {
			lines = lines[1:2]
		}
	}
	s.window = max(1, height-len(lines))
	start := max(0, s.selected-s.window+1)
	end := min(len(labels), start+s.window)
	for i := start; i < end; i++ {
		label := "  " + selectionText(labels[i])
		if i == s.selected {
			label = selectedStyle.Render(ansi.Truncate("› "+selectionText(labels[i]), width, "…"))
		}
		lines = append(lines, label)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return strings.Join(lines, "\n")
}
