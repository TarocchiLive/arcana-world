package presentation

import (
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"arcana-world/internal/danmaku"
)

const publicFields = ".Sequence .RoomID .Kind .User .UID .Text .Gift .CoinType .Count .Amount .Face .MedalName .MedalLevel .Mystery .GuardUnit .Time .Deleted .Title .Coins .GuardDuration .Guard .Silent"

type detailField struct{ method, name string }

// 仅允许各协议已知的具名字段，不能借模板访问内部事件或服务对象。
var detailFields = map[string][]detailField{
	"USER_TOAST_MSG":               {{"F", "data.username"}, {"F", "data.role_name"}, {"N", "data.guard_level"}, {"N", "data.num"}, {"F", "data.unit"}},
	"GUIARD_MSG":                   {{"F", "msg"}},
	"LIVE_OPEN_PLATFORM_SEND_GIFT": {{"F", "data.uname"}, {"F", "data.gift_name"}, {"N", "data.gift_num"}},
	"LIVE_OPEN_PLATFORM_GUARD":     {{"F", "data.user_info.uname"}, {"F", "data.role_name"}, {"N", "data.guard_level"}, {"N", "data.guard_num"}, {"F", "data.guard_unit"}},
	"RECALL_DANMU_MSG":             {{"N", "data.target_id"}},
	"WARNING":                      {{"F", "msg"}},
	"ROOM_SILENT_ON":               {{"N", "data.level"}, {"N", "data.second"}, {"F", "data.msg"}, {"F", "data.type"}},
	"room_admin_entrance":          {{"N", "uid"}, {"F", "msg"}},
	"ROOM_ADMIN_REVOKE":            {{"N", "uid"}, {"F", "msg"}},
}

func compileTemplate(id, source string) (*template.Template, error) {
	if len(source) > maxTemplateSource {
		return nil, fmt.Errorf("template exceeds 16 KiB")
	}
	if err := rejectDefinitions(source); err != nil {
		return nil, err
	}
	t, err := template.New(id).Option("missingkey=error").Parse(source)
	if err != nil {
		return nil, err
	}
	if len(t.Templates()) != 1 {
		return nil, fmt.Errorf("define/block are not allowed")
	}
	v := treeValidator{id: id, source: source}
	if err := v.list(t.Tree.Root, true, 0); err != nil {
		return nil, err
	}
	return t, nil
}

type treeValidator struct {
	id, source string
	nodes      int
}

func (v *treeValidator) failure(n parse.Node, text string) error {
	pos := int(n.Position())
	if pos > len(v.source) {
		pos = len(v.source)
	}
	return fmt.Errorf("line %d: %s", 1+strings.Count(v.source[:pos], "\n"), text)
}

func (v *treeValidator) visit(n parse.Node, depth int) error {
	v.nodes++
	if depth > 32 || v.nodes > 2048 {
		return v.failure(n, "template complexity limit exceeded")
	}
	return nil
}

func (v *treeValidator) list(list *parse.ListNode, root bool, depth int) error {
	if list == nil {
		return nil
	}
	for _, n := range list.Nodes {
		if err := v.visit(n, depth); err != nil {
			return err
		}
		switch n := n.(type) {
		case *parse.TextNode, *parse.CommentNode:
		case *parse.ActionNode:
			if err := v.pipe(n.Pipe, root, depth+1); err != nil {
				return err
			}
		case *parse.IfNode:
			if err := v.pipe(n.Pipe, root, depth+1); err != nil {
				return err
			}
			if err := v.list(n.List, root, depth+1); err != nil {
				return err
			}
			if err := v.list(n.ElseList, root, depth+1); err != nil {
				return err
			}
		case *parse.WithNode:
			if err := v.pipe(n.Pipe, root, depth+1); err != nil {
				return err
			}
			// 只允许 with 生成标量上下文，根对象必须通过 $ 引用。
			if err := v.list(n.List, false, depth+1); err != nil {
				return err
			}
			if err := v.list(n.ElseList, root, depth+1); err != nil {
				return err
			}
		default:
			return v.failure(n, "only text, actions, if and with are allowed")
		}
	}
	return nil
}

func (v *treeValidator) pipe(p *parse.PipeNode, root bool, depth int) error {
	if err := v.visit(p, depth); err != nil {
		return err
	}
	if len(p.Decl) != 0 || p.IsAssign {
		return v.failure(p, "variables are not allowed")
	}
	for i, cmd := range p.Cmds {
		if err := v.command(cmd, root, depth+1, i > 0); err != nil {
			return err
		}
	}
	return nil
}

