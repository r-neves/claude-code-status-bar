package status

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Layout selects how many rows the statusline occupies.
type Layout int

const (
	// LayoutCompact prints three rows: header, workspace, rate limits. The
	// rate-limit windows sit side by side on one row.
	LayoutCompact Layout = iota
	// LayoutUltra prints two rows, folding the rate limits in beside the
	// working directory and dropping the lowest-value segments.
	LayoutUltra
	// LayoutFull reproduces the stacked, blank-line-separated arrangement of
	// the original claude-status-go.
	LayoutFull
)

// ParseLayout resolves a layout by name.
func ParseLayout(s string) (Layout, bool) {
	switch s {
	case "compact":
		return LayoutCompact, true
	case "ultra":
		return LayoutUltra, true
	case "full":
		return LayoutFull, true
	}
	return 0, false
}

// LayoutNames returns every layout name, sorted.
func LayoutNames() []string { return []string{"compact", "full", "ultra"} }

// Options carries everything Render needs beyond the parsed stdin payload.
type Options struct {
	Theme    Theme
	Bar      BarStyle
	Layout   Layout
	Width    int // terminal columns; <= 0 disables fitting
	BarWidth int
	Hostname string
	Home     string
	Repo     Repo
	Extra    *ExtraUsage // nil unless --extra resolved usage credits
	Now      time.Time
}

// segment is one unit of the status bar. Segments with a higher prio are
// dropped first when a row does not fit; prio 0 is never dropped.
type segment struct {
	text string
	prio int
}

// Render builds the complete status bar, newline separated and without a
// trailing newline.
func Render(in Input, o Options) string {
	if o.BarWidth <= 0 {
		o.BarWidth = 10
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}

	switch o.Layout {
	case LayoutFull:
		return o.renderFull(in)
	case LayoutUltra:
		return o.renderUltra(in)
	default:
		return o.renderCompact(in)
	}
}

func (o Options) sep() string { return " " + Dim + "│" + Reset + " " }

// ---- Rows -----------------------------------------------------------------

// header is the first row: host, model, context, cache, duration, cost, effort.
func (o Options) header(in Input) []segment {
	t := o.Theme
	segs := []segment{
		{text: "💻 " + t.Magenta + o.Hostname + Reset, prio: 5},
		{text: t.Blue + modelName(in) + Reset, prio: 0},
		{text: o.contextSeg(in), prio: 0},
	}
	if pct, ok := in.CachePercent(); ok {
		segs = append(segs, segment{text: o.cacheSeg(pct), prio: 4})
	}
	if d := time.Duration(in.Cost.TotalDurationMs) * time.Millisecond; d > 0 {
		segs = append(segs, segment{
			text: Dim + "⏱ " + Reset + t.White + formatDuration(d) + Reset,
			prio: 3,
		})
	}
	if c := in.Cost.TotalCostUSD; c > 0 {
		segs = append(segs, segment{text: o.costSeg(c), prio: 2})
	}
	if e := in.EffortLevel(); e != "" {
		segs = append(segs, segment{text: o.effortSeg(e), prio: 3})
	}
	return segs
}

// workspace is the second row: directory, git, PR, and session statistics.
func (o Options) workspace(in Input) []segment {
	t := o.Theme
	segs := []segment{{text: o.dirSeg(in), prio: 0}}

	if c := o.Repo.Commit; c != "" {
		segs = append(segs, segment{text: Dim + c + Reset, prio: 6})
	}
	if in.PR != nil && in.PR.Number > 0 {
		segs = append(segs, segment{text: o.prSeg(in.PR), prio: 2})
	}
	if a := in.Agent; a != nil && a.Name != "" {
		segs = append(segs, segment{text: "🤖 " + t.Orange + a.Name + Reset, prio: 1})
	}
	if in.Cost.TotalLinesAdded > 0 || in.Cost.TotalLinesRemoved > 0 {
		segs = append(segs, segment{
			text: "📊 " + t.Green + fmt.Sprintf("+%d", in.Cost.TotalLinesAdded) + Reset +
				" " + t.Red + fmt.Sprintf("-%d", in.Cost.TotalLinesRemoved) + Reset +
				Dim + " lines" + Reset,
			prio: 3,
		})
	}
	if n := in.ContextWindow.TotalOutputTokens; n > 0 {
		segs = append(segs, segment{
			text: t.White + formatTokens(n) + Reset + Dim + " out" + Reset,
			prio: 5,
		})
	}
	if d := time.Duration(in.Cost.TotalAPIDurationMs) * time.Millisecond; d > 0 {
		segs = append(segs, segment{
			text: t.White + formatDuration(d) + Reset + Dim + " api" + Reset,
			prio: 6,
		})
	}
	return segs
}

