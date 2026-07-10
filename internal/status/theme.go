package status

import (
	"fmt"
	"sort"
	"strings"
)

// ANSI control sequences.
const (
	Dim   = "\033[2m"
	Reset = "\033[0m"
)

func rgb(r, g, b int) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

// Theme is the palette used to colorize every segment.
type Theme struct {
	Blue, Orange, Green, Cyan, Red, Yellow, White, Magenta string
}

var themes = map[string]Theme{
	"default": {
		Blue:    rgb(0, 153, 255),
		Orange:  rgb(255, 176, 85),
		Green:   rgb(0, 175, 80),
		Cyan:    rgb(86, 182, 194),
		Red:     rgb(255, 85, 85),
		Yellow:  rgb(230, 200, 0),
		White:   rgb(220, 220, 220),
		Magenta: rgb(180, 140, 255),
	},
	"dracula": {
		Blue:    rgb(189, 147, 249),
		Orange:  rgb(255, 184, 108),
		Green:   rgb(80, 250, 123),
		Cyan:    rgb(139, 233, 253),
		Red:     rgb(255, 85, 85),
		Yellow:  rgb(241, 250, 140),
		White:   rgb(248, 248, 242),
		Magenta: rgb(255, 121, 198),
	},
	"catppuccin": {
		Blue:    rgb(137, 180, 250),
		Orange:  rgb(250, 179, 135),
		Green:   rgb(166, 227, 161),
		Cyan:    rgb(148, 226, 213),
		Red:     rgb(243, 139, 168),
		Yellow:  rgb(249, 226, 175),
		White:   rgb(205, 214, 244),
		Magenta: rgb(203, 166, 247),
	},
	"solarized": {
		Blue:    rgb(38, 139, 210),
		Orange:  rgb(203, 75, 22),
		Green:   rgb(133, 153, 0),
		Cyan:    rgb(42, 161, 152),
		Red:     rgb(220, 50, 47),
		Yellow:  rgb(181, 137, 0),
		White:   rgb(147, 161, 161),
		Magenta: rgb(108, 113, 196),
	},
	"mono": {
		Blue:    rgb(170, 170, 170),
		Orange:  rgb(153, 153, 153),
		Green:   rgb(204, 204, 204),
		Cyan:    rgb(187, 187, 187),
		Red:     rgb(136, 136, 136),
		Yellow:  rgb(153, 153, 153),
		White:   rgb(204, 204, 204),
		Magenta: rgb(170, 170, 170),
	},
	// plain leaves every color empty, for NO_COLOR or terminals without truecolor.
	"plain": {},
}

// LookupTheme resolves a theme by name, reporting whether it exists.
func LookupTheme(name string) (Theme, bool) {
	t, ok := themes[name]
	return t, ok
}

// ThemeNames returns every theme name, sorted.
func ThemeNames() []string { return sortedKeys(themes) }

// BarStyle describes the runes used to draw a progress bar.
type BarStyle struct {
	Filled string
	Empty  string
}

var barStyles = map[string]BarStyle{
	"line":      {Filled: "━", Empty: "─"},
	"classic":   {Filled: "●", Empty: "○"},
	"block":     {Filled: "█", Empty: "░"},
	"braille":   {Filled: "⣿", Empty: "⠄"},
	"geometric": {Filled: "▰", Empty: "▱"},
}

// LookupBar resolves a bar style by name, reporting whether it exists.
func LookupBar(name string) (BarStyle, bool) {
	b, ok := barStyles[name]
	return b, ok
}

// BarNames returns every bar style name, sorted.
func BarNames() []string { return sortedKeys(barStyles) }

func sortedKeys[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// colorFor maps a 0-100 utilization onto a green-to-red gradient.
func (t Theme) colorFor(pct int) string {
	switch {
	case pct >= 90:
		return t.Red
	case pct >= 70:
		return t.Yellow
	case pct >= 50:
		return t.Orange
	default:
		return t.Green
	}
}

// bar renders a width-cell progress bar filled to pct percent.
func (t Theme) bar(pct, width int, style BarStyle) string {
	pct = clamp(pct, 0, 100)
	filled := pct * width / 100
	return t.colorFor(pct) + strings.Repeat(style.Filled, filled) +
		Dim + strings.Repeat(style.Empty, width-filled) + Reset
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
