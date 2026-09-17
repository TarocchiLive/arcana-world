package danmaku

import (
	"encoding/json"
	"strconv"
	"strings"

	"arcana-world/internal/i18n"
)

// Catalog entries describe documented wire fields, not guessed command prefixes.
// Nested lists and objects remain structured JSON; numbers never pass through float64.
type commandField struct {
	path string
	kind byte
}

type commandSpec struct {
	title  i18n.Key
	fields []commandField
}

func projectCatalogEvent(p projection, cmd string, root map[string]any) projection {
	spec, ok := modernCommandSpecs[cmd]
	if !ok {
		spec, ok = legacyCommandSpecs[cmd]
	}
	if !ok {
		spec, ok = supplementalCommandSpecs[cmd]
	}
	if !ok {
		return p
	}
	return projectCommandSpec(p, cmd, root, spec)
}

func projectCommandSpec(p projection, cmd string, root map[string]any, spec commandSpec) projection {
	fields := make([]EventField, 0, len(spec.fields))
	for _, field := range spec.fields {
		value, exists := commandPath(root, field.path)
		if !exists || value == nil {
			continue
		}
		text, valid := commandFieldText(value, field.kind)
		if !valid {
			return p
		}
		fields = append(fields, EventField{Name: strings.TrimPrefix(field.path, "data."), Value: text})
	}
	if len(spec.fields) != 0 && len(fields) == 0 {
		return p
	}
	p.event.Kind, p.event.Text, p.event.Title, p.event.Fields = "detail", cmd, string(spec.title), fields
	return p
}

func commandPath(root map[string]any, path string) (any, bool) {
	var value any = root
	for path != "" {
		part, rest, _ := strings.Cut(path, ".")
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = object[part]
		if !ok {
			return nil, false
		}
		path = rest
	}
	return value, true
}

func commandFieldText(value any, kind byte) (string, bool) {
	switch kind {
	case 's':
		text, ok := value.(string)
		return text, ok
	case 'n':
		number, ok := value.(json.Number)
		return string(number), ok
	case 'i':
		text := stringValue(value)
		if text == "" {
			return "", false
		}
		for _, digit := range text {
			if digit < '0' || digit > '9' {
				return "", false
			}
		}
		_, err := strconv.ParseUint(text, 10, 64)
		return text, err == nil
	case 'b':
		flag, ok := value.(bool)
		return strconv.FormatBool(flag), ok
	case 'a':
		if _, ok := value.([]any); !ok {
			return "", false
		}
	case 'o':
		if _, ok := value.(map[string]any); !ok {
			return "", false
		}
	case 'j':
		if text, ok := value.(string); ok {
			if !decodeEventJSON(strings.NewReader(text), &value) {
				return "", false
			}
		}
		if _, ok := value.(map[string]any); !ok {
			return "", false
		}
	default:
		return "", false
	}
	raw, err := json.Marshal(value)
	return string(raw), err == nil
}