// ---- Layouts --------------------------------------------------------------

func (o Options) renderCompact(in Input) string {
	rows := []string{
		joinFit(o.header(in), o.sep(), o.Width),
		joinFit(o.workspace(in), o.sep(), o.Width),
	}
	if r := o.rateRow(in, o.Width); r != "" {
		rows = append(rows, r)
	}
	return strings.Join(rows, "\n")
}

// Floors for the shared second row of the ultra layout: neither the directory
// nor the rate windows may be squeezed below a legible size.
const (
	minDirWidth  = 12
	minRateWidth = 15
)

// renderUltra folds the rate limits onto the workspace row. The rate windows
// are rendered first, against a budget that reserves room for the directory,
// so that a deep path and a wide rate row cannot together overflow the line.
func (o Options) renderUltra(in Input) string {
	// Host and the per-session token and api counters are the least useful
	// segments when space is this tight.
	head := dropPrio(o.header(in), 5)
	rows := []string{joinFit(head, o.sep(), o.Width)}

	sepW := DisplayWidth(o.sep())

	rateBudget := 0 // 0 leaves rateRow at full detail
	if o.Width > 0 {
		rateBudget = max(o.Width-minDirWidth-sepW, minRateWidth)
	}
	rate := o.rateRow(in, rateBudget)

	dirBudget := o.Width
	if o.Width > 0 && rate != "" {
		dirBudget = o.Width - DisplayWidth(rate) - sepW
	}
	// The rate windows are the reason to use this layout, so when the terminal
	// is too narrow for both, the directory is what gives way.
	if o.Width > 0 && dirBudget <= 0 {
		return strings.Join(append(rows, truncate(rate, o.Width)), "\n")
	}

	second := joinFit([]segment{{text: o.dirSeg(in), prio: 0}}, o.sep(), dirBudget)
	if rate != "" {
		second += o.sep() + rate
	}
	return strings.Join(append(rows, second), "\n")
}

