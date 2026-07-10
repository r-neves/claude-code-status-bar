package status

import (
	"os"
	"strconv"
	"strings"
)

// stripANSI removes SGR color codes and OSC 8 hyperlink wrappers so the
// remaining runes correspond one-to-one with printed cells.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case '[': // CSI, terminated by a byte in @-~
			i += 2
			for i < len(s) && (s[i] < '@' || s[i] > '~') {
				i++
			}
			i++
		case ']': // OSC, terminated by BEL or ST (ESC backslash)
			i += 2
			for i < len(s) {
				if s[i] == 0x07 {
					i++
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default:
			i += 2
		}
	}
	return b.String()
}

// runeWidth returns how many terminal cells a rune occupies. It covers the
// zero-width and wide characters this program actually emits rather than
// carrying the full Unicode width tables.
func runeWidth(r rune) int {
	switch {
	case r == 0x200d, r == 0xfe0f, r == 0xfe0e: // ZWJ, variation selectors
		return 0
	case r < 0x20: // control characters
		return 0
	case r >= 0x0300 && r <= 0x036f: // combining diacriticals
		return 0
	case r >= 0x1100 && r <= 0x115f, // Hangul Jamo
		r >= 0x2e80 && r <= 0x303e,
		r >= 0x3041 && r <= 0x33ff,
		r >= 0x3400 && r <= 0x4dbf,
		r >= 0x4e00 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe6f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f9ff, // emoji and pictographs
		r >= 0x1fa00 && r <= 0x1faff, // extended pictographs, includes U+1FA9F
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	default:
		// Misc symbols such as U+27F3 render single-width in target terminals.
		return 1
	}
}

// DisplayWidth is the printed cell width of s, ignoring escape sequences.
func DisplayWidth(s string) int {
	w := 0
	for _, r := range stripANSI(s) {
		w += runeWidth(r)
	}
	return w
}

// DefaultWidth is assumed when the real terminal width cannot be determined.
const DefaultWidth = 120

// TerminalWidth reports the usable column count. Claude Code pipes both stdin
// and stdout, so the ioctl is issued against /dev/tty. COLUMNS takes priority,
// which also makes the width injectable from tests and scripts.
func TerminalWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if n, ok := ttyWidth(); ok && n > 0 {
		return n
	}
	return DefaultWidth
}

// truncate shortens s to at most max cells, appending an ellipsis when it cuts.
// Escape sequences are copied through without consuming width.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if DisplayWidth(s) <= max {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	w := 0
	for i := 0; i < len(runes); i++ {
		if runes[i] == 0x1b {
			start := i
			i++
			if i < len(runes) && runes[i] == '[' {
				i++
				for i < len(runes) && (runes[i] < '@' || runes[i] > '~') {
					i++
				}
			} else if i < len(runes) && runes[i] == ']' {
				for i < len(runes) && runes[i] != 0x07 {
					i++
				}
			}
			if i >= len(runes) {
				i = len(runes) - 1
			}
			b.WriteString(string(runes[start : i+1]))
			continue
		}
		rw := runeWidth(runes[i])
		if w+rw > max-1 {
			break
		}
		b.WriteRune(runes[i])
		w += rw
	}
	b.WriteString("…" + Reset)
	return b.String()
}
