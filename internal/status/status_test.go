package status

import (
	"strings"
	"testing"
	"time"
)

func TestParseBranchHeader(t *testing.T) {
	tests := []struct {
		name, line, want string
	}{
		{"tracking upstream", "## main...origin/main", "main"},
		{"ahead of upstream", "## main...origin/main [ahead 1]", "main"},
		{"gone upstream", "## feat/x...origin/feat/x [gone]", "feat/x"},
		{"no upstream", "## main", "main"},
		{"unborn branch", "## No commits yet on main", "main"},
		{"unborn with upstream", "## No commits yet on main...origin/main", "main"},
		{"detached head", "## HEAD (no branch)", ""},
		{"not a header", "?? file.go", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBranchHeader(tc.line); got != tc.want {
				t.Errorf("parseBranchHeader(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	out := strings.Join([]string{
		"## main...origin/main",
		"?? untracked.go",
		"A  added.go",
		" M modified.go",
		"MM staged-and-modified.go",
		"D  deleted.go",
		"",
	}, "\n")

	var r Repo
	parseStatus(out, &r)

	if r.Branch != "main" {
		t.Errorf("Branch = %q, want main", r.Branch)
	}
	if !r.Dirty {
		t.Error("Dirty = false, want true")
	}
	if r.Added != 2 { // untracked.go, added.go
		t.Errorf("Added = %d, want 2", r.Added)
	}
	if r.Modified != 3 { // modified, staged-and-modified, deleted
		t.Errorf("Modified = %d, want 3", r.Modified)
	}
}

func TestParseStatusCleanRepo(t *testing.T) {
	var r Repo
	parseStatus("## main\n", &r)
	if r.Dirty {
		t.Error("clean repo reported dirty")
	}
	if r.Added != 0 || r.Modified != 0 {
		t.Errorf("clean repo has counts: +%d ~%d", r.Added, r.Modified)
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		name, in string
		want     int
	}{
		{"plain ascii", "abc", 3},
		{"sgr color", "\033[2mabc\033[0m", 3},
		{"truecolor", rgb(1, 2, 3) + "hi" + Reset, 2},
		{"wide emoji", "💻", 2},
		{"emoji plus space", "💻 ", 3},
		{"window emoji U+1FA9F", "🪟", 2},
		{"variation selector is zero width", "⏱️", 1},
		{"osc8 hyperlink counts only text", termLink("#42", "https://example.com/very/long/url"), 3},
		{"colored hyperlink", termLink("\033[34m#42\033[0m", "https://x.y"), 3},
		{"box drawing", "│", 1},
		{"bar runes", "━━━───────", 10},
		{"reset glyph", "⟳", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DisplayWidth(tc.in); got != tc.want {
				t.Errorf("DisplayWidth(%q) = %d, want %d", stripANSI(tc.in), got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 10); got != "abcdef" {
		t.Errorf("no truncation expected, got %q", got)
	}
	got := truncate("abcdef", 4)
	if w := DisplayWidth(got); w > 4 {
		t.Errorf("truncate(%q, 4) width = %d, want <= 4", got, w)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("truncate should mark elision, got %q", got)
	}
	if truncate("abc", 0) != "" {
		t.Error("zero width should yield empty string")
	}
}

// Truncating colored text must not exceed the budget once escapes are stripped.
func TestTruncatePreservesWidthBudgetWithANSI(t *testing.T) {
	s := rgb(1, 2, 3) + "abcdefghij" + Reset
	got := truncate(s, 5)
	if w := DisplayWidth(got); w > 5 {
		t.Errorf("width = %d, want <= 5 (got %q)", w, stripANSI(got))
	}
}

func TestEpochUnmarshal(t *testing.T) {
	tests := []struct {
		name, json string
		wantUnix   int64
		wantSet    bool
	}{
		{"unix seconds", `1738425600`, 1738425600, true},
		{"unix float", `1738425600.5`, 1738425600, true},
		{"rfc3339", `"2025-02-01T16:00:00Z"`, 1738425600, true},
		{"quoted epoch", `"1738425600"`, 1738425600, true},
		{"null", `null`, 0, false},
		{"empty string", `""`, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e Epoch
			if err := e.UnmarshalJSON([]byte(tc.json)); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.json, err)
			}
			if e.Set != tc.wantSet {
				t.Fatalf("Set = %v, want %v", e.Set, tc.wantSet)
			}
			if tc.wantSet && e.Time.Unix() != tc.wantUnix {
				t.Errorf("Unix = %d, want %d", e.Time.Unix(), tc.wantUnix)
			}
		})
	}
}

func TestParseRealPayload(t *testing.T) {
	raw := []byte(`{
	  "cwd": "/tmp/proj",
	  "model": {"id": "claude-opus-4-8", "display_name": "Opus 4.8"},
	  "cost": {"total_cost_usd": 3.45, "total_duration_ms": 2520000},
	  "context_window": {"context_window_size": 200000, "used_percentage": 38,
	    "current_usage": {"input_tokens": 65000, "cache_creation_input_tokens": 8000,
	                      "cache_read_input_tokens": 53000}},
	  "effort": {"level": "high"},
	  "rate_limits": {"five_hour": {"used_percentage": 23.5, "resets_at": 1738425600}}
	}`)
	in, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if in.ContextTokens() != 126000 {
		t.Errorf("ContextTokens = %d, want 126000", in.ContextTokens())
	}
	if in.ContextPercent() != 38 {
		t.Errorf("ContextPercent = %d, want 38", in.ContextPercent())
	}
	if in.EffortLevel() != "high" {
		t.Errorf("EffortLevel = %q, want high", in.EffortLevel())
	}
	if in.RateLimits.SevenDay != nil {
		t.Error("absent seven_day should stay nil")
	}
	if pct, ok := in.CachePercent(); !ok || pct != 44 {
		t.Errorf("CachePercent = %d,%v; want 44,true", pct, ok)
	}
}

// current_usage is null before the first API call and again after /compact.
func TestNullCurrentUsage(t *testing.T) {
	in, err := Parse([]byte(`{"context_window": {"context_window_size": 200000, "current_usage": null}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if in.ContextTokens() != 0 {
		t.Errorf("ContextTokens = %d, want 0", in.ContextTokens())
	}
	if in.ContextPercent() != 0 {
		t.Errorf("ContextPercent = %d, want 0", in.ContextPercent())
	}
	if _, ok := in.CachePercent(); ok {
		t.Error("CachePercent should report absent")
	}
}

func TestContextPercentFallsBackToTokenCounts(t *testing.T) {
	in, err := Parse([]byte(`{"context_window": {"context_window_size": 100000,
	  "current_usage": {"input_tokens": 25000}}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := in.ContextPercent(); got != 25 {
		t.Errorf("ContextPercent = %d, want 25", got)
	}
}

func TestJoinFitDropsHighestPrioFirst(t *testing.T) {
	segs := []segment{
		{text: "keep", prio: 0},
		{text: "mid", prio: 3},
		{text: "drop", prio: 9},
	}
	if got := joinFit(segs, " | ", 100); !strings.Contains(got, "drop") {
		t.Errorf("expected all segments, got %q", got)
	}
	// "keep | mid" is 10 cells; "drop" must go first.
	got := joinFit(segs, " | ", 10)
	if strings.Contains(got, "drop") {
		t.Errorf("prio 9 should be dropped, got %q", got)
	}
	if !strings.Contains(got, "mid") || !strings.Contains(got, "keep") {
		t.Errorf("lower prio segments should survive, got %q", got)
	}
	// Only "keep" (4 cells) fits.
	if got := joinFit(segs, " | ", 4); strings.Contains(got, "mid") {
		t.Errorf("prio 3 should be dropped, got %q", got)
	}
}

func testOptions() Options {
	theme, _ := LookupTheme("plain")
	bar, _ := LookupBar("line")
	return Options{
		Theme:    theme,
		Bar:      bar,
		BarWidth: 10,
		Hostname: "testhost",
		Home:     "/home/u",
		Now:      time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
	}
}

func testInput() Input {
	pct := 38.0
	return Input{
		Cwd:   "/home/u/proj",
		Model: Model{DisplayName: "Opus 4.8"},
		Cost:  Cost{TotalCostUSD: 3.45, TotalDurationMs: 2_520_000},
		ContextWindow: ContextWindow{
			ContextWindowSize: 200_000,
			UsedPercentage:    &pct,
			CurrentUsage:      &CurrentUsage{InputTokens: 65_000, CacheReadInputTokens: 53_000},
		},
		Effort: &Effort{Level: "high"},
		RateLimits: &RateLimits{
			FiveHour: &Window{UsedPercentage: 30, ResetsAt: Epoch{Time: time.Unix(1783000000, 0), Set: true}},
			SevenDay: &Window{UsedPercentage: 62, ResetsAt: Epoch{Time: time.Unix(1783400000, 0), Set: true}},
		},
	}
}

// The whole point of the compact layout: current and weekly share one row.
func TestCompactPutsRateWindowsOnOneRow(t *testing.T) {
	o := testOptions()
	o.Layout = LayoutCompact
	o.Width = 200

	rows := strings.Split(Render(testInput(), o), "\n")
	if len(rows) != 3 {
		t.Fatalf("compact layout = %d rows, want 3:\n%s", len(rows), strings.Join(rows, "\n"))
	}
	rate := rows[2]
	if !strings.Contains(rate, "current") || !strings.Contains(rate, "weekly") {
		t.Errorf("rate row missing a window: %q", rate)
	}
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			t.Error("compact layout must not emit blank rows")
		}
	}
}

func TestFullLayoutStacksRateWindows(t *testing.T) {
	o := testOptions()
	o.Layout = LayoutFull
	o.Width = 200

	out := Render(testInput(), o)
	rows := strings.Split(out, "\n")
	if len(rows) != 6 { // header, blank, workspace, blank, current, weekly
		t.Fatalf("full layout = %d rows, want 6:\n%s", len(rows), out)
	}
	if !strings.HasPrefix(rows[4], "current") {
		t.Errorf("row 4 = %q, want current window", rows[4])
	}
	if !strings.HasPrefix(rows[5], "weekly") {
		t.Errorf("row 5 = %q, want weekly window", rows[5])
	}
}

func TestUltraLayoutIsTwoRows(t *testing.T) {
	o := testOptions()
	o.Layout = LayoutUltra
	o.Width = 200

	rows := strings.Split(Render(testInput(), o), "\n")
	if len(rows) != 2 {
		t.Fatalf("ultra layout = %d rows, want 2:\n%s", len(rows), strings.Join(rows, "\n"))
	}
	if !strings.Contains(rows[1], "current") || !strings.Contains(rows[1], "weekly") {
		t.Errorf("ultra row 2 should carry both windows: %q", rows[1])
	}
}

// Every row must fit the terminal, otherwise wrapping undoes the space saving.
func TestRowsNeverExceedTerminalWidth(t *testing.T) {
	for _, width := range []int{200, 120, 100, 80, 60, 40, 30} {
		for _, layout := range []Layout{LayoutCompact, LayoutUltra} {
			o := testOptions()
			o.Layout = layout
			o.Width = width

			for i, row := range strings.Split(Render(testInput(), o), "\n") {
				if got := DisplayWidth(row); got > width {
					t.Errorf("layout=%v width=%d row %d overflows: %d cells (%q)",
						layout, width, i, got, stripANSI(row))
				}
			}
		}
	}
}

// The rate row degrades by shrinking bars, then dropping reset times and long
// labels, rather than by dropping a whole window. Both percentages survive at
// every width down to the point where the row can no longer hold them.
func TestRateRowKeepsBothWindowsWhenNarrow(t *testing.T) {
	o := testOptions()
	for _, width := range []int{80, 60, 50, 42, 34, 28, 20} {
		row := o.rateRow(testInput(), width)
		plain := stripANSI(row)

		if w := DisplayWidth(row); w > width {
			t.Errorf("width=%d: rate row is %d cells (%q)", width, w, plain)
		}
		if !strings.Contains(plain, "30%") || !strings.Contains(plain, "62%") {
			t.Errorf("width=%d: dropped a window: %q", width, plain)
		}
		if n := strings.Count(plain, "│"); n != 1 {
			t.Errorf("width=%d: want exactly 1 separator, got %d (%q)", width, n, plain)
		}
	}
}

// Below the last degradation level the row is truncated rather than wrapped.
func TestRateRowTruncatesWhenImpossiblyNarrow(t *testing.T) {
	o := testOptions()
	row := o.rateRow(testInput(), 10)
	if w := DisplayWidth(row); w > 10 {
		t.Errorf("rate row width = %d, want <= 10 (%q)", w, stripANSI(row))
	}
}

// Short labels appear only once the long ones stop fitting.
func TestRateRowUsesShortLabelsOnlyWhenForced(t *testing.T) {
	o := testOptions()
	if plain := stripANSI(o.rateRow(testInput(), 80)); !strings.Contains(plain, "current") {
		t.Errorf("wide row should use long labels: %q", plain)
	}
	if plain := stripANSI(o.rateRow(testInput(), 20)); !strings.Contains(plain, "5h") {
		t.Errorf("narrow row should use short labels: %q", plain)
	}
}

func TestRateRowEmptyWithoutRateLimits(t *testing.T) {
	o := testOptions()
	in := testInput()
	in.RateLimits = nil
	if got := o.rateRow(in, 100); got != "" {
		t.Errorf("rateRow without limits = %q, want empty", got)
	}
	o.Layout, o.Width = LayoutCompact, 200
	if rows := strings.Split(Render(in, o), "\n"); len(rows) != 2 {
		t.Errorf("compact without rate limits = %d rows, want 2", len(rows))
	}
}

func TestBurnRate(t *testing.T) {
	x := ExtraUsage{Enabled: true, UsedCredits: 450, MonthlyLimit: 2000}

	if _, _, ok := x.BurnRate(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Error("day 1 should not project")
	}

	daily, projected, ok := x.BurnRate(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("day 10 should project")
	}
	if daily != 45 {
		t.Errorf("daily = %v cents, want 45", daily)
	}
	if projected != 45*31 { // March has 31 days
		t.Errorf("projected = %v cents, want %v", projected, 45*31)
	}
}

func TestBurnRateFebruary(t *testing.T) {
	x := ExtraUsage{Enabled: true, UsedCredits: 280, MonthlyLimit: 2000}
	daily, projected, ok := x.BurnRate(time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected projection")
	}
	if daily != 20 {
		t.Errorf("daily = %v, want 20", daily)
	}
	if projected != 20*28 { // 2026 is not a leap year
		t.Errorf("projected = %v, want %v", projected, 20*28)
	}
}

func TestFirstOfNextMonthWrapsYear(t *testing.T) {
	got := firstOfNextMonth(time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC))
	want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("firstOfNextMonth = %v, want %v", got, want)
	}
}

func TestFormatTokens(t *testing.T) {
	tests := map[int]string{0: "0", 999: "999", 1000: "1.0k", 126_000: "126.0k", 1_500_000: "1.5m"}
	for in, want := range tests {
		if got := formatTokens(in); got != want {
			t.Errorf("formatTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := map[time.Duration]string{
		45 * time.Second:               "45s",
		3*time.Minute + 18*time.Second: "3m18s",
		2*time.Hour + 5*time.Minute:    "2h5m",
		2_520_000 * time.Millisecond:   "42m0s",
	}
	for in, want := range tests {
		if got := formatDuration(in); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatClock(t *testing.T) {
	tests := map[time.Time]string{
		time.Date(2026, 3, 15, 0, 5, 0, 0, time.UTC):   "12:05am",
		time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC):  "12:00pm",
		time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC): "2:30pm",
		time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC):   "9:00am",
	}
	for in, want := range tests {
		if got := formatClock(in); got != want {
			t.Errorf("formatClock(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestBarFillIsProportional(t *testing.T) {
	theme, _ := LookupTheme("plain")
	style, _ := LookupBar("line")
	for _, tc := range []struct{ pct, filled int }{{0, 0}, {50, 5}, {100, 10}, {-5, 0}, {150, 10}} {
		got := stripANSI(theme.bar(tc.pct, 10, style))
		if n := strings.Count(got, style.Filled); n != tc.filled {
			t.Errorf("bar(%d) filled = %d, want %d", tc.pct, n, tc.filled)
		}
		if DisplayWidth(got) != 10 {
			t.Errorf("bar(%d) width = %d, want 10", tc.pct, DisplayWidth(got))
		}
	}
}

func TestShortenHome(t *testing.T) {
	if got := shortenHome("/home/u/proj", "/home/u"); got != "~/proj" {
		t.Errorf("shortenHome = %q, want ~/proj", got)
	}
	if got := shortenHome("/var/log", "/home/u"); got != "/var/log" {
		t.Errorf("shortenHome = %q, want /var/log", got)
	}
	if got := shortenHome("/home/u", ""); got != "/home/u" {
		t.Errorf("empty home should pass through, got %q", got)
	}
}

func TestPlainThemeEmitsNoColor(t *testing.T) {
	o := testOptions()
	o.Layout, o.Width = LayoutCompact, 200
	out := Render(testInput(), o)
	// Dim and Reset still provide structure; no truecolor SGR should appear.
	if strings.Contains(out, "\033[38;2;") {
		t.Error("plain theme emitted truecolor sequences")
	}
}