func (v *treeValidator) command(cmd *parse.CommandNode, root bool, depth int, piped bool) error {
	if err := v.visit(cmd, depth); err != nil {
		return err
	}
	if len(cmd.Args) == 0 {
		return v.failure(cmd, "empty command")
	}
	first := cmd.Args[0]
	argc := len(cmd.Args) - 1
	if piped {
		argc++
	}
	switch n := first.(type) {
	case *parse.IdentifierNode:
		min, max := 0, 0
		switch n.Ident {
		case "and", "or":
			min, max = 2, 32
		case "not", "len":
			min, max = 1, 1
		case "eq":
			min, max = 2, 32
		case "ne", "lt", "le", "gt", "ge":
			min, max = 2, 2
		default:
			return v.failure(n, "function is not allowed: "+n.Ident)
		}
		if argc < min || argc > max {
			return v.failure(n, "invalid function argument count")
		}
	case *parse.FieldNode:
		if !root {
			return v.failure(n, "field access inside with requires the root prefix $")
		}
		if err := v.field(n, n.Ident, cmd.Args[1:], piped); err != nil {
			return err
		}
	case *parse.VariableNode:
		if len(n.Ident) < 2 || n.Ident[0] != "$" {
			return v.failure(n, "only root field references are allowed")
		}
		if err := v.field(n, n.Ident[1:], cmd.Args[1:], piped); err != nil {
			return err
		}
	default:
		if argc != 0 {
			return v.failure(n, "value cannot be called")
		}
		if err := v.value(n, root, depth+1); err != nil {
			return err
		}
	}
	for _, arg := range cmd.Args[1:] {
		if err := v.value(arg, root, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (v *treeValidator) value(n parse.Node, root bool, depth int) error {
	if err := v.visit(n, depth); err != nil {
		return err
	}
	switch n := n.(type) {
	case *parse.StringNode, *parse.NumberNode, *parse.BoolNode, *parse.NilNode:
		return nil
	case *parse.DotNode:
		if root {
			return v.failure(n, "the complete event object cannot be exposed")
		}
		return nil
	case *parse.FieldNode:
		if !root {
			return v.failure(n, "field access inside with requires the root prefix $")
		}
		return v.field(n, n.Ident, nil, false)
	case *parse.VariableNode:
		if len(n.Ident) < 2 || n.Ident[0] != "$" {
			return v.failure(n, "only root field references are allowed")
		}
		return v.field(n, n.Ident[1:], nil, false)
	case *parse.PipeNode:
		return v.pipe(n, root, depth+1)
	default:
		return v.failure(n, "expression is not allowed")
	}
}

func (v *treeValidator) field(n parse.Node, path []string, args []parse.Node, piped bool) error {
	if piped {
		return v.failure(n, "piped method arguments are not allowed")
	}
	if len(path) == 2 && path[0] == "Time" && path[1] == "Format" {
		if len(args) != 1 {
			return v.failure(n, "Time.Format requires one literal layout")
		}
		layout, ok := args[0].(*parse.StringNode)
		if !ok || len(layout.Text) > 256 {
			return v.failure(n, "Time.Format requires a literal layout of at most 256 bytes")
		}
		return nil
	}
	if len(path) != 1 {
		return v.failure(n, "unknown or disallowed field/method")
	}
	name := path[0]
	if name == "F" || name == "N" {
		count := 1
		if name == "F" {
			count = 2
		}
		if len(args) != count {
			return v.failure(n, "invalid field accessor argument count")
		}
		key, ok := args[0].(*parse.StringNode)
		if !ok {
			return v.failure(n, "field accessor requires a literal field name")
		}
		allowed := false
		for _, field := range detailFields[v.id] {
			if field.method == name && field.name == key.Text {
				allowed = true
				break
			}
		}
		if !allowed {
			return v.failure(n, "field is not available for this event: "+key.Text)
		}
		if name == "F" {
			if _, ok := args[1].(*parse.StringNode); !ok {
				return v.failure(n, "F requires a literal fallback")
			}
		}
		return nil
	}
	if len(args) != 0 {
		return v.failure(n, "field does not accept arguments")
	}
	if !strings.Contains(" "+publicFields+" ", " ."+name+" ") {
		return v.failure(n, "unknown or disallowed field: "+name)
	}
	return nil
}

// 语法树会丢弃未使用的空 define；因此在解析前也检查动作的首个词。
// 扫描时跳过注释及字符串，避免把示例文字误判为动作。
func rejectDefinitions(source string) error {
	for offset := 0; offset < len(source); {
		start := strings.Index(source[offset:], "{{")
		if start < 0 {
			return nil
		}
		start += offset
		i := start + 2
		if i < len(source) && source[i] == '-' {
			i++
		}
		for i < len(source) && strings.ContainsRune(" \t\r\n", rune(source[i])) {
			i++
		}
		if strings.HasPrefix(source[i:], "/*") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return nil
			}
			offset = i + 2 + end + 2
			continue
		}
		word := i
		for i < len(source) && source[i] >= 'a' && source[i] <= 'z' {
			i++
		}
		if source[word:i] == "define" || source[word:i] == "block" {
			return fmt.Errorf("line %d: define/block are not allowed", 1+strings.Count(source[:start], "\n"))
		}
		var quote byte
		for i < len(source) {
			c := source[i]
			if quote != 0 {
				if c == '\\' && quote != '`' {
					i += 2
					continue
				}
				if c == quote {
					quote = 0
				}
			} else if c == '"' || c == '\'' || c == '`' {
				quote = c
			} else if c == '}' && i+1 < len(source) && source[i+1] == '}' {
				i += 2
				break
			}
			i++
		}
		offset = i
	}
	return nil
}

// 用缺省和完整事件提前发现参数类型错误，数据相关分支仍由运行时回退保护。
func validateExecution(t *template.Template, id string, english bool) error {
	event := danmaku.Event{Kind: id, Time: time.Date(2006, 1, 2, 15, 4, 5, 0, time.Local)}
	if len(detailFields[id]) != 0 || id == "ROOM_SILENT_OFF" {
		event.Kind, event.Text = "detail", id
	}
	for _, populated := range []bool{false, true} {
		if populated {
			event.User, event.UID, event.Gift = "Viewer", "123", "Gift"
			event.Count, event.Amount, event.RoomID, event.Sequence = 2, 30, 123, 1
			event.CoinType, event.GuardUnit = "gold", "月"
			if event.Kind != "detail" {
				event.Text = "Message"
			}
			for _, field := range detailFields[id] {
				value := "Value"
				if field.method == "N" {
					value = "2"
				}
				event.Fields = append(event.Fields, danmaku.EventField{Name: strings.TrimPrefix(field.name, "data."), Value: value})
			}
		}
		var out boundedOutput
		if err := t.Execute(&out, eventData{Event: &event, english: english}); err != nil {
			return err
		}
	}
	return nil
}
