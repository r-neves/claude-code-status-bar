// Command claude-code-status-bar renders a compact, single-binary statusline
// for Claude Code. It reads the session JSON on stdin and writes ANSI text.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/r-neves/claude-code-status-bar/internal/status"
)

// version is overwritten at link time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		// Never fail loudly: a broken statusline must not disrupt the UI.
		fmt.Print("Claude")
		fmt.Fprintln(os.Stderr, "claude-code-status-bar:", err)
	}
}

func run() error {
	fs := flag.NewFlagSet("claude-code-status-bar", flag.ContinueOnError)
	var (
		themeName  = fs.String("theme", env("CLAUDE_STATUSLINE_THEME", "default"), "color theme")
		barName    = fs.String("bar", env("CLAUDE_STATUSLINE_BAR", "line"), "progress bar style")
		layoutName = fs.String("layout", env("CLAUDE_STATUSLINE_LAYOUT", "compact"), "row layout: compact, ultra, full")
		barWidth   = fs.Int("bar-width", 10, "progress bar width in cells")
		width      = fs.Int("width", 0, "terminal width override; 0 autodetects")
		extra      = fs.Bool("extra", false, "fetch pay-as-you-go credit usage from the OAuth API")
		showVer    = fs.Bool("version", false, "print version and exit")
		listThemes = fs.Bool("themes", false, "list themes and exit")
		listBars   = fs.Bool("bars", false, "list bar styles and exit")
	)
	// Consumed by dumpPath, which runs before flag values are needed.
	fs.String("dump", "", "write the raw stdin JSON to this path for debugging")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: claude-code-status-bar [flags] < session.json\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	switch {
	case *showVer:
		fmt.Printf("claude-code-status-bar %s\n", version)
		return nil
	case *listThemes:
		fmt.Println(strings.Join(status.ThemeNames(), ", "))
		return nil
	case *listBars:
		fmt.Println(strings.Join(status.BarNames(), ", "))
		return nil
	}

	theme, ok := status.LookupTheme(*themeName)
	if !ok {
		return fmt.Errorf("unknown theme %q (have: %s)", *themeName, strings.Join(status.ThemeNames(), ", "))
	}
	if os.Getenv("NO_COLOR") != "" {
		theme, _ = status.LookupTheme("plain")
	}
	bar, ok := status.LookupBar(*barName)
	if !ok {
		return fmt.Errorf("unknown bar style %q (have: %s)", *barName, strings.Join(status.BarNames(), ", "))
	}
	layout, ok := status.ParseLayout(*layoutName)
	if !ok {
		return fmt.Errorf("unknown layout %q (have: %s)", *layoutName, strings.Join(status.LayoutNames(), ", "))
	}

	in, err := readInput()
	if err != nil {
		return err
	}

	ctx := context.Background()
	opts := status.Options{
		Theme:    theme,
		Bar:      bar,
		Layout:   layout,
		BarWidth: *barWidth,
		Width:    resolveWidth(*width),
		Hostname: shortHostname(),
		Repo:     status.RepoInfo(ctx, in.Dir()),
		Now:      time.Now(),
	}
	if home, err := os.UserHomeDir(); err == nil {
		opts.Home = home
	}
	if *extra {
		opts.Extra = status.FetchExtraUsage(ctx)
	}

	fmt.Print(status.Render(in, opts))
	return nil
}

// readInput decodes the session JSON from stdin. When stdin is a terminal there
// is no Claude Code to pipe data, so a representative sample is rendered
// instead, which makes tweaking themes and layouts possible from a shell.
func readInput() (status.Input, error) {
	info, err := os.Stdin.Stat()
	if err == nil && info.Mode()&os.ModeCharDevice != 0 {
		return demoInput(), nil
	}

	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	if err != nil {
		return status.Input{}, fmt.Errorf("read stdin: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return status.Input{}, fmt.Errorf("empty stdin")
	}
	if path := dumpPath(); path != "" {
		_ = os.WriteFile(path, raw, 0o600)
	}
	in, err := status.Parse(raw)
	if err != nil {
		return status.Input{}, fmt.Errorf("parse stdin json: %w", err)
	}
	return in, nil
}

// dumpPath extracts --dump from the raw arguments, since readInput consumes
// stdin before parsed flag values are threaded through.
func dumpPath() string {
	for i, a := range os.Args {
		if a == "--dump" || a == "-dump" {
			if i+1 < len(os.Args) {
				return os.Args[i+1]
			}
		}
		for _, p := range []string{"--dump=", "-dump="} {
			if rest, ok := strings.CutPrefix(a, p); ok {
				return rest
			}
		}
	}
	return ""
}

func resolveWidth(override int) int {
	if override > 0 {
		return override
	}
	return status.TerminalWidth()
}

func shortHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	if i := strings.Index(h, "."); i > 0 {
		return h[:i]
	}
	return h
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// demoInput mirrors a mid-session payload so running the binary from a shell
// exercises every segment.
func demoInput() status.Input {
	cwd, _ := os.Getwd()
	pct := 38.0
	now := time.Now()

	return status.Input{
		Cwd:   cwd,
		Model: status.Model{DisplayName: "Opus 4.8", ID: "claude-opus-4-8"},
		Cost: status.Cost{
			TotalCostUSD:       3.45,
			TotalDurationMs:    42 * 60 * 1000,
			TotalAPIDurationMs: 198_000,
			TotalLinesAdded:    482,
			TotalLinesRemoved:  37,
		},
		ContextWindow: status.ContextWindow{
			ContextWindowSize: 200_000,
			TotalOutputTokens: 24_500,
			UsedPercentage:    &pct,
			CurrentUsage: &status.CurrentUsage{
				InputTokens:              65_000,
				CacheCreationInputTokens: 8_000,
				CacheReadInputTokens:     53_000,
			},
		},
		Effort: &status.Effort{Level: "high"},
		PR:     &status.PR{Number: 42, URL: "https://github.com/r-neves/claude-code-status-bar/pull/42"},
		RateLimits: &status.RateLimits{
			FiveHour: &status.Window{
				UsedPercentage: 30,
				ResetsAt:       status.Epoch{Time: now.Add(2 * time.Hour), Set: true},
			},
			SevenDay: &status.Window{
				UsedPercentage: 20,
				ResetsAt:       status.Epoch{Time: now.Add(4 * 24 * time.Hour), Set: true},
			},
		},
	}
}
