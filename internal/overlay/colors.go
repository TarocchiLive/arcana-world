package overlay

import (
	"errors"
	"strings"
)

// Colors keeps event colors independent from the shared text/background opacity.
type Colors struct {
	Text       string `json:"text"`
	Background string `json:"background"`
	Captain    string `json:"captain"`
	Admiral    string `json:"admiral"`
	Governor   string `json:"governor"`
	SuperChat  string `json:"super_chat"`
}

func DefaultColors() Colors {
	return Colors{Text: "#FFFFFF", Background: "#181C24", Captain: "#E88C8C", Admiral: "#E88C8C", Governor: "#E88C8C", SuperChat: "#E6C77A"}
}

// ParseColor accepts exactly #RRGGBB and returns packed 0xRRGGBB.
func ParseColor(value string) (uint32, error) {
	if len(value) != 7 || value[0] != '#' {
		return 0, errors.New("overlay: color must be #RRGGBB")
	}
	var rgb uint32
	for i := 1; i < len(value); i++ {
		var digit byte
		switch c := value[i]; {
		case c >= '0' && c <= '9':
			digit = c - '0'
		case c >= 'a' && c <= 'f':
			digit = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			digit = c - 'A' + 10
		default:
			return 0, errors.New("overlay: color must be #RRGGBB")
		}
		rgb = rgb<<4 | uint32(digit)
	}
	return rgb, nil
}

func (colors Colors) Normalize() (Colors, error) {
	defaults := DefaultColors()
	for _, pair := range [...]struct {
		value    *string
		fallback string
	}{
		{&colors.Text, defaults.Text}, {&colors.Background, defaults.Background},
		{&colors.Captain, defaults.Captain}, {&colors.Admiral, defaults.Admiral},
		{&colors.Governor, defaults.Governor}, {&colors.SuperChat, defaults.SuperChat},
	} {
		if *pair.value == "" {
			*pair.value = pair.fallback
		}
		if _, err := ParseColor(*pair.value); err != nil {
			return Colors{}, err
		}
		*pair.value = strings.ToUpper(*pair.value)
	}
	return colors, nil
}

// TextRun uses UTF-8 byte offsets into Config.Text, with an exclusive End.
type TextRun struct {
	Start, End int
	RGB        uint32
}

// TextRuns resolves a normalized configuration into adjacent colored ranges.
func (cfg Config) TextRuns() []TextRun {
	if cfg.Text == "" {
		return nil
	}
	text, _ := ParseColor(cfg.Colors.Text)
	if cfg.TextRoles == "" {
		return []TextRun{{End: len(cfg.Text), RGB: text}}
	}
	captain, _ := ParseColor(cfg.Colors.Captain)
	admiral, _ := ParseColor(cfg.Colors.Admiral)
	governor, _ := ParseColor(cfg.Colors.Governor)
	superChat, _ := ParseColor(cfg.Colors.SuperChat)
	runs := make([]TextRun, 0, len(cfg.TextRoles))
	start := 0
	for _, role := range []byte(cfg.TextRoles) {
		if start == len(cfg.Text) {
			break
		}
		end := len(cfg.Text)
		if newline := strings.IndexByte(cfg.Text[start:], '\n'); newline >= 0 {
			end = start + newline + 1
		}
		rgb := text
		switch role {
		case 'c':
			rgb = captain
		case 'a':
			rgb = admiral
		case 'g':
			rgb = governor
		case 's':
			rgb = superChat
		}
		if n := len(runs); n > 0 && runs[n-1].RGB == rgb {
			runs[n-1].End = end
		} else {
			runs = append(runs, TextRun{Start: start, End: end, RGB: rgb})
		}
		start = end
	}
	return runs
}