// renderFull reproduces the original stacked arrangement, with each rate window
// on its own row after a blank line.
func (o Options) renderFull(in Input) string {
	var b strings.Builder
	b.WriteString(joinFit(o.header(in), o.sep(), o.Width))
	b.WriteString("\n\n")
	b.WriteString(joinFit(o.workspace(in), o.sep(), o.Width))

	items := o.rateItems(in)
	if len(items) == 0 {
		return b.String()
	}
	b.WriteString("\n\n")

	labelW := 0
	for _, it := range items {
		labelW = max(labelW, len(it.label))
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		label := it.label + strings.Repeat(" ", labelW-len(it.label))
		line := o.Theme.White + label + Reset + " " +
			o.Theme.bar(it.pct, o.BarWidth, o.Bar) + " " + it.value
		if it.hasReset {
			line += " " + Dim + "⟳" + Reset + "  " + o.Theme.White + it.resetLong + Reset
		}
		if it.note != "" {
			line += "  " + it.note
		}
		lines = append(lines, line)
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}

// ---- Rate limits ----------------------------------------------------------

// rateItem is one usage window, pre-formatted at both detail levels.
type rateItem struct {
	label      string
	short      string
	pct        int
	value      string // "30%" or "$4.50/$20.00"
	shortValue string
	resetShort string
	resetLong  string
	hasReset   bool
	note       string // burn-rate annotation, extra usage only
}

func (o Options) rateItems(in Input) []rateItem {
	t := o.Theme
	var items []rateItem

	addWindow := func(label, short string, w *Window, shortFmt, longFmt func(time.Time) string) {
		pct := roundPct(w.UsedPercentage)
		it := rateItem{
			label:      label,
			short:      short,
			pct:        pct,
			value:      t.colorFor(pct) + fmt.Sprintf("%3d%%", pct) + Reset,
			shortValue: t.colorFor(pct) + fmt.Sprintf("%d%%", pct) + Reset,
			hasReset:   w.ResetsAt.Set,
		}
		if w.ResetsAt.Set {
			local := w.ResetsAt.Time.Local()
			it.resetShort = shortFmt(local)
			it.resetLong = longFmt(local)
		}
		items = append(items, it)
	}

	if rl := in.RateLimits; rl != nil {
		// The 5-hour window resets within the day, so a clock time is what
		// matters. The 7-day window resets days out, so lead with the date.
		if w := rl.FiveHour; w != nil {
			addWindow("current", "5h", w, formatClock, formatClock)
		}
		if w := rl.SevenDay; w != nil {
			addWindow("weekly", "7d", w, formatDay, func(t time.Time) string {
				return formatDay(t) + ", " + formatClock(t)
			})
		}
	}

	if x := o.Extra; x != nil && x.Enabled {
		pct := roundPct(x.Utilization)
		reset := formatDay(firstOfNextMonth(o.Now))
		it := rateItem{
			label:      "extra",
			short:      "extra",
			pct:        pct,
			hasReset:   true,
			resetShort: reset,
			resetLong:  reset,
			value: t.colorFor(pct) + fmt.Sprintf("$%.2f", x.UsedCredits/100) +
				Dim + "/" + Reset + t.White + fmt.Sprintf("$%.2f", x.MonthlyLimit/100) + Reset,
			shortValue: t.colorFor(pct) + fmt.Sprintf("$%.0f", x.UsedCredits/100) +
				Dim + "/" + Reset + t.White + fmt.Sprintf("$%.0f", x.MonthlyLimit/100) + Reset,
		}
		if daily, projected, ok := x.BurnRate(o.Now); ok && x.MonthlyLimit > 0 {
			color := t.Green
			switch ratio := projected / x.MonthlyLimit * 100; {
			case ratio >= 100:
				color = t.Red
			case ratio >= 80:
				color = t.Yellow
			}
			it.note = fmt.Sprintf("📈 %s$%.2f/day%s → %s$%.2f proj%s",
				Dim, daily/100, Reset, color, projected/100, Reset)
		}
		items = append(items, it)
	}
	return items
}

// rateRow renders every usage window on a single row, shrinking the bars and
// then dropping the reset times and long labels until the row fits in width.
// A width of 0 disables fitting and renders at full detail.
func (o Options) rateRow(in Input, width int) string {
	items := o.rateItems(in)
	if len(items) == 0 {
		return ""
	}

	// Progressively cheaper renderings, tried in order.
	levels := []struct {
		barWidth  int
		showReset bool
		showNote  bool
		shortName bool
		shortVal  bool
	}{
		{o.BarWidth, true, true, false, false},
		{o.BarWidth, true, false, false, false},
		{o.BarWidth - 2, true, false, false, false},
		{o.BarWidth - 4, false, false, false, false},
		{5, false, false, true, true},
		{0, false, false, true, true},
	}

	var out string
	for _, lv := range levels {
		parts := make([]string, 0, len(items))
		for _, it := range items {
			label, value := it.label, it.value
			if lv.shortName {
				label = it.short
			}
			if lv.shortVal {
				value = it.shortValue
			}

			p := o.Theme.White + label + Reset
			if lv.barWidth > 0 {
				p += " " + o.Theme.bar(it.pct, lv.barWidth, o.Bar)
			}
			p += " " + value
			if lv.showReset && it.hasReset {
				p += " " + Dim + "⟳" + Reset + " " + o.Theme.White + it.resetShort + Reset
			}
			if lv.showNote && it.note != "" {
				p += "  " + it.note
			}
			parts = append(parts, p)
		}
		out = strings.Join(parts, o.sep())
		if width <= 0 || DisplayWidth(out) <= width {
			return out
		}
	}
	return truncate(out, width)
}

// ---- Segment builders -----------------------------------------------------

func (o Options) contextSeg(in Input) string {
	t := o.Theme
	pct := in.ContextPercent()
	return "🪟 " + t.White + formatTokens(in.ContextTokens()) + Dim + "/" + Reset +
		t.White + formatTokens(in.ContextSize()) + Reset + " " +
		t.colorFor(pct) + fmt.Sprintf("(%d%%)", pct) + Reset
}

func (o Options) cacheSeg(pct int) string {
	color := Dim
	switch {
	case pct >= 70:
		color = o.Theme.Green
	case pct >= 40:
		color = o.Theme.Yellow
	}
	return color + fmt.Sprintf("💾 %d%% cache", pct) + Reset
}

func (o Options) costSeg(usd float64) string {
	t := o.Theme
	color := t.Green
	switch {
	case usd >= 10:
		color = t.Red
	case usd >= 5:
		color = t.Yellow
	case usd >= 2:
		color = t.Orange
	}
	return color + fmt.Sprintf("💰$%.2f", usd) + Reset
}

func (o Options) effortSeg(level string) string {
	t := o.Theme
	switch level {
	case "high", "xhigh", "max":
		return t.Magenta + "● " + level + Reset
	case "low":
		return Dim + "◔ " + level + Reset
	default:
		return Dim + "◑ " + level + Reset
	}
}

func (o Options) dirSeg(in Input) string {
	t := o.Theme
	s := "📂 " + t.Cyan + shortenHome(in.Dir(), o.Home) + Reset

	r := o.Repo
	if r.Branch == "" {
		return s
	}
	inner := r.Branch
	if r.Dirty {
		inner += t.Red + "*" + t.Green
	}
	if r.Added > 0 {
		inner += " " + t.Green + fmt.Sprintf("+%d", r.Added) + t.Green
	}
	if r.Modified > 0 {
		inner += " " + t.Yellow + fmt.Sprintf("~%d", r.Modified) + t.Green
	}
	return s + " " + t.Green + "(" + inner + ")" + Reset
}

func (o Options) prSeg(pr *PR) string {
	color := o.Theme.Blue
	if pr.ReviewState == "approved" {
		color = o.Theme.Green
	}
	text := color + fmt.Sprintf("#%d", pr.Number) + Reset
	if pr.URL != "" {
		return termLink(text, pr.URL)
	}
	return text
}

// ---- Helpers --------------------------------------------------------------

// joinFit renders segments separated by sep, dropping the highest-prio
// segments until the row fits in width. A width of 0 disables fitting.
func joinFit(segs []segment, sep string, width int) string {
	live := make([]bool, len(segs))
	for i := range segs {
		live[i] = segs[i].text != ""
	}
	for {
		out := assemble(segs, live, sep)
		if width <= 0 || DisplayWidth(out) <= width {
			return out
		}
		worst, idx := 0, -1
		for i := range segs {
			if live[i] && segs[i].prio > worst {
				worst, idx = segs[i].prio, i
			}
		}
		if idx < 0 {
			return truncate(out, width)
		}
		live[idx] = false
	}
}

func assemble(segs []segment, live []bool, sep string) string {
	parts := make([]string, 0, len(segs))
	for i, s := range segs {
		if live[i] {
			parts = append(parts, s.text)
		}
	}
	return strings.Join(parts, sep)
}

// dropPrio removes every segment whose prio is at least min.
func dropPrio(segs []segment, min int) []segment {
	out := segs[:0:0]
	for _, s := range segs {
		if s.prio < min {
			out = append(out, s)
		}
	}
	return out
}

func modelName(in Input) string {
	if in.Model.DisplayName != "" {
		return in.Model.DisplayName
	}
	return "Claude"
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// formatClock renders a 12-hour clock time, e.g. "2:30pm".
func formatClock(t time.Time) string {
	h := t.Hour() % 12
	if h == 0 {
		h = 12
	}
	suffix := "am"
	if t.Hour() >= 12 {
		suffix = "pm"
	}
	return fmt.Sprintf("%d:%02d%s", h, t.Minute(), suffix)
}

// formatDay renders a short date, e.g. "mar 18".
func formatDay(t time.Time) string {
	return fmt.Sprintf("%s %d", strings.ToLower(t.Format("Jan")), t.Day())
}

func firstOfNextMonth(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
}

func roundPct(f float64) int { return clamp(int(math.Round(f)), 0, 100) }

func shortenHome(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// termLink wraps text in an OSC 8 hyperlink. Terminals without support print
// the text unchanged.
func termLink(text, url string) string {
	return "\033]8;;" + url + "\033\\" + text + "\033]8;;\033\\"
}
